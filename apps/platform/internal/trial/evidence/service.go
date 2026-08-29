package trace

// Ingestion (TRACE-002~005, TRACE-008) and the two read shapes (TRACE-006/007).
//
// Everything arriving here crossed the trust boundary of ADR-001: `sandbox`
// events are untrusted input. The order is fixed and is the whole security
// argument of this package - verify the token, resolve the scope from the
// platform's own data, validate the envelope, mask, then store. Nothing reaches
// the database before the masker has run (iron rule 11, enforced again by
// 0019's CHECK).

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// ErrNotFound is "no such run, or not yours". Existence is private (WS-006).
var ErrNotFound = errors.New("run not found")

// Service owns trace ingestion and reading.
type Service struct {
	Pool               *pgxpool.Pool
	Signer             *Signer
	ReadRunState       func(context.Context, pgtype.UUID, pgtype.UUID) (RunState, bool, error)
	ReadIngestRunState func(context.Context, pgtype.UUID) (IngestRunState, bool, error)
	ReadRunTransitions func(context.Context, pgtype.UUID, pgtype.UUID) ([]RunTransition, error)
}

// RunState is the small Run-owned fact needed by Trace's read views.
type RunState struct {
	Status       string
	StatusReason *string
}

// IngestRunState supplies trusted scope and lifecycle state for trace ingestion.
type IngestRunState struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	Status      string
	FinishedAt  *time.Time
}

// RunTransition is one authoritative lifecycle step in occurrence order.
type RunTransition struct {
	ToStatus string
	Reason   *string
}

var errRunReaderNotConfigured = errors.New("trace run reader is not configured")
var errPersistenceNotConfigured = errors.New("trace persistence is not configured")

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

// IngestReport is what one batch did. Duplicates are counted, not treated as
// errors: at-least-once delivery makes them ordinary, and a producer that
// retries after an uncertain response is doing the right thing.
type IngestReport struct {
	Received  int `json:"received"`
	Stored    int `json:"stored"`
	Duplicate int `json:"duplicate"`
	Rejected  int `json:"rejected"`
	// Reasons carries one line per rejected event, so a producer can fix its
	// output without the platform having to log user content.
	Reasons []string `json:"reasons,omitempty"`
}

// Ingest stores one batch for the run the grant names.
//
// The grant, not the body, decides which run and which attempt these events
// belong to: a sandbox that puts another run's id in its payload is describing
// a run it was never given a token for, and that event is rejected rather than
// stored under the wrong timeline.
func (s *Service) Ingest(ctx context.Context, grant Grant, token string, events []Event) (IngestReport, error) {
	if s.ReadIngestRunState == nil {
		return IngestReport{}, errRunReaderNotConfigured
	}
	run, found, err := s.ReadIngestRunState(ctx, grant.RunID)
	if !found && err == nil {
		return IngestReport{}, ErrNotFound
	}
	if err != nil {
		return IngestReport{}, err
	}

	// TRACE-008: events that arrive after the run reached a terminal state are
	// accepted and flagged, never dropped. A sandbox pushes its final batch as
	// it shuts down, which is frequently after the platform has already settled
	// the run, and discarding exactly the tail of a failed run would remove the
	// part a user most needs (RUN-004).
	late := terminal(gen.RunStatus(run.Status))

	// The ingestion token is a secret the workload can read (it is in its own
	// environment), so it is the one known value the masker always has.
	masker := &Masker{Known: []string{token}}

	report := IngestReport{Received: len(events)}
	for i := range events {
		event := &events[i]
		if err := s.ingestOne(ctx, run, grant, masker, event, late); err != nil {
			switch {
			case errors.Is(err, ErrInvalid):
				report.Rejected++
				report.Reasons = append(report.Reasons, err.Error())
				metrics.TraceEvents.WithLabelValues(sourceLabel(event.EmittedBy), "rejected").Inc()
			case errors.Is(err, errDuplicate):
				report.Duplicate++
				metrics.TraceEvents.WithLabelValues(sourceLabel(event.EmittedBy), "duplicate").Inc()
			default:
				return report, err
			}
			continue
		}
		report.Stored++
		metrics.TraceEvents.WithLabelValues(sourceLabel(event.EmittedBy), "stored").Inc()
	}
	return report, nil
}

