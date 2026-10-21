package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func servingBody(t *testing.T, body string, chunked bool) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if chunked {
			w.(http.Flusher).Flush()
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/signed?secret=must-not-leak"
}

func TestFetchGrantAcceptsAnObjectExactlyAtTheLimit(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		got, err := FetchGrant(context.Background(), servingBody(t, "12345", chunked), 5)
		if err != nil || string(got) != "12345" {
			t.Fatalf("chunked=%v: got %q, %v; want the five bytes", chunked, got, err)
		}
	}
}

func TestFetchGrantRejectsAnObjectOneByteOverTheLimit(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		url := servingBody(t, "123456", chunked)
		_, err := FetchGrant(context.Background(), url, 5)
		if err == nil || err.Error() != "object exceeds the grant size limit" {
			t.Fatalf("chunked=%v: err = %v, want the size-limit refusal", chunked, err)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("grant URL leaked into error: %v", err)
		}
	}
}
