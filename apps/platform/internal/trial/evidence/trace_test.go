package trace

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestRunReadersFailClosedAndHideMissingRuns(t *testing.T) {
	ctx := context.Background()
	var svc Service
	if _, err := svc.Ingest(ctx, Grant{}, "token", nil); !errors.Is(err, errRunReaderNotConfigured) {
		t.Errorf("Ingest without owner reader: %v", err)
	}
	if _, err := svc.Advanced(ctx, pgtype.UUID{}, pgtype.UUID{}, 0); !errors.Is(err, errRunReaderNotConfigured) {
		t.Errorf("Advanced without owner reader: %v", err)
	}
	if _, err := svc.General(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, errRunReaderNotConfigured) {
		t.Errorf("General without transition reader: %v", err)
	}

	svc.ReadRunState = func(context.Context, pgtype.UUID, pgtype.UUID) (RunState, bool, error) {
		return RunState{}, false, nil
	}
	if _, err := svc.Advanced(ctx, pgtype.UUID{}, pgtype.UUID{}, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing scoped run: %v", err)
	}
	svc.ReadIngestRunState = func(context.Context, pgtype.UUID) (IngestRunState, bool, error) {
		return IngestRunState{}, false, nil
	}
	if _, err := svc.Ingest(ctx, Grant{}, "token", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing ingestion run: %v", err)
	}
}

func TestPublishedPersistenceFacesFailClosed(t *testing.T) {
	ctx := context.Background()
	if err := RecordOrchestratorEvent(ctx, nil, pgtype.UUID{}, pgtype.UUID{}, 1, TypeError, "error", nil); !errors.Is(err, errPersistenceNotConfigured) {
		t.Errorf("RecordOrchestratorEvent without transaction: %v", err)
	}
	if _, err := (&Service{}).MaskingActivity(ctx, time.Now(), time.Now()); !errors.Is(err, errPersistenceNotConfigured) {
		t.Errorf("MaskingActivity without pool: %v", err)
	}
}

func runUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatal(err)
	}
	return u
}

// The token is the whole authorization story for ingestion, so each way of
// forging one gets its own assertion.
func TestIngestionTokenGrantsExactlyOneAttempt(t *testing.T) {
	signer := &Signer{Secret: []byte("secret")}
	runID := runUUID(t, "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20")
	now := time.Now()

	grant, err := signer.Verify(signer.Mint(runID, 2, now), now)
	if err != nil {
		t.Fatalf("freshly minted token rejected: %v", err)
	}
	if grant.RunID != runID || grant.Attempt != 2 {
		t.Fatalf("grant %v does not name the run and attempt it was minted for", grant)
	}
}

func TestIngestionTokenRejectsTampering(t *testing.T) {
	signer := &Signer{Secret: []byte("secret")}
	runID := runUUID(t, "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20")
	now := time.Now()
	token := signer.Mint(runID, 1, now)

	// Editing the attempt is the interesting forgery: it would let one attempt
	// write into another attempt's stream, where the seq numbers collide.
	forged := strings.Replace(token, ".1.", ".2.", 1)
	if _, err := signer.Verify(forged, now); err == nil {
		t.Error("a token with an edited attempt was accepted")
	}
	if _, err := signer.Verify(token+"x", now); err == nil {
		t.Error("a token with an edited signature was accepted")
	}
	if _, err := (&Signer{Secret: []byte("other")}).Verify(token, now); err == nil {
		t.Error("a token signed with another secret was accepted")
	}
	if _, err := signer.Verify(token, now.Add(DefaultTTL+time.Minute)); err != ErrTokenExpired {
		t.Error("an expired token was not reported as expired")
	}
}

// A signer with no secret must mint nothing: that is what makes "trace
// ingestion is not configured" mean "the endpoint accepts nothing", rather than
// "the endpoint accepts everything".
func TestDisabledSignerMintsAndVerifiesNothing(t *testing.T) {
	signer := &Signer{}
	if url := signer.IngestionURL("http://platform:8080", runUUID(t, "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20"), 1, time.Now()); url != "" {
		t.Errorf("disabled signer minted %q", url)
	}
	if _, err := signer.Verify("anything", time.Now()); err == nil {
		t.Error("disabled signer verified a token")
	}
}

// vendorKey is a fake OpenAI project key, assembled at run time rather than
// written as one literal. Repository hygiene, not obfuscation: the pre-push
// secret scan greps for the vendor prefixes, and a test fixture that trips it on
// every commit trains people to ignore the scan. What is under test is the
// pattern in mask.go, and it sees the same string either way.
var vendorKey = "sk-" + "proj-" + strings.Repeat("A", 28)