var errDuplicate = errors.New("event already stored")

func (s *Service) ingestOne(
	ctx context.Context, run IngestRunState, grant Grant,
	masker *Masker, event *Event, late bool,
) error {
	if err := event.Validate(); err != nil {
		return err
	}
	// This capability is issued to the execution plane. Letting its untrusted
	// workload choose another producer would let it forge control-plane errors,
	// usage and outputs in the audit timeline.
	if event.EmittedBy != SourceSandbox {
		return fmt.Errorf("%w: ingestion token only permits sandbox events", ErrInvalid)
	}
	// The token binds the batch to one attempt. An event claiming a different
	// run or attempt is not a token this producer holds.
	if event.RunID != pgconv.UUIDString(grant.RunID) {
		return fmt.Errorf("%w: run_id does not match the ingestion token", ErrInvalid)
	}
	if event.Attempt != grant.Attempt {
		return fmt.Errorf("%w: attempt does not match the ingestion token", ErrInvalid)
	}

	masked, err := masker.Mask(event.Payload)
	if err != nil {
		return fmt.Errorf("%w: payload is not a JSON object", ErrInvalid)
	}
	metrics.TraceMaskedFields.Add(float64(len(masked.Fields)))

	var eventID pgtype.UUID
	if err := eventID.Scan(event.EventID); err != nil {
		return fmt.Errorf("%w: event_id must be a UUID", ErrInvalid)
	}
	fields, err := json.Marshal(masked.Fields)
	if err != nil {
		return err
	}

	// Clamped, because OccurredAt is the sandbox's clock and the sandbox is
	// untrusted (event.go's Validate only checks it is non-zero). NFR-004's 3
	// second gap series feeds alerts.yml, and an unclamped subtraction lets one
	// workload with a drifted - or chosen - clock write any value it likes into
	// it, including a negative one. The bound is the trace token's own lifetime:
	// beyond that the event could not have been ingested at all, so a larger
	// reading is a clock artefact rather than a lag measurement.
	//
	// The same limitation is argued and accepted for CountTraceMaskingInWindow in
	// db/queries/trace.sql; this is the same producer timestamp, bounded.
	lag := time.Since(event.OccurredAt)
	lag = min(max(lag, 0), DefaultTTL)
	metrics.TraceIngestLag.Observe(lag.Seconds())

	rows, err := s.queries().InsertTraceEvent(ctx, gen.InsertTraceEventParams{
		EventID:     eventID,
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Attempt:     int32(grant.Attempt),
		Seq:         event.Seq,
		OccurredAt:  pgtype.Timestamptz{Time: event.OccurredAt, Valid: true},
		EventType:   event.Type,
		Source:      event.EmittedBy,
		Status:      event.Status,
		// Stored as the version the producer declared, so a later translation
		// layer knows what it is reading (contract README §9).
		SchemaVersion: event.SchemaVersion,
		// Always true: the masker above is the only path to this insert, and
		// 0019's CHECK refuses anything else.
		Masked:       true,
		MaskedFields: fields,
		Payload:      masked.Payload,
		Late:         late,
	})
	if err != nil {
		// 23505 here means this stream position is already held by a *different*
		// event: 0032's guard raises it for a repeated (run, attempt, source,
		// seq), and 0019's index for a repeated event_id that somehow outran
		// 0031. An ordinary at-least-once redelivery never reaches either -
		// 0031's dedupe trigger consumes an equal event_id first and returns no
		// rows - so this is a producer contradicting itself, one unacceptable
		// event and not a failed batch. It is refused the way every other
		// unacceptable event is: counted, named and stepped over. Aborting the
		// batch instead would 500 the push, lose every event queued behind this
		// one (TRACE-008), and hand the producer a retry that can never
		// converge, because the events ahead of it are already committed.
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return fmt.Errorf("%w: event_id %s conflicts with an event already stored at seq %d of this stream",
				ErrInvalid, event.EventID, event.Seq)
		}
		return err
	}
	if rows == 0 {
		return errDuplicate
	}
	return nil
}

