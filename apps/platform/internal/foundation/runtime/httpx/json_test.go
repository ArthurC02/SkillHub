package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type decodeTarget struct {
	Name string `json:"name"`
}

func decodeWith(t *testing.T, body string, maxBytes int64) (decodeTarget, bool, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	var got decodeTarget
	ok := DecodeJSON(rec, req, maxBytes, &got, "custom bad body")
	return got, ok, rec
}

func TestDecodeJSONValidBodyReturnsValueAndWritesNothing(t *testing.T) {
	body := `{"name":"abc"}`
	got, ok, rec := decodeWith(t, body, int64(len(body)))
	if !ok || got.Name != "abc" {
		t.Fatalf("ok=%v got=%+v, want ok=true name=abc", ok, got)
	}
	if rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "" {
		t.Fatalf("wrote to response: %q %v", rec.Body.String(), rec.Header())
	}
}

func TestDecodeJSONBodyExactlyAtLimitIsAccepted(t *testing.T) {
	body := `{"name":"abc"}`
	_, ok, rec := decodeWith(t, body, int64(len(body)))
	if !ok || rec.Code != http.StatusOK {
		t.Fatalf("ok=%v code=%d", ok, rec.Code)
	}
}

func TestDecodeJSONBodyOneByteOverLimitIsRejectedWithCallerMessage(t *testing.T) {
	body := `{"name":"abc"}`
	_, ok, rec := decodeWith(t, body, int64(len(body))-1)
	if ok {
		t.Fatal("ok=true, want false")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d, want 400", rec.Code)
	}
	if want := "{\"error\":\"custom bad body\"}\n"; rec.Body.String() != want {
		t.Fatalf("body=%q, want %q", rec.Body.String(), want)
	}
}

func TestDecodeJSONMalformedBodyIsRejectedWithCallerMessage(t *testing.T) {
	_, ok, rec := decodeWith(t, `{"name":`, 1024)
	if ok || rec.Code != http.StatusBadRequest {
		t.Fatalf("ok=%v code=%d, want false/400", ok, rec.Code)
	}
	if want := "{\"error\":\"custom bad body\"}\n"; rec.Body.String() != want {
		t.Fatalf("body=%q, want %q", rec.Body.String(), want)
	}
}

func TestDecodeOneJSONAcceptsExactlyOneKnownObject(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"one known object", `{"name":"abc"}`, true},
		{"an unknown field", `{"name":"abc","extra":1}`, false},
		{"a second object after the first", `{"name":"abc"}{"name":"def"}`, false},
		{"trailing garbage", `{"name":"abc"} x`, false},
		{"malformed", `{"name":`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
			var got decodeTarget
			ok := DecodeOneJSON(rec, req, 1024, &got, "custom bad body")
			if ok != c.ok {
				t.Fatalf("ok=%v, want %v", ok, c.ok)
			}
			if c.ok && got.Name != "abc" {
				t.Fatalf("name=%q, want abc", got.Name)
			}
			if !c.ok && rec.Body.String() != "{\"error\":\"custom bad body\"}\n" {
				t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDecodeOneJSONRefusesABodyOneByteOverItsLimit(t *testing.T) {
	body := `{"name":"abc"}`
	for limit, want := range map[int64]bool{int64(len(body)): true, int64(len(body)) - 1: false} {
		rec := httptest.NewRecorder()
		var got decodeTarget
		ok := DecodeOneJSON(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), limit, &got, "custom bad body")
		if ok != want {
			t.Fatalf("limit %d: ok=%v, want %v", limit, ok, want)
		}
	}
}
