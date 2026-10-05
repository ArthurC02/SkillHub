package trace

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("connection reset by peer") }

func ingestStatus(t *testing.T, body io.Reader) (int, string) {
	t.Helper()
	signer := &Signer{Secret: []byte("secret")}
	token := signer.Mint(runUUID(t, "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20"), 1, time.Now())
	req := httptest.NewRequest(http.MethodPost, "/ingest/"+token, body)
	req.SetPathValue("token", token)
	rec := httptest.NewRecorder()
	(&Handler{Svc: &Service{Signer: signer}}).Ingest(rec, req)
	return rec.Code, rec.Body.String()
}

func TestIngestSeparatesAnOversizedBatchFromAnUnreadableOne(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     io.Reader
		wantCode int
		wantText string
	}{
		{"a batch of exactly the limit is read and judged malformed", strings.NewReader(strings.Repeat("x", maxBatchBytes)), http.StatusBadRequest, "JSON array"},
		{"a batch one byte over the limit is too large", strings.NewReader(strings.Repeat("x", maxBatchBytes+1)), http.StatusRequestEntityTooLarge, "too large"},
		{"a body that fails mid-read is unreadable, not too large", brokenBody{}, http.StatusBadRequest, "could not read the trace batch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, text := ingestStatus(t, tc.body)
			if code != tc.wantCode || !strings.Contains(text, tc.wantText) {
				t.Fatalf("got %d %q, want %d containing %q", code, text, tc.wantCode, tc.wantText)
			}
		})
	}
}