// RecordOrchestratorEvent writes one event from the control plane's own side,
// inside the caller's transaction so it commits with the state change it
// describes (iron rule 9).
//
// The seq is allocated by reading the stream's current maximum in that same
// transaction. That is a serialization point, and deliberately so for this
// producer only: the orchestrator writes a handful of events per run, and a
// gapless sequence is what makes a lost event detectable at all (TRACE-001 §7).
func RecordOrchestratorEvent(
	ctx context.Context, tx pgx.Tx, workspaceID, runID pgtype.UUID,
	attempt int, eventType, status string, payload any,
) error {
	if tx == nil {
		return errPersistenceNotConfigured
	}
	q := gen.New(tx)
	if err := q.LockTraceIngestRun(ctx, runID); err != nil {
		return err
	}
	seq, err := q.NextTraceSeq(ctx, gen.NextTraceSeqParams{
		RunID: runID, Attempt: int32(attempt), Source: SourceOrchestr,
	})
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// The control plane's own text still goes through the masker: a provider
	// error message routinely quotes a URL that carries a signature (the schema
	// marks error.message secret_bearing for exactly this reason), and the
	// query-parameter signatures in secretPatterns are the shape that catches.
	//
	// Patterns only, and that is a real limit rather than an oversight: no
	// per-run credential is in scope inside another context's transaction - a
	// run id cannot re-derive its own ingestion token, whose expiry is stamped
	// at mint time - so Known is empty here, and a credential carried as a bare
	// path segment is not redacted, because no pattern is positional. [Service.Ingest]
	// is the one caller that holds that value, and it does pass it as Known.
	masker := &Masker{}
	masked, err := masker.Mask(encoded)
	if err != nil {
		return err
	}
	fields, err := json.Marshal(masked.Fields)
	if err != nil {
		return err
	}
	metrics.TraceMaskedFields.Add(float64(len(masked.Fields)))

	var eventID pgtype.UUID
	if err := eventID.Scan(newUUID()); err != nil {
		return err
	}
	var statusPtr *string
	if status != "" {
		statusPtr = &status
	}
	rows, err := q.InsertTraceEvent(ctx, gen.InsertTraceEventParams{
		EventID: eventID, WorkspaceID: workspaceID, RunID: runID,
		Attempt: int32(attempt), Seq: seq,
		OccurredAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		EventType:     eventType,
		Source:        SourceOrchestr,
		Status:        statusPtr,
		SchemaVersion: schemaVersionFor(eventType),
		Masked:        true,
		MaskedFields:  fields,
		Payload:       masked.Payload,
	})
	if err != nil {
		return err
	}
	if rows > 0 {
		metrics.TraceEvents.WithLabelValues(SourceOrchestr, "stored").Inc()
	}
	return nil
}

// --- reading -----------------------------------------------------------------

// StreamHealth is one (attempt, emitted_by) stream's completeness. It is what
// makes "缺失或延遲事件可被識別" (TRACE-001) an answer rather than a hope: seq is
// gapless by contract, so a hole is a lost event and nothing else.
type StreamHealth struct {
	Attempt      int     `json:"attempt"`
	EmittedBy    string  `json:"emitted_by"`
	Received     int     `json:"received"`
	HighestSeq   int64   `json:"highest_seq"`
	MissingCount int64   `json:"missing_count"`
	MissingSeq   []int64 `json:"missing_seq,omitempty"`
	LateEvents   int     `json:"late_events"`
}

