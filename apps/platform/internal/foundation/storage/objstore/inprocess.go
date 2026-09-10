package objstore

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const streamingSignAlgorithm = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"

type inProcessBackend struct {
	mu            sync.RWMutex
	bucket        string
	bucketCreated bool
	objects       map[string]storedObject

	accessKey string
}

type storedObject struct {
	data    []byte
	modTime time.Time
	etag    string
}

func NewInProcess(bucket string) (*Client, func(), error) {

	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, nil, fmt.Errorf("objstore inprocess key: %w", err)
	}
	accessKey := "inprocess" + hex.EncodeToString(seed[:])
	backend := &inProcessBackend{
		bucket: bucket, objects: make(map[string]storedObject), accessKey: accessKey,
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("objstore inprocess listen: %w", err)
	}
	srv := &http.Server{Handler: backend}
	go func() { _ = srv.Serve(ln) }()

	client, err := New(ln.Addr().String(), accessKey, accessKey, bucket, false)
	if err != nil {
		_ = srv.Close()
		return nil, nil, err
	}
	stop := func() { _ = srv.Close() }
	return client, stop, nil
}

func (b *inProcessBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !b.carriesKey(r) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(parts) == 0 || parts[0] != b.bucket {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 1 || parts[1] == "" {
		b.serveBucket(w, r)
		return
	}
	b.serveObject(w, r, parts[1])
}

func (b *inProcessBackend) carriesKey(r *http.Request) bool {
	if b.accessKey == "" {
		return true
	}
	if strings.Contains(r.Header.Get("Authorization"), b.accessKey) {
		return true
	}
	return strings.HasPrefix(r.URL.Query().Get("X-Amz-Credential"), b.accessKey)
}

func (b *inProcessBackend) serveBucket(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></LocationConstraint>`)
	case http.MethodHead:
		b.mu.RLock()
		exists := b.bucketCreated
		b.mu.RUnlock()
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		b.mu.Lock()
		b.bucketCreated = true
		b.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (b *inProcessBackend) serveObject(w http.ResponseWriter, r *http.Request, key string) {
	switch r.Method {
	case http.MethodGet:
		obj, ok := b.lookup(key)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		setObjectHeaders(w, obj)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(obj.data)
	case http.MethodHead:
		obj, ok := b.lookup(key)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		setObjectHeaders(w, obj)
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		data, err := readObjectBody(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sum := md5.Sum(data)
		obj := storedObject{data: data, modTime: time.Now().UTC(), etag: hex.EncodeToString(sum[:])}
		b.mu.Lock()
		b.objects[key] = obj
		b.mu.Unlock()
		w.Header().Set("ETag", `"`+obj.etag+`"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		b.mu.Lock()
		delete(b.objects, key)
		b.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (b *inProcessBackend) lookup(key string) (storedObject, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	obj, ok := b.objects[key]
	return obj, ok
}

func setObjectHeaders(w http.ResponseWriter, obj storedObject) {
	w.Header().Set("ETag", `"`+obj.etag+`"`)
	w.Header().Set("Last-Modified", obj.modTime.Format(http.TimeFormat))
	w.Header().Set("Content-Length", strconv.Itoa(len(obj.data)))
}

// readObjectBody dispatches on X-Amz-Content-Sha256: the minio-go client
// stamps this value on every non-TLS PutObject to mark the body as
// aws-chunked rather than raw bytes.
func readObjectBody(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	if r.Header.Get("X-Amz-Content-Sha256") == streamingSignAlgorithm {
		return decodeAWSChunked(r.Body)
	}
	return readCapped(r.Body, MaxObjectBytes)
}

// maxChunkBytes bounds a value read straight off an untrusted request header;
// without a ceiling here, a declared chunk size becomes the size of the
// allocation below.
const maxChunkBytes = 64 << 20

// decodeAWSChunked strips the aws-chunked framing: a sequence of
// "<hex-size>;chunk-signature=<sig>\r\n<data>\r\n" chunks ending in a
// zero-size chunk. Chunk signatures are read but not verified.
func decodeAWSChunked(r io.Reader) ([]byte, error) {
	br := bufio.NewReader(r)
	var out bytes.Buffer
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("aws-chunked: read chunk header: %w", err)
		}
		sizeHex := strings.TrimRight(line, "\r\n")
		if i := strings.IndexByte(sizeHex, ';'); i >= 0 {
			sizeHex = sizeHex[:i]
		}
		size, err := strconv.ParseInt(sizeHex, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("aws-chunked: bad chunk size %q: %w", sizeHex, err)
		}
		if size == 0 {
			break
		}
		if size < 0 || size > maxChunkBytes {
			return nil, fmt.Errorf("aws-chunked: chunk of %d bytes is over the %d byte limit", size, maxChunkBytes)
		}
		if int64(out.Len())+size > MaxObjectBytes {
			return nil, fmt.Errorf("aws-chunked: body is larger than the %d byte ceiling", MaxObjectBytes)
		}
		chunk := make([]byte, size)
		if _, err := io.ReadFull(br, chunk); err != nil {
			return nil, fmt.Errorf("aws-chunked: read chunk data: %w", err)
		}
		out.Write(chunk)
		if _, err := io.ReadFull(br, make([]byte, 2)); err != nil {
			return nil, fmt.Errorf("aws-chunked: read chunk trailer: %w", err)
		}
	}
	return out.Bytes(), nil
}
