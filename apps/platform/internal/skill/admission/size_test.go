package ingest

// 03:INGEST-016: a package refused for its size says the numbers, echoes nothing
// the caller sent (NFR-001), and is counted.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// The refusal a creator actually reads. Before this, it was "package exceeds the
// upload size limit" on both upload doors: no ceiling, no size, and therefore no
// way to tell 1 KB over from 10 MB over — which is the difference between
// trimming a file and coming to ask.
func TestAnOversizedUploadIsToldBothNumbers(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/skills/import/upload", strings.NewReader(""))
	r.ContentLength = skillpkg.MaxZipBytes * 2
	writeTooLarge(w, r)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status %d, want 413", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, skillpkg.HumanMB(skillpkg.MaxZipBytes)) {
		t.Errorf("the refusal does not name the platform's ceiling: %s", body)
	}
	if !strings.Contains(body, skillpkg.HumanMB(skillpkg.MaxZipBytes*2)) {
		t.Errorf("the refusal does not name what was actually sent: %s", body)
	}
}

// http.MaxBytesReader stops at limit+1 and never learns how much more there was,
// so when the request does not declare a size over the ceiling there is no honest
// second number. Saying the ceiling alone is the requirement; inventing a size
// would be worse than saying nothing.
func TestAnOversizedUploadWithNoUsableLengthOnlyClaimsTheCeiling(t *testing.T) {
	for _, length := range []int64{-1, 0, skillpkg.MaxZipBytes} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/skills/import/upload", strings.NewReader(""))
		r.ContentLength = length
		writeTooLarge(w, r)

		body := w.Body.String()
		if !strings.Contains(body, skillpkg.HumanMB(skillpkg.MaxZipBytes)) {
			t.Errorf("ContentLength=%d: the ceiling is missing: %s", length, body)
		}
		if strings.Contains(body, "這一次送出的是") {
			t.Errorf("ContentLength=%d: a size was claimed that nobody measured: %s", length, body)
		}
	}
}

// NFR-001. The refusal is two integers the platform already knew; nothing the
// caller chose may travel back out in it — not a file name, not a path, not a URL.
func TestASizeRefusalEchoesNothingTheCallerSent(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/skills/import/upload?name=secret-client.zip",
		strings.NewReader(""))
	r.Header.Set("X-Filename", "/home/alice/acme-internal/payroll.zip")
	r.ContentLength = skillpkg.MaxZipBytes * 3
	writeTooLarge(w, r)

	body := w.Body.String()
	for _, leak := range []string{"secret-client", "payroll", "/home/alice", "acme-internal"} {
		if strings.Contains(body, leak) {
			t.Errorf("the refusal echoed %q back to the caller: %s", leak, body)
		}
	}
}

// The counting half. Refusals only: a counter that also moves on success answers
// "is this ceiling too tight" (05 R-13) with noise.
func TestAnOversizedUploadIsCounted(t *testing.T) {
	before := refusalCount(t, metrics.CeilingUpload)
	otherBefore := refusalCount(t, metrics.CeilingURL)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/skills/import/upload", strings.NewReader(""))
	r.ContentLength = skillpkg.MaxZipBytes * 2
	writeTooLarge(w, r)
	if got := refusalCount(t, metrics.CeilingUpload) - before; got != 1 {
		t.Errorf("upload size refusals counted %v times, want 1", got)
	}
	// And it must not land in another door's series: the three ceilings exist to
	// be told apart.
	if got := refusalCount(t, metrics.CeilingURL) - otherBefore; got != 0 {
		t.Errorf("an upload refusal moved the url series by %v", got)
	}
}

func refusalCount(t *testing.T, ceiling string) float64 {
	t.Helper()
	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	prefix := `skillhub_package_size_refused_total{ceiling="` + ceiling + `"} `
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			if err != nil {
				t.Fatalf("unparseable counter line %q: %v", line, err)
			}
			return v
		}
	}
	return 0 // never incremented: Prometheus does not export an untouched child
}

// 04 丙-138 — the sentence a creator reads when a URL import is refused.
//
// It used to be 「匯入失敗：fetch failed: host "gitlab.com" is not on the allowed
// source list」: an English clause inside a Chinese one, two 「失敗」 stacked, and
// for a Chinese reader the refusal did not even establish what was wrong. The
// standard was already set two functions up in this same file — `writeTooLarge`
// answers in full Chinese with both numbers.
//
// Both halves are pinned, because either one alone is passable by a wrong
// implementation: the sentence has to be the readable one, and the sentinel's
// own text — a Go identifier, not copy — must not travel with it.
func TestAUrlRefusalReachesTheCreatorAsOneChineseSentence(t *testing.T) {
	f := &URLFetcher{Allowed: map[string]bool{"github.com": true}}
	_, _, err := f.Fetch(t.Context(), "https://gitlab.com/example/skill/archive/main.zip")
	if err == nil {
		t.Fatal("a host off the allow list was not refused")
	}

	w := httptest.NewRecorder()
	(&Handler{}).respond(w, Result{}, err)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "不在允許清單內") {
		t.Errorf("the refusal is not the sentence a reader can act on: %s", body)
	}
	if !strings.Contains(body, "gitlab.com") {
		t.Errorf("the refusal does not name the host that was refused: %s", body)
	}
	// The classification stays on the error and off the screen.
	if strings.Contains(body, "fetch failed") {
		t.Errorf("the Go sentinel's own text reached the client: %s", body)
	}
	if !errors.Is(err, ErrFetch) {
		t.Error("stripping the prefix must not cost the classification")
	}
}