// AdvancedView is TRACE-007: the masked raw events in their reconstructed order,
// plus an explicit statement of what is missing. The UI is required to show
// incompleteness rather than paper over it (ADR-009), which it can only do if
// this says so.
type AdvancedView struct {
	RunID string `json:"run_id"`
	// Complete is false when any stream has a hole. It is not "the run finished";
	// a finished run with a gap is incomplete, and a running one with no gap yet
	// is complete so far.
	Complete  bool           `json:"complete"`
	Streams   []StreamHealth `json:"streams"`
	Events    []EventView    `json:"events"`
	NextAfter int64          `json:"next_after"`
	HasMore   bool           `json:"has_more"`
	// Internal to the evaluator; user-facing pages are already explicitly paged.
	EvaluationTruncated bool `json:"-"`
}

// EventView is one stored event as the API returns it. Payload is already
// masked - there is no unmasked form to ask for.
type EventView struct {
	EventID      string          `json:"event_id"`
	Attempt      int             `json:"attempt"`
	Seq          int64           `json:"seq"`
	OccurredAt   string          `json:"occurred_at"`
	EmittedBy    string          `json:"emitted_by"`
	Type         string          `json:"type"`
	Status       string          `json:"status,omitempty"`
	Late         bool            `json:"late,omitempty"`
	MaskedFields []string        `json:"masked_fields"`
	Payload      json.RawMessage `json:"payload"`

	// The same instant as OccurredAt, kept unformatted for sortEventViews.
	// Unexported, so encoding/json leaves the wire contract alone.
	//
	// It exists because RFC3339Nano trims trailing zeros from the fraction,
	// which makes the formatted strings sort differently from the instants they
	// name whenever one fraction is a prefix of another: ".123Z" vs ".1234Z"
	// compares 'Z' (0x5A) against '4' (0x34) at the fifth character and puts
	// .1234 first. A whole second is worse -- the fraction is dropped entirely,
	// so "45Z" vs "45.5Z" compares 'Z' against '.' and puts .5 before it.
	// TestAdvancedViewNamesMissingEventsAndRefusesToLookComplete caught it as
	// "events came back out of order: 3 then 1", which is a trace shown to a
	// user in an order the run did not happen in.
	occurredAt time.Time
}

// Summary is TRACE-006: the general mode. It answers "what happened, in
// language a non-technical user reads" without them opening a single raw event.
//
// Run status is NOT derived from run_lifecycle events. The only authority on run
// state is the runs table (iron rule 5), so it is read from there and the trace
// contributes only what the trace knows.
type Summary struct {
	RunID        string          `json:"run_id"`
	Status       string          `json:"status"`
	StatusReason string          `json:"status_reason,omitempty"`
	Complete     bool            `json:"complete"`
	Skills       []SkillUse      `json:"skills"`
	SkillsTotal  int             `json:"skills_total"`
	ResourceRead int             `json:"resources_read"`
	ToolCalls    ToolCallSummary `json:"tool_calls"`
	Errors       []ErrorSummary  `json:"errors"`
	ErrorsTotal  int             `json:"errors_total"`
	Truncated    bool            `json:"summary_truncated"`
	FinalOutput  string          `json:"final_output,omitempty"`
	Usage        *UsageSummary   `json:"usage,omitempty"`
	Steps        []string        `json:"steps"`
	// LastEventAt is when this run last produced anything (設計系統 §2.12). Empty
	// means no event has arrived yet — a real state for a run still being
	// provisioned, and the caller words it rather than rendering a blank or, far
	// worse, "0 seconds ago". A watching screen needs it because a changing
	// counter alone cannot distinguish working from wedged: the counter simply
	// stops, and stopping looks identical to finishing.
	LastEventAt string `json:"last_event_at,omitempty"`
}

type SkillUse struct {
	Name     string `json:"name"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type ToolCallSummary struct {
	Total       int    `json:"total"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	TotalMS     int64  `json:"total_duration_ms"`
	SlowestMS   int64  `json:"slowest_duration_ms"`
	SlowestName string `json:"slowest_tool,omitempty"`
}

