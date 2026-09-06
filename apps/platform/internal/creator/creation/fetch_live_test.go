package creation

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFetcherAgainstTheLiveInternet is a measurement, not a gate: it runs only
// with SKILLHUB_LIVE_FETCH=1 on a machine with egress (the measurement
// container has none, so run q could only show network_error). It exercises
// the three endings the owner's rule names (05 R-47): a page that reads, a
// site that refuses (reported once), and a page that does not exist.
func TestFetcherAgainstTheLiveInternet(t *testing.T) {
	if os.Getenv("SKILLHUB_LIVE_FETCH") != "1" {
		t.Skip("SKILLHUB_LIVE_FETCH=1 selects this live measurement")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	f := NewFetcher(false)
	for _, tc := range []struct{ url, want, contains string }{
		{"https://example.com/", "ok", "Example Domain"},
		{"https://www.rfc-editor.org/rfc/rfc2119.txt", "ok", "MUST"},
		{"https://httpbin.org/status/403", "blocked", ""},
		{"https://example.com/definitely-not-here", "not_found", ""},
	} {
		start := time.Now()
		rec, text := f.Fetch(ctx, tc.url)
		t.Logf("%s -> %s bytes=%d sha=%.12s in %s", tc.url, rec.Status, rec.Bytes, rec.SHA256, time.Since(start).Round(time.Millisecond))
		if rec.Status != tc.want {
			t.Errorf("%s: status %q, want %q", tc.url, rec.Status, tc.want)
		}
		if tc.contains != "" && !strings.Contains(text, tc.contains) {
			t.Errorf("%s: text does not contain %q: %.120s", tc.url, tc.contains, text)
		}
	}
}
