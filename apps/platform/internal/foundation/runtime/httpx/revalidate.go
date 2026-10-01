package httpx

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
)

const (
	SharedBriefly = "public, max-age=30"
	PrivateFresh  = "private, no-cache"
	etagBytes     = 16

	largestPooledBody = 256 << 10
)

var responseBodies = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func borrowBody() *bytes.Buffer { return responseBodies.Get().(*bytes.Buffer) }

func returnBody(b *bytes.Buffer) {
	if b.Cap() > largestPooledBody {
		return
	}
	b.Reset()
	responseBodies.Put(b)
}

type bufferedResponse struct {
	header http.Header
	status int
	body   *bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	b.WriteHeader(http.StatusOK)
	return b.body.Write(p)
}

func Revalidated(cacheControl string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buffered := &bufferedResponse{header: w.Header(), body: borrowBody()}
		defer returnBody(buffered.body)
		next(buffered, r)
		if buffered.status == 0 {
			buffered.status = http.StatusOK
		}
		if buffered.status != http.StatusOK {
			w.WriteHeader(buffered.status)
			_, _ = w.Write(buffered.body.Bytes())
			return
		}
		sum := sha256.Sum256(buffered.body.Bytes())
		etag := `"` + base64.RawURLEncoding.EncodeToString(sum[:etagBytes]) + `"`
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", cacheControl)
		if matchesAny(r.Header.Get("If-None-Match"), etag) {
			w.Header().Del("Content-Type")
			w.Header().Del("Content-Length")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buffered.body.Bytes())
	}
}

func matchesAny(ifNoneMatch, etag string) bool {
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag || candidate == "*" {
			return true
		}
	}
	return false
}