type ErrorSummary struct {
	Category string `json:"category"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// UsageSummary keeps the contract's null semantics: a nil cost means the gateway
// did not report one, and the UI must render that as "unreported" rather than as
// zero (contract README §5).
type UsageSummary struct {
	Model        string   `json:"model,omitempty"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
	CostSource   string   `json:"cost_source,omitempty"`
}

const tracePageSize = int32(1_000)

// Advanced returns the raw (masked) events plus stream health.
func (s *Service) Advanced(ctx context.Context, workspaceID, runID pgtype.UUID, after int64) (AdvancedView, error) {
	if _, err := s.runState(ctx, workspaceID, runID); err != nil {
		return AdvancedView{}, err
	}
	health, err := s.traceStreamHealth(ctx, workspaceID, runID)
	if err != nil {
		return AdvancedView{}, err
	}
	rows, err := s.queries().ListTraceEventsAfter(ctx, gen.ListTraceEventsAfterParams{
		RunID: runID, WorkspaceID: workspaceID, AfterIngestSeq: after, PageLimit: tracePageSize + 1,
	})
	if err != nil {
		return AdvancedView{}, err
	}
	view := AdvancedView{RunID: pgconv.UUIDString(runID), Streams: health, Complete: true, NextAfter: after}
	if len(rows) > int(tracePageSize) {
		view.HasMore = true
		rows = rows[:tracePageSize]
	}
	if len(rows) > 0 {
		view.NextAfter = rows[len(rows)-1].IngestSeq
	}
	for _, stream := range view.Streams {
		if stream.MissingCount > 0 {
			view.Complete = false
		}
	}
	view.Events = eventViews(rows)
	sortEventViews(view.Events)
	return view, nil
}

func eventViews(rows []gen.TraceEvent) []EventView {
	events := make([]EventView, 0, len(rows))
	for _, row := range rows {
		var fields []string
		_ = json.Unmarshal(row.MaskedFields, &fields)
		if fields == nil {
			fields = []string{}
		}
		var status string
		if row.Status != nil {
			status = *row.Status
		}
		events = append(events, EventView{
			EventID: pgconv.UUIDString(row.EventID), Attempt: int(row.Attempt), Seq: row.Seq,
			OccurredAt: row.OccurredAt.Time.UTC().Format(time.RFC3339Nano),
			occurredAt: row.OccurredAt.Time.UTC(),
			EmittedBy:  row.Source, Type: row.EventType, Status: status, Late: row.Late,
			MaskedFields: fields, Payload: json.RawMessage(row.Payload),
		})
	}
	return events
}

func evaluationEventViews(rows []gen.ListEvaluationTraceEventsRow) []EventView {
	events := make([]EventView, 0, len(rows))
	for _, row := range rows {
		var fields []string
		_ = json.Unmarshal(row.MaskedFields, &fields)
		if fields == nil {
			fields = []string{}
		}
		var status string
		if row.Status != nil {
			status = *row.Status
		}
		events = append(events, EventView{
			EventID: pgconv.UUIDString(row.EventID), Attempt: int(row.Attempt), Seq: row.Seq,
			OccurredAt: row.OccurredAt.Time.UTC().Format(time.RFC3339Nano),
			occurredAt: row.OccurredAt.Time.UTC(),
			EmittedBy:  row.Source, Type: row.EventType, Status: status, Late: row.Late,
			MaskedFields: fields, Payload: json.RawMessage(row.Payload),
		})
	}
	return events
}

func sortEventViews(events []EventView) {
	sort.Slice(events, func(i, j int) bool {
		// The instant, never the formatted string -- see EventView.occurredAt.
		if !events[i].occurredAt.Equal(events[j].occurredAt) {
			return events[i].occurredAt.Before(events[j].occurredAt)
		}
		if events[i].EmittedBy != events[j].EmittedBy {
			return events[i].EmittedBy < events[j].EmittedBy
		}
		if events[i].Attempt != events[j].Attempt {
			return events[i].Attempt < events[j].Attempt
		}
		return events[i].Seq < events[j].Seq
	})
}

