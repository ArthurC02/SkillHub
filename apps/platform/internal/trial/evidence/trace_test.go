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

func TestDisabledSignerMintsAndVerifiesNothing(t *testing.T) {
	signer := &Signer{}
	if url := signer.IngestionURL("http://platform:8080", runUUID(t, "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20"), 1, time.Now()); url != "" {
		t.Errorf("disabled signer minted %q", url)
	}
	if _, err := signer.Verify("anything", time.Now()); err == nil {
		t.Error("disabled signer verified a token")
	}
}

var vendorKey = "sk-" + "proj-" + strings.Repeat("A", 28)

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

	if !strings.Contains(masked, "wrote output.xlsx") || !strings.Contains(masked, "3412") {
		t.Errorf("masker altered non-secret values: %s", masked)
	}

	want := []string{"/arguments/command", "/arguments/echo", "/arguments/env/0"}
	if !reflect.DeepEqual(result.Fields, want) {
		t.Errorf("masked_fields = %v, want %v", result.Fields, want)
	}
}

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

func TestValidateAcceptsTheSeqAndAttemptCeilingsThemselves(t *testing.T) {
	base := func() Event {
		return Event{
			SchemaVersion: "1.0", EventID: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
			RunID: "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20", Attempt: 1, Seq: 1,
			OccurredAt: time.Now(), EmittedBy: SourceSandbox, Type: TypeAgentOutput,
			Payload: json.RawMessage(`{"kind":"final","text":"done","truncated":false}`),
		}
	}
	cases := map[string]func(*Event){
		"seq at the ceiling":     func(e *Event) { e.Seq = maxTraceSeq },
		"attempt at the ceiling": func(e *Event) { e.Attempt = maxTraceAttempt },
	}
	for name, mutate := range cases {
		event := base()
		mutate(&event)
		if err := event.Validate(); err != nil {
			t.Errorf("%s was rejected: %v", name, err)
		}
	}
}

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

func TestEventsSortByTheInstantAndNotItsFormattedString(t *testing.T) {
	base := time.Date(2026, 8, 26, 12, 0, 45, 0, time.UTC)

	pairs := []struct {
		name           string
		earlier, later time.Time
	}{

		{"a fraction that is a prefix of the other", base.Add(123 * time.Millisecond), base.Add(123400 * time.Microsecond)},

		{"a whole second against a fraction of it", base, base.Add(500 * time.Millisecond)},
	}
	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			view := func(at time.Time, seq int64) EventView {
				return EventView{Seq: seq, OccurredAt: at.Format(time.RFC3339Nano), occurredAt: at, EmittedBy: "sandbox"}
			}

			if p.earlier.Format(time.RFC3339Nano) <= p.later.Format(time.RFC3339Nano) {
				t.Fatalf("fixture no longer disagrees: %q vs %q",
					p.earlier.Format(time.RFC3339Nano), p.later.Format(time.RFC3339Nano))
			}
			events := []EventView{view(p.later, 3), view(p.earlier, 1)}
			sortEventViews(events)
			if events[0].Seq != 1 || events[1].Seq != 3 {
				t.Errorf("got seq %d then %d, want the earlier event first (%q before %q)",
					events[0].Seq, events[1].Seq,
					p.earlier.Format(time.RFC3339Nano), p.later.Format(time.RFC3339Nano))
			}
		})
	}
}
