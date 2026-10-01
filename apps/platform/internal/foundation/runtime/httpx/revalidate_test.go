package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func answering(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func serve(h http.HandlerFunc, ifNoneMatch string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	if ifNoneMatch != "" {
		r.Header.Set("If-None-Match", ifNoneMatch)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestAnAnswerCarriesItsTagAndCachePolicy(t *testing.T) {
	w := serve(Revalidated(SharedBriefly, answering(http.StatusOK, `{"a":1}`)), "")
	if w.Code != http.StatusOK || w.Body.String() != `{"a":1}` {
		t.Fatalf("got %d %q, want the handler's 200 and body", w.Code, w.Body.String())
	}
	if w.Header().Get("ETag") == "" || w.Header().Get("Cache-Control") != SharedBriefly {
		t.Fatalf("ETag %q Cache-Control %q", w.Header().Get("ETag"), w.Header().Get("Cache-Control"))
	}
}

func TestDifferentBodiesGetDifferentTags(t *testing.T) {
	a := serve(Revalidated(PrivateFresh, answering(http.StatusOK, `{"a":1}`)), "").Header().Get("ETag")
	b := serve(Revalidated(PrivateFresh, answering(http.StatusOK, `{"a":2}`)), "").Header().Get("ETag")
	if a == b {
		t.Fatalf("two bodies share the tag %s", a)
	}
}

func TestARevalidationWithTheCurrentTagIsAnsweredWithoutABody(t *testing.T) {
	h := Revalidated(PrivateFresh, answering(http.StatusOK, `{"a":1}`))
	etag := serve(h, "").Header().Get("ETag")
	for name, ifNoneMatch := range map[string]string{
		"exact":       etag,
		"weak":        "W/" + etag,
		"one of many": `"other", ` + etag,
		"any version": "*",
	} {
		t.Run(name, func(t *testing.T) {
			w := serve(h, ifNoneMatch)
			if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
				t.Fatalf("got %d with %d body bytes, want 304 and none", w.Code, w.Body.Len())
			}
			if w.Header().Get("ETag") != etag || w.Header().Get("Content-Type") != "" {
				t.Fatalf("ETag %q Content-Type %q, want the tag and no content type", w.Header().Get("ETag"), w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestAStaleTagGetsTheFullAnswer(t *testing.T) {
	w := serve(Revalidated(PrivateFresh, answering(http.StatusOK, `{"a":1}`)), `"stale"`)
	if w.Code != http.StatusOK || w.Body.String() != `{"a":1}` {
		t.Fatalf("got %d %q, want 200 and the body", w.Code, w.Body.String())
	}
}

func TestAnErrorIsPassedThroughUntaggedAndUncached(t *testing.T) {
	w := serve(Revalidated(SharedBriefly, answering(http.StatusNotFound, `{"error":"gone"}`)), "*")
	if w.Code != http.StatusNotFound || w.Body.String() != `{"error":"gone"}` {
		t.Fatalf("got %d %q, want the handler's 404 and body", w.Code, w.Body.String())
	}
	if w.Header().Get("ETag") != "" || w.Header().Get("Cache-Control") != "" {
		t.Fatalf("an error was tagged %q and cached %q", w.Header().Get("ETag"), w.Header().Get("Cache-Control"))
	}
}

func TestAHandlerThatOnlyWritesABodyIsAnOK(t *testing.T) {
	h := Revalidated(SharedBriefly, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) })
	if w := serve(h, ""); w.Code != http.StatusOK || w.Header().Get("ETag") == "" {
		t.Fatalf("got %d with ETag %q, want an implicit 200 that is tagged", w.Code, w.Header().Get("ETag"))
	}
}

func TestAHandlerThatWritesNothingIsAnEmptyOK(t *testing.T) {
	w := serve(Revalidated(SharedBriefly, func(http.ResponseWriter, *http.Request) {}), "")
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatalf("got %d with %d body bytes, want an empty 200", w.Code, w.Body.Len())
	}
}

func TestAnAnswerNeverCarriesWhatAnEarlierAnswerWrote(t *testing.T) {
	long := Revalidated(PrivateFresh, answering(http.StatusOK, `{"owner":"alice","secret":"not for bob"}`))
	short := Revalidated(PrivateFresh, answering(http.StatusOK, `{"owner":"bob"}`))
	for range 50 {
		serve(long, "")
		if w := serve(short, ""); w.Body.String() != `{"owner":"bob"}` {
			t.Fatalf("got %q, want only the second handler's body", w.Body.String())
		}
	}
}
