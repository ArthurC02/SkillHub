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

// streamingSignAlgorithm is the value minio-go puts in the
// X-Amz-Content-Sha256 header of every non-TLS PutObject request (see
// signer.StreamingSignV4 in github.com/minio/minio-go/v7/pkg/signer). Its
// presence is how the backend below knows the request body is aws-chunked
// framed rather than raw object bytes.
const streamingSignAlgorithm = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"

// inProcessBackend is an in-process HTTP server that speaks enough of the S3
// wire protocol for the production *Client (backed by minio-go) to talk to it
// unmodified. NewInProcess starts one bound to an ephemeral localhost port and
// hands back a *Client already pointed at it - the same client type, same
// minio-go request path, that talks to SeaweedFS or a managed S3 service in
// every other environment (ADR-018).
//
// What it does NOT do, and must never be mistaken for:
//   - it does not verify request signatures
//   - it does not verify expiry on presigned URLs
//   - it does not distinguish GET-authorized access from PUT-authorized access
//
// A prior read-only survey exercised an equivalent stand-in against four
// mutations of a presigned URL - a tampered signature, a stripped signature,
// a URL expired by three seconds, and a GET-scoped presign string reused for
// a PUT - and all four returned HTTP 200; the last one actually overwrote the
// object. This backend is therefore never evidence for SBX-008 (short-lived,
// single-direction authorization). Its only job is to exercise the real
// minio-go client code path in tests instead of a hand-rolled substitute
// protocol (PORT-001: a fake must not diverge in behavior from the real
// implementation it stands in for).
type inProcessBackend struct {
	mu            sync.RWMutex
	bucket        string
	bucketCreated bool
	objects       map[string]storedObject
	// accessKey is a per-process random string every request must carry, in the
	// Authorization header (minio-go signs each call) or in a presigned URL's
	// X-Amz-Credential (the sandbox hits those with a plain HTTP client).
	//
	// This is NOT authentication and the doc comment above still stands: nothing
	// verifies a signature, so a caller holding the key can do anything. What it
	// buys is that a caller must HOLD it. The listener is on loopback with no
	// credential at all, and in clean mode it is a real deployment — every skill
	// package, dataset, run artifact and download bundle on that machine was
	// readable and writable by any other local process that could guess a key,
	// including a script in a browser tab the demo has open (IMPORT_ALLOW_INSECURE
	// is almost certainly on there too, which puts loopback back on the import
	// fetcher's reachable set).
	accessKey string
}

type storedObject struct {
	data    []byte
	modTime time.Time
	etag    string
}

// NewInProcess starts the in-process S3 stand-in described on
// inProcessBackend and returns a *Client wired to it, plus a func to shut the
// server down. The bucket does not exist until the caller calls
// EnsureBucket (or Client.mc.MakeBucket) against the returned client, just
// like every other environment.
func NewInProcess(bucket string) (*Client, func(), error) {
	// crypto/rand, not a fixed string: the point of the key is that a process
	// which was not handed it cannot guess it.
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

// carriesKey reports whether the request presents this process's access key at
// all. Signed requests carry it inside the Authorization header's Credential
// field; presigned URLs carry it as X-Amz-Credential. Either is enough — see
// the accessKey field for why "holds the key" and not "proved the signature" is
// the bar here.
func (b *inProcessBackend) carriesKey(r *http.Request) bool {
	if b.accessKey == "" {
		return true // a zero-value backend in a unit test; no key was ever issued
	}
	if strings.Contains(r.Header.Get("Authorization"), b.accessKey) {
		return true
	}
	return strings.HasPrefix(r.URL.Query().Get("X-Amz-Credential"), b.accessKey)
}

func (b *inProcessBackend) serveBucket(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// minio-go resolves the bucket's region (GET /bucket/?location) before
		// every single request, including BucketExists itself - not just
		// MakeBucket. There is one region here, so answer the same empty
		// LocationConstraint (= us-east-1) unconditionally.
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
		// S3 DELETE always answers 204, present or not (objstore.Remove's doc
		// comment on this same contract).
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

// readObjectBody returns the raw object bytes carried by a PUT request,
// undoing the aws-chunked / streaming-signature-v4 framing that minio-go
// applies to every non-TLS PutObject call (signer.StreamingSignV4). A
// presigned PUT hit directly with a plain HTTP client (as the sandbox does)
// carries no such framing and is read as-is.
func readObjectBody(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	if r.Header.Get("X-Amz-Content-Sha256") == streamingSignAlgorithm {
		return decodeAWSChunked(r.Body)
	}
	return readCapped(r.Body, MaxObjectBytes)
}

// maxChunkBytes caps one aws-chunked frame. minio-go's StreamingReader writes
// chunks far below this (its default part size is 16 MiB), so no legitimate PUT
// comes near it; the number exists because the size is read out of the request.
//
// Before this, `chunk := make([]byte, size)` allocated whatever the caller's hex
// header asked for: a PUT declaring `7fffffffffffffff;chunk-signature=x` asked
// for 8 EB in one allocation, and in clean mode — where this backend is the
// real object store and the listener takes requests from any local process --
// that is the demo machine going down mid-demonstration.
const maxChunkBytes = 64 << 20

// decodeAWSChunked strips the aws-chunked framing minio-go's StreamingReader
// writes: a sequence of "<hex-size>;chunk-signature=<sig>\r\n<data>\r\n"
// chunks terminated by a zero-size chunk ("0;chunk-signature=<sig>\r\n\r\n").
// Chunk signatures are not verified - see the inProcessBackend doc comment.
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
		if _, err := io.ReadFull(br, make([]byte, 2)); err != nil { // trailing CRLF
			return nil, fmt.Errorf("aws-chunked: read chunk trailer: %w", err)
		}
	}
	return out.Bytes(), nil
}
