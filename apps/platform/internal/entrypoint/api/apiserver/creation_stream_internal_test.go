package apiserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAStreamOutlivesTheServerWriteTimeout(t *testing.T) {
	const writeTimeout = 100 * time.Millisecond
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		holdOpenForStreaming(w)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(3 * writeTimeout)
		_, _ = w.Write([]byte("still streaming"))
	}))
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "still streaming") {
		t.Fatalf("body %q: the server's write timeout cut the stream", body)
	}
}
