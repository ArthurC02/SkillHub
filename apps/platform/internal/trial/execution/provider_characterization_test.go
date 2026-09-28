package run

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func providerAnswering(t *testing.T, status int, body string) *httpProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &httpProvider{name: "test", baseURL: srv.URL, HTTP: srv.Client()}
}

func TestARefusalWithoutABodyIsNamedByItsStatusText(t *testing.T) {
	_, err := providerAnswering(t, http.StatusConflict, "").Observe(context.Background(), "sbx-1")
	if pe, ok := errors.AsType[*providerError](err); !ok || pe.Message != "Conflict" {
		t.Fatalf("err = %v, want a provider refusal carrying the status text", err)
	}
}

func TestAnExpectedAnswerThatIsNotJSONIsADecodeError(t *testing.T) {
	_, err := providerAnswering(t, http.StatusOK, "not json").Observe(context.Background(), "sbx-1")
	if err == nil || !strings.Contains(err.Error(), "decode GET /runs/sbx-1") {
		t.Fatalf("err = %v, want the decode failure named with its request", err)
	}
	if _, ok := errors.AsType[*providerError](err); ok {
		t.Errorf("a decode failure was classified as a provider refusal: %v", err)
	}
}

func TestAnOversizedExpectedAnswerIsATransportErrorNotARefusal(t *testing.T) {
	body := `{"state":"running"}` + strings.Repeat(" ", 4<<20)
	_, err := providerAnswering(t, http.StatusOK, body).Observe(context.Background(), "sbx-1")
	if err == nil {
		t.Fatal("an answer past the read limit was accepted")
	}
	if _, ok := errors.AsType[*providerError](err); ok {
		t.Errorf("an oversized answer with an expected status was classified as a refusal: %v", err)
	}
}

func TestTheGatewayEnforcesItsReadLimitOnAWellFormedAnswer(t *testing.T) {
	const limit = int64(32)
	body := `{"ok":true}` + strings.Repeat(" ", int(limit))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	g := &Gateway{adminBaseURL: srv.URL, client: srv.Client()}
	var out map[string]any
	if err := g.do(context.Background(), adminRequest{method: http.MethodGet, path: "/", responseLimit: limit}, &out); err == nil {
		t.Fatal("a valid JSON answer larger than the limit was accepted")
	}
	if err := g.do(context.Background(), adminRequest{method: http.MethodGet, path: "/", responseLimit: 1 << 10}, &out); err != nil {
		t.Fatalf("the same answer inside a larger limit was refused: %v", err)
	}
}

func TestARunKeyIsAttributedToItsRunAndAttempt(t *testing.T) {
	var got struct {
		Metadata map[string]string `json:"metadata"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"key":"sk-virtual"}`)
	}))
	defer srv.Close()

	g := &Gateway{adminBaseURL: srv.URL, client: srv.Client()}
	if _, err := g.Issue(context.Background(), "run-1", "attempt-1", time.Minute, 0); err != nil {
		t.Fatal(err)
	}
	if len(got.Metadata) != 2 || got.Metadata["run_id"] != "run-1" || got.Metadata["run_attempt_id"] != "attempt-1" {
		t.Errorf("metadata = %v, want the run and its attempt", got.Metadata)
	}
}

func TestAnArtifactContentTypeOf255BytesIsAcceptedAnd256IsNot(t *testing.T) {
	validHash := strings.Repeat("0", 64)
	record := func(contentType string) error {
		d := &driver{cur: gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}}
		return d.recordArtifacts(context.Background(), gen.RunAttempt{}, ProviderRun{Result: &RunResult{
			Artifacts: []RunArtifact{{FileName: "out.txt", ContentHash: validHash, ContentType: contentType}},
		}})
	}
	atLimit := "text/" + strings.Repeat("a", 250)
	if err := record(atLimit); err == nil || !strings.Contains(err.Error(), "persistence is not configured") {
		t.Errorf("a 255-byte content type did not reach persistence: %v", err)
	}
	if err := record(atLimit + "a"); err == nil || !strings.Contains(err.Error(), "invalid artifact content type") {
		t.Errorf("a 256-byte content type was not refused: %v", err)
	}
}

func TestAnArtifactHashLongerThanSHA256IsRefused(t *testing.T) {
	d := &driver{cur: gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}}
	err := d.recordArtifacts(context.Background(), gen.RunAttempt{}, ProviderRun{Result: &RunResult{
		Artifacts: []RunArtifact{{FileName: "out.txt", ContentHash: strings.Repeat("0", 66)}},
	}})
	if err == nil || !strings.Contains(err.Error(), "invalid artifact hash") {
		t.Fatalf("a 33-byte hash was not refused: %v", err)
	}
}

func TestTheSummaryAsksTheFirstCompatibleProvider(t *testing.T) {
	first, second := withSlots("first", 1), withSlots("second", 1)
	first.Injects, second.Injects = []string{"FROM_FIRST"}, []string{"FROM_SECOND"}
	svc := &Service{Providers: registryWithCapabilities(first, second)}

	if got := svc.injectedSecretsFor(context.Background(), withGatewayGrant()); !slices.Equal(got, []string{"FROM_FIRST"}) {
		t.Errorf("injected = %v, want the first compatible provider's declaration", got)
	}
}