// AdvancedAll is for the one-shot evaluator at run completion. User-facing
// polling must use Advanced with its cursor so it never retransmits the trace.
func (s *Service) AdvancedAll(ctx context.Context, workspaceID, runID pgtype.UUID) (AdvancedView, error) {
	if _, err := s.runState(ctx, workspaceID, runID); err != nil {
		return AdvancedView{}, err
	}
	health, err := s.traceStreamHealth(ctx, workspaceID, runID)
	if err != nil {
		return AdvancedView{}, err
	}
	all := AdvancedView{RunID: pgconv.UUIDString(runID), Complete: true, Streams: health, Events: []EventView{}}
	for _, stream := range health {
		if stream.MissingCount > 0 {
			all.Complete = false
		}
	}
	rows, err := s.queries().ListEvaluationTraceEvents(ctx, gen.ListEvaluationTraceEventsParams{
		EvaluationRunID: runID, EvaluationWorkspaceID: workspaceID,
	})
	if err != nil {
		return AdvancedView{}, err
	}
	all.Events = evaluationEventViews(rows)
	if len(rows) > 0 && rows[0].EvaluationTruncated {
		all.EvaluationTruncated = true
		all.Complete = false
	}
	return all, nil
}

// LiveEvents answers which of the cited events still exist, scoped to one run in
// one workspace (ADR-026 decision 2).
//
// Exported because internal/eval has to re-answer availability at read time
// rather than trust a flag stored when the evidence was first cited: retention
// here is a partition drop, so a reference stored as available goes on claiming
// the original is there long after the month it lived in was dropped. That is
// this package's failure mode, so answering it is this package's job - eval had
// been asking the table directly, which froze this schema against a caller this
// package has no way to find (ADR-035 G 組).
//
// It answers existence and nothing else. Which events a report cites, and what a
// missing one means to a reader - the excerpt stays, labelled, never blanked
// (ADR-009) - remain eval's decisions.
//
// Reads through this Service's own pool, outside any transaction, exactly as
// [Service.General] does: it is a report read. The returned slice is the live
// subset of eventIDs, in no promised order; the caller matches by id.
func (s *Service) LiveEvents(
	ctx context.Context, workspaceID, runID pgtype.UUID, eventIDs []pgtype.UUID,
) ([]pgtype.UUID, error) {
	return s.queries().FindLiveTraceEvents(ctx, gen.FindLiveTraceEventsParams{
		WorkspaceID: workspaceID, RunID: runID, EventIds: eventIDs,
	})
}

// MaskingActivity counts stored events and redacted fields over two adjacent
// windows: recent starts the near one, since starts the pair.
//
// Exported for internal/run's supervisor, which reads it as 02:SEC-010's
// `TraceMaskingStopped` P1 criterion. The split is why it is exported rather than
// left as a query that package calls: what the masker did is this package's fact,
// and under 0019's CHECK (masked) the only reading of it that detects anything is
// the two-window one this query encodes - a broken masker produces rows that
// claim to be masked, not unmasked rows. How much traffic counts as evidence, how
// long the condition must hold, and what to do about it stay in run/halt.go,
// which is why the window sizes are the caller's arguments and not constants here.
//
// Deployment-wide and therefore not workspace scoped: a masker that stopped
// stopped for everyone, and iron rule 3 governs user data reads, which this is
// not - it returns three counts and no row.
type MaskingActivityFacts struct {
	RecentEvents  int64
	EarlierEvents int64
	MaskedFields  int64
}