// TRACE-005. Each case is a shape that has actually turned up in a trace
// payload; the assertion is always the same two things - the secret is gone and
// the pointer to where it was is recorded.
func TestMaskerRedactsSecretsAndRecordsWhere(t *testing.T) {
	masker := &Masker{Known: []string{"a-long-known-ingestion-token-value"}}
	payload := `{
		"tool_name": "bash",
		"arguments": {
			"command": "curl -H 'Authorization: Bearer abcdefghijklmnopqrstuvwxyz' https://x",
			"env": ["OPENAI_API_KEY=` + vendorKey + `"],
			"echo": "a-long-known-ingestion-token-value"
		},
		"result_summary": "wrote output.xlsx, 1204 rows in",
		"duration_ms": 3412,
		"truncated": false
	}`

	result, err := masker.Mask(json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	masked := string(result.Payload)
	for _, secret := range []string{"abcdefghijklmnopqrstuvwxyz", vendorKey, "a-long-known-ingestion-token-value"} {
		if strings.Contains(masked, secret) {
			t.Errorf("secret %q survived masking: %s", secret, masked)
		}
	}
	if !strings.Contains(masked, Placeholder) {
		t.Errorf("nothing was redacted: %s", masked)
	}
	// Untouched values must stay untouched: a masker that eats the duration or
	// the result summary has broken the trace to no purpose.
	if !strings.Contains(masked, "wrote output.xlsx") || !strings.Contains(masked, "3412") {
		t.Errorf("masker altered non-secret values: %s", masked)
	}

	want := []string{"/arguments/command", "/arguments/echo", "/arguments/env/0"}
	if !reflect.DeepEqual(result.Fields, want) {
		t.Errorf("masked_fields = %v, want %v", result.Fields, want)
	}
}

// The placeholder must not preserve length or prefix: a partial mask leaks
// entropy about the secret (contract README §6d).
func TestMaskerReplacesWholeValueWithFixedPlaceholder(t *testing.T) {
	masker := &Masker{}
	short, err := masker.Mask(json.RawMessage(`{"m":"key sk-AAAAAAAAAAAAAAAAAAAAAAAA end"}`))
	if err != nil {
		t.Fatal(err)
	}
	long, err := masker.Mask(json.RawMessage(`{"m":"key sk-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA end"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(short.Payload) != string(long.Payload) {
		t.Errorf("masked output still reveals the secret's length:\n%s\n%s", short.Payload, long.Payload)
	}
}

// A "known" value short enough to occur in ordinary text must not be used as a
// search term, or the masker would redact the trace into uselessness.
func TestMaskerIgnoresShortKnownValues(t *testing.T) {
	masker := &Masker{Known: []string{"the"}}
	result, err := masker.Mask(json.RawMessage(`{"m":"the quick brown fox"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result.Payload), Placeholder) {
		t.Errorf("a short known value carpet-redacted ordinary text: %s", result.Payload)
	}
	if result.Fields == nil || len(result.Fields) != 0 {
		t.Fatalf("an unmasked payload must report [] rather than null, got %#v", result.Fields)
	}
}

// TRACE-008: a hole in a producer's sequence is a lost event, and the read side
// has to be able to say so rather than present a shorter timeline as complete.
func TestStreamHealthNamesTheMissingSequenceNumbers(t *testing.T) {
	rows := []gen.TraceEvent{
		traceRow(1, SourceSandbox, 1, false),
		traceRow(1, SourceSandbox, 2, false),
		// 3 never arrived.
		traceRow(1, SourceSandbox, 4, false),
		traceRow(1, SourceOrchestr, 1, true),
		// A second attempt is its own stream and starts again at 1.
		traceRow(2, SourceSandbox, 1, false),
	}
	health := streamHealth(rows)
	if len(health) != 3 {
		t.Fatalf("got %d streams, want 3 (two producers on attempt 1, one on attempt 2)", len(health))
	}
	// Streams are ordered (attempt, emitted_by), so attempt 1's orchestrator
	// stream comes before its sandbox stream.
	if health[0].EmittedBy != SourceOrchestr || health[0].LateEvents != 1 {
		t.Errorf("late event was not counted on its own stream: %+v", health[0])
	}
	if !reflect.DeepEqual(health[1].MissingSeq, []int64{3}) {
		t.Errorf("attempt 1 sandbox stream missing = %v, want [3]", health[1].MissingSeq)
	}
	if len(health[2].MissingSeq) != 0 {
		t.Errorf("attempt 2's own stream reported holes from attempt 1: %v", health[2].MissingSeq)
	}
}

func TestStreamHealthBoundsTheMissingSequenceSample(t *testing.T) {
	health := streamHealth([]gen.TraceEvent{
		traceRow(1, SourceSandbox, 1, false),
		traceRow(1, SourceSandbox, maxTraceSeq, false),
	})
	if len(health) != 1 {
		t.Fatalf("got %d streams, want 1", len(health))
	}
	if health[0].MissingCount != maxTraceSeq-2 {
		t.Fatalf("missing_count = %d, want %d", health[0].MissingCount, maxTraceSeq-2)
	}
	if len(health[0].MissingSeq) != 1_000 {
		t.Fatalf("missing sample has %d entries, want 1000", len(health[0].MissingSeq))
	}
}

func traceRow(attempt int32, source string, seq int64, late bool) gen.TraceEvent {
	return gen.TraceEvent{
		Attempt: attempt, Source: source, Seq: seq, Late: late,
		Payload: []byte("{}"), MaskedFields: []byte("[]"),
	}
}

// Envelope validation is the trust boundary: a sandbox is untrusted input, and
// each of these is a way it could poison the timeline.
func TestValidateRejectsMalformedEnvelopes(t *testing.T) {
	base := func() Event {
		return Event{
			SchemaVersion: "1.0", EventID: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
			RunID: "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20", Attempt: 1, Seq: 1,
			OccurredAt: time.Now(), EmittedBy: SourceSandbox, Type: TypeAgentOutput,
			Payload: json.RawMessage(`{"kind":"final","text":"done","truncated":false}`),
		}
	}
	good := base()
	if err := good.Validate(); err != nil {
		t.Fatalf("a well-formed event was rejected: %v", err)
	}

	cases := map[string]func(*Event){
		"seq below 1":         func(e *Event) { e.Seq = 0 },
		"seq above limit":     func(e *Event) { e.Seq = maxTraceSeq + 1 },
		"attempt below 1":     func(e *Event) { e.Attempt = 0 },
		"attempt above limit": func(e *Event) { e.Attempt = maxTraceAttempt + 1 },
		"unknown producer":    func(e *Event) { e.EmittedBy = "the_workload" },
		"unknown type":        func(e *Event) { e.Type = "shell_escape" },
		"unknown status":      func(e *Event) { s := "pwned"; e.Status = &s },
		"future major":        func(e *Event) { e.SchemaVersion = "2.0" },
		"no payload":          func(e *Event) { e.Payload = nil },
		"non-object payload":  func(e *Event) { e.Payload = json.RawMessage(`[]`) },
		"oversized payload":   func(e *Event) { e.Payload = json.RawMessage(strings.Repeat("x", maxPayloadBytes+1)) },
		"no occurrence time":  func(e *Event) { e.OccurredAt = time.Time{} },
	}
	for name, mutate := range cases {
		event := base()
		mutate(&event)
		if err := event.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A minor version bump is additive by the contract's own rule, so an event from
// a newer producer must still be stored rather than dropped.
func TestValidateAcceptsAdditiveMinorVersions(t *testing.T) {
	event := Event{
		SchemaVersion: "1.7", EventID: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
		RunID: "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20", Attempt: 1, Seq: 1,
		OccurredAt: time.Now(), EmittedBy: SourceSandbox, Type: TypeUsage,
		Payload: json.RawMessage(`{"model":"m","input_tokens":1,"output_tokens":1}`),
	}
	if err := event.Validate(); err != nil {
		t.Errorf("a 1.x event was rejected: %v", err)
	}
}

// TRACE-006 must render a cost the gateway never reported as unreported, not as
// zero: showing 0 would tell the user their run was free (contract README §5).
func TestSummaryKeepsUnreportedCostNil(t *testing.T) {
	var summary Summary
	summary.fold(gen.TraceEvent{
		EventType: TypeUsage, MaskedFields: []byte("[]"),
		Payload: []byte(`{"scope":"run_total","model":"gpt-5-mini","input_tokens":27042,"output_tokens":1180,"cost_usd":null}`),
	})
	if summary.Usage == nil {
		t.Fatal("usage event did not reach the summary")
	}
	if summary.Usage.CostUSD != nil {
		t.Errorf("cost_usd = %v, want nil (unreported)", *summary.Usage.CostUSD)
	}
	if summary.Usage.InputTokens != 27042 {
		t.Errorf("input tokens = %d, want 27042", summary.Usage.InputTokens)
	}
}