func (s *Service) MaskingActivity(ctx context.Context, recent, since time.Time) (MaskingActivityFacts, error) {
	if s == nil || s.Pool == nil {
		return MaskingActivityFacts{}, errPersistenceNotConfigured
	}
	row, err := s.queries().CountTraceMaskingInWindow(ctx, gen.CountTraceMaskingInWindowParams{
		Recent: pgtype.Timestamptz{Time: recent, Valid: true},
		Since:  pgtype.Timestamptz{Time: since, Valid: true},
	})
	return MaskingActivityFacts{
		RecentEvents: row.RecentEvents, EarlierEvents: row.EarlierEvents, MaskedFields: row.MaskedFields,
	}, err
}

// General returns the human-readable progress summary.
func (s *Service) General(ctx context.Context, workspaceID, runID pgtype.UUID) (Summary, error) {
	if s.ReadRunTransitions == nil {
		return Summary{}, errRunReaderNotConfigured
	}
	run, err := s.runState(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}
	folded, err := s.queries().GetTraceGeneralFold(ctx, gen.GetTraceGeneralFoldParams{
		FoldRunID: runID, FoldWorkspaceID: workspaceID,
	})
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{
		RunID:    pgconv.UUIDString(runID),
		Status:   run.Status,
		Complete: true,
		Skills:   []SkillUse{},
		Errors:   []ErrorSummary{},
		Steps:    []string{},
	}
	if run.StatusReason != nil {
		summary.StatusReason = *run.StatusReason
	}
	if err := json.Unmarshal(folded, &summary); err != nil {
		return Summary{}, err
	}
	summary.Truncated = summary.SkillsTotal > len(summary.Skills) || summary.ErrorsTotal > len(summary.Errors)
	health, err := s.traceStreamHealth(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}
	for _, stream := range health {
		if stream.MissingCount > 0 {
			summary.Complete = false
		}
	}

	// The progress steps come from the authoritative transition history, not
	// from run_lifecycle trace events (iron rule 5).
	transitions, err := s.ReadRunTransitions(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}
	for _, t := range transitions {
		step := t.ToStatus
		if t.Reason != nil && *t.Reason != "" {
			step += ": " + *t.Reason
		}
		summary.Steps = append(summary.Steps, step)
	}

	return summary, nil
}

func (s *Service) runState(ctx context.Context, workspaceID, runID pgtype.UUID) (RunState, error) {
	if s.ReadRunState == nil {
		return RunState{}, errRunReaderNotConfigured
	}
	run, found, err := s.ReadRunState(ctx, workspaceID, runID)
	if err != nil {
		return RunState{}, err
	}
	if !found {
		return RunState{}, ErrNotFound
	}
	return run, nil
}

func (s *Service) traceStreamHealth(ctx context.Context, workspaceID, runID pgtype.UUID) ([]StreamHealth, error) {
	rows, err := s.queries().GetTraceStreamHealth(ctx, gen.GetTraceStreamHealthParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]StreamHealth, 0, len(rows))
	for _, row := range rows {
		out = append(out, StreamHealth{
			Attempt: int(row.Attempt), EmittedBy: row.Source, Received: int(row.Received),
			HighestSeq: row.HighestSeq, MissingCount: row.MissingCount,
			MissingSeq: row.MissingSeq, LateEvents: int(row.LateEvents),
		})
	}
	return out, nil
}

// newUUID is a version 4 UUID for an orchestrator-produced event_id. The
// producer assigns it (that is what makes it an idempotency key), so it cannot
// be left to the database's default.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on any supported platform; if it ever did,
		// a duplicate-looking event_id would silently drop events, so stop here.
		panic("crypto/rand: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func terminal(status gen.RunStatus) bool {
	switch status {
	case gen.RunStatusSucceeded, gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut:
		return true
	default:
		return false
	}
}

// sourceLabel keeps metric cardinality bounded by the contract's producer enum:
// a sandbox that invents a producer name must not be able to create a series.
func sourceLabel(source string) string {
	if sources[source] {
		return source
	}
	return "unknown"
}
