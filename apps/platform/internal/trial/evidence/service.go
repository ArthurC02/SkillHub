package trace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

var ErrNotFound = errors.New("run not found")

type Service struct {
	Pool               *pgxpool.Pool
	Signer             *Signer
	ReadRunState       func(ctx context.Context, workspaceID, runID pgtype.UUID) (RunState, bool, error)
	ReadIngestRunState func(context.Context, pgtype.UUID) (IngestRunState, bool, error)
	ReadRunTransitions func(ctx context.Context, workspaceID, runID pgtype.UUID) ([]RunTransition, error)

	Credits func(usd float64) (credits int64, ok bool)
}

type RunState struct {
	Status       string
	StatusReason *string
	Started      bool
}

type IngestRunState struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	Status      string
	FinishedAt  *time.Time
}

type RunTransition struct {
	ToStatus string
	Reason   *string
}

var errRunReaderNotConfigured = errors.New("trace run reader is not configured")
var errPersistenceNotConfigured = errors.New("trace persistence is not configured")

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

func nothingWasCollected(run RunState, health []StreamHealth) bool {
	if !run.Started {
		return false
	}
	return !slices.ContainsFunc(health, func(h StreamHealth) bool { return h.EmittedBy == SourceSandbox })
}

type IngestReport struct {
	Received  int `json:"received"`
	Stored    int `json:"stored"`
	Duplicate int `json:"duplicate"`
	Rejected  int `json:"rejected"`

	Reasons []string `json:"reasons,omitempty"`
}

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

	masker := &Masker{Known: []string{token}}

	report := IngestReport{Received: len(events)}
	accepted := make([]gen.InsertTraceEventParams, 0, len(events))
	for i := range events {
		event := &events[i]
		stored, err := prepareEvent(run, grant, masker, event)
		if errors.Is(err, ErrInvalid) {
			report.countRejected(event.EmittedBy, err)
			continue
		}
		if err != nil {
			return report, err
		}
		accepted = append(accepted, stored)
	}
	if s.storeTogether(ctx, &report, accepted) {
		return report, nil
	}
	return report, s.storeOneByOne(ctx, &report, accepted)
}

func (r *IngestReport) countRejected(source string, err error) {
	r.Rejected++
	r.Reasons = append(r.Reasons, err.Error())
	metrics.TraceEvents.WithLabelValues(sourceLabel(source), "rejected").Inc()
}

func (r *IngestReport) countDuplicate(source string) {
	r.Duplicate++
	metrics.TraceEvents.WithLabelValues(sourceLabel(source), "duplicate").Inc()
}

func (r *IngestReport) countStored(source string) {
	r.Stored++
	metrics.TraceEvents.WithLabelValues(sourceLabel(source), "stored").Inc()
}

// storeTogether inserts every event in one statement, so one refused event
// keeps all of them out and it reports false. An event whose id is already
// stored, or repeats an earlier one in the batch, is skipped by the database.
func (s *Service) storeTogether(ctx context.Context, report *IngestReport, accepted []gen.InsertTraceEventParams) bool {
	if len(accepted) == 0 {
		return true
	}
	storedIDs, err := s.queries().InsertTraceEvents(ctx, columnsOf(accepted))
	if err != nil {
		return false
	}
	unclaimed := make(map[pgtype.UUID]int, len(storedIDs))
	for _, id := range storedIDs {
		unclaimed[id]++
	}
	for _, e := range accepted {
		if unclaimed[e.EventID] > 0 {
			unclaimed[e.EventID]--
			report.countStored(e.Source)
		} else {
			report.countDuplicate(e.Source)
		}
	}
	return true
}

func columnsOf(events []gen.InsertTraceEventParams) gen.InsertTraceEventsParams {
	n := len(events)
	c := gen.InsertTraceEventsParams{
		EventIds: make([]pgtype.UUID, n), WorkspaceIds: make([]pgtype.UUID, n), RunIds: make([]pgtype.UUID, n),
		Attempts: make([]int32, n), Seqs: make([]int64, n), OccurredAts: make([]pgtype.Timestamptz, n),
		EventTypes: make([]string, n), Sources: make([]string, n), Statuses: make([]string, n),
		SchemaVersions: make([]string, n), Masked: make([]bool, n), MaskedFields: make([][]byte, n), Payloads: make([][]byte, n),
	}
	for i, e := range events {
		c.EventIds[i], c.WorkspaceIds[i], c.RunIds[i] = e.EventID, e.WorkspaceID, e.RunID
		c.Attempts[i], c.Seqs[i], c.OccurredAts[i] = e.Attempt, e.Seq, e.OccurredAt
		c.EventTypes[i], c.Sources[i], c.SchemaVersions[i] = e.EventType, e.Source, e.SchemaVersion
		c.Masked[i], c.MaskedFields[i], c.Payloads[i] = e.Masked, e.MaskedFields, e.Payload
		if e.Status != nil {
			c.Statuses[i] = *e.Status
		}
	}
	return c
}

func (s *Service) storeOneByOne(ctx context.Context, report *IngestReport, accepted []gen.InsertTraceEventParams) error {
	for _, e := range accepted {
		err := s.storeOne(ctx, e)
		switch {
		case err == nil:
			report.countStored(e.Source)
		case errors.Is(err, ErrInvalid):
			report.countRejected(e.Source, err)
		case errors.Is(err, errDuplicate):
			report.countDuplicate(e.Source)
		default:
			return err
		}
	}
	return nil
}

var errDuplicate = errors.New("event already stored")

func prepareEvent(run IngestRunState, grant Grant, masker *Masker, event *Event) (gen.InsertTraceEventParams, error) {
	if err := event.Validate(); err != nil {
		return gen.InsertTraceEventParams{}, err
	}

	if event.EmittedBy != SourceSandbox {
		return gen.InsertTraceEventParams{}, fmt.Errorf("%w: ingestion token only permits sandbox events", ErrInvalid)
	}

	if event.RunID != pgconv.UUIDString(grant.RunID) {
		return gen.InsertTraceEventParams{}, fmt.Errorf("%w: run_id does not match the ingestion token", ErrInvalid)
	}
	if event.Attempt != grant.Attempt {
		return gen.InsertTraceEventParams{}, fmt.Errorf("%w: attempt does not match the ingestion token", ErrInvalid)
	}

	masked, err := masker.Mask(event.Payload)
	if errors.Is(err, errPayloadHoldsNUL) {
		return gen.InsertTraceEventParams{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err != nil {
		return gen.InsertTraceEventParams{}, fmt.Errorf("%w: payload is not a JSON object", ErrInvalid)
	}

	var eventID pgtype.UUID
	if err := eventID.Scan(event.EventID); err != nil {
		return gen.InsertTraceEventParams{}, fmt.Errorf("%w: event_id must be a UUID", ErrInvalid)
	}

	lag := time.Since(event.OccurredAt)
	lag = min(max(lag, 0), DefaultTTL)
	metrics.TraceIngestLag.Observe(lag.Seconds())

	return masked.stored(gen.InsertTraceEventParams{
		EventID:     eventID,
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Attempt:     int32(grant.Attempt),
		Seq:         event.Seq,
		OccurredAt:  pgtype.Timestamptz{Time: event.OccurredAt, Valid: true},
		EventType:   event.Type,
		Source:      event.EmittedBy,
		Status:      event.Status,

		SchemaVersion: event.SchemaVersion,
	})
}

func (s *Service) storeOne(ctx context.Context, e gen.InsertTraceEventParams) error {
	rows, err := s.queries().InsertTraceEvent(ctx, e)
	if err != nil {

		if pgconv.IsUniqueViolation(err) {
			return fmt.Errorf("%w: event_id %s conflicts with an event already stored at seq %d of this stream",
				ErrInvalid, pgconv.UUIDString(e.EventID), e.Seq)
		}
		return err
	}
	if rows == 0 {
		return errDuplicate
	}
	return nil
}

type OrchestratorEvent struct {
	WorkspaceID pgtype.UUID
	RunID       pgtype.UUID
	Attempt     int
	Type        string
	Status      string
	Payload     any
}

func RecordOrchestratorEvent(ctx context.Context, tx pgx.Tx, event OrchestratorEvent) error {
	if tx == nil {
		return errPersistenceNotConfigured
	}
	workspaceID, runID, attempt, eventType, status := event.WorkspaceID, event.RunID, event.Attempt, event.Type, event.Status
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
	encoded, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	if err := validatePayload(eventType, encoded); err != nil {
		return err
	}

	masker := &Masker{}
	masked, err := masker.Mask(encoded)
	if err != nil {
		return err
	}

	var eventID pgtype.UUID
	if err := eventID.Scan(newUUID()); err != nil {
		return err
	}
	var statusPtr *string
	if status != "" {
		statusPtr = &status
	}
	stored, err := masked.stored(gen.InsertTraceEventParams{
		EventID: eventID, WorkspaceID: workspaceID, RunID: runID,
		Attempt: int32(attempt), Seq: seq,
		OccurredAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		EventType:     eventType,
		Source:        SourceOrchestr,
		Status:        statusPtr,
		SchemaVersion: schemaVersionFor(eventType),
	})
	if err != nil {
		return err
	}
	rows, err := q.InsertTraceEvent(ctx, stored)
	if err != nil {
		return err
	}
	if rows > 0 {
		metrics.TraceEvents.WithLabelValues(SourceOrchestr, "stored").Inc()
	}
	return nil
}

type StreamHealth struct {
	Attempt      int     `json:"attempt"`
	EmittedBy    string  `json:"emitted_by"`
	Received     int     `json:"received"`
	HighestSeq   int64   `json:"highest_seq"`
	MissingCount int64   `json:"missing_count"`
	MissingSeq   []int64 `json:"missing_seq,omitempty"`
	LateEvents   int     `json:"late_events"`

	lastEventAt time.Time
}

type AdvancedView struct {
	RunID string `json:"run_id"`

	Complete  bool           `json:"complete"`
	Streams   []StreamHealth `json:"streams"`
	Events    []EventView    `json:"events"`
	NextAfter int64          `json:"next_after"`
	HasMore   bool           `json:"has_more"`

	EvaluationTruncated bool `json:"-"`
}

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

	occurredAt time.Time
}

type ProgressStep struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

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
	Steps        []ProgressStep  `json:"steps"`

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

type UsageSummary struct {
	Model        string `json:"model,omitempty"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`

	CostCredits *int64 `json:"cost_credits"`
	CostSource  string `json:"cost_source,omitempty"`

	CostUSD *float64 `json:"-"`
}

const tracePageSize = int32(1_000)

const (
	evaluationTailEvents       = int32(500)
	evaluationActivationEvents = int32(100)
	evaluationErrorEvents      = int32(100)
	reportedMissingSeqs        = int32(1_000)
)

func (s *Service) Advanced(ctx context.Context, workspaceID, runID pgtype.UUID, after int64) (AdvancedView, error) {
	run, err := s.runState(ctx, workspaceID, runID)
	if err != nil {
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
	if nothingWasCollected(run, health) {
		view.Complete = false
	}
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
	events := make([]gen.TraceEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, gen.TraceEvent{
			EventID: row.EventID, Attempt: row.Attempt, Seq: row.Seq, OccurredAt: row.OccurredAt,
			Source: row.Source, EventType: row.EventType, Status: row.Status, Late: row.Late,
			MaskedFields: row.MaskedFields, Payload: row.Payload,
		})
	}
	return eventViews(events)
}

func sortEventViews(events []EventView) {
	sort.Slice(events, func(i, j int) bool {

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

func (s *Service) AdvancedAll(ctx context.Context, workspaceID, runID pgtype.UUID) (AdvancedView, error) {
	run, err := s.runState(ctx, workspaceID, runID)
	if err != nil {
		return AdvancedView{}, err
	}
	health, err := s.traceStreamHealth(ctx, workspaceID, runID)
	if err != nil {
		return AdvancedView{}, err
	}
	all := AdvancedView{RunID: pgconv.UUIDString(runID), Complete: true, Streams: health, Events: []EventView{}}
	if nothingWasCollected(run, health) {
		all.Complete = false
	}
	for _, stream := range health {
		if stream.MissingCount > 0 {
			all.Complete = false
		}
	}
	rows, err := s.queries().ListEvaluationTraceEvents(ctx, gen.ListEvaluationTraceEventsParams{
		EvaluationRunID: runID, EvaluationWorkspaceID: workspaceID,
		TailEvents:          evaluationTailEvents,
		ActivationEventType: TypeSkillActivation, ActivationEvents: evaluationActivationEvents,
		ErrorEventType: TypeError, ErrorEvents: evaluationErrorEvents,
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

func (s *Service) LiveEvents(
	ctx context.Context, workspaceID, runID pgtype.UUID, eventIDs []pgtype.UUID,
) ([]pgtype.UUID, error) {
	return s.queries().FindLiveTraceEvents(ctx, gen.FindLiveTraceEventsParams{
		WorkspaceID: workspaceID, RunID: runID, EventIds: eventIDs,
	})
}

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
		Source: SourceSandbox,
	})
	return MaskingActivityFacts{
		RecentEvents: row.RecentEvents, EarlierEvents: row.EarlierEvents, MaskedFields: row.MaskedFields,
	}, err
}

func (s *Service) General(ctx context.Context, workspaceID, runID pgtype.UUID) (Summary, error) {
	if s.ReadRunTransitions == nil {
		return Summary{}, errRunReaderNotConfigured
	}
	run, err := s.runState(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}
	fold, err := s.readGeneralFold(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}

	summary := fold.summary
	summary.RunID, summary.Status, summary.Complete, summary.Steps = pgconv.UUIDString(runID), run.Status, true, []ProgressStep{}
	if run.StatusReason != nil {
		summary.StatusReason = *run.StatusReason
	}
	if summary.Usage != nil {
		summary.Usage.CostUSD = fold.costUSD
		s.priceInCredits(summary.Usage)
	}
	summary.Truncated = summary.SkillsTotal > len(summary.Skills) || summary.ErrorsTotal > len(summary.Errors)
	health, err := s.traceStreamHealth(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}
	if anyStreamMissing(health) {
		summary.Complete = false
	}
	if last := lastEventOf(health); !last.IsZero() {
		summary.LastEventAt = last.UTC().Format("2006-01-02T15:04:05Z")
	}

	if nothingWasCollected(run, health) {
		summary.Complete = false
	}
	transitions, err := s.ReadRunTransitions(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}
	for _, t := range transitions {
		summary.Steps = append(summary.Steps, progressStepOf(t))
	}

	return summary, nil
}

func (s *Service) priceInCredits(usage *UsageSummary) {
	if c := usage.CostUSD; c != nil && s.Credits != nil {
		if credits, ok := s.Credits(*c); ok {
			usage.CostCredits = &credits
		}
	}
}

func lastEventOf(health []StreamHealth) time.Time {
	var last time.Time
	for _, stream := range health {
		if stream.lastEventAt.After(last) {
			last = stream.lastEventAt
		}
	}
	return last
}

func anyStreamMissing(health []StreamHealth) bool {
	for _, stream := range health {
		if stream.MissingCount > 0 {
			return true
		}
	}
	return false
}

func progressStepOf(t RunTransition) ProgressStep {
	step := ProgressStep{Status: t.ToStatus}
	if t.Reason != nil && *t.Reason != "" {
		step.Reason = *t.Reason
	}
	return step
}

func (s *Service) readGeneralFold(ctx context.Context, workspaceID, runID pgtype.UUID) (generalFold, error) {
	q := s.queries()
	rows, err := q.ListTraceGeneralFacts(ctx, gen.ListTraceGeneralFactsParams{
		RunID: runID, WorkspaceID: workspaceID, EventTypes: generalEventTypes(),
	})
	if err != nil {
		return generalFold{}, err
	}
	fold := foldGeneral(rows)
	if final := fold.finalOutput; final != nil {
		fold.summary.FinalOutput, err = q.GetTraceEventText(ctx, gen.GetTraceEventTextParams{
			RunID: runID, WorkspaceID: workspaceID, Source: final.Source, Attempt: final.Attempt, Seq: final.Seq,
		})
		if err != nil {
			return generalFold{}, err
		}
	}
	return fold, nil
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
		RunID: runID, WorkspaceID: workspaceID, MissingSeqReported: reportedMissingSeqs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]StreamHealth, 0, len(rows))
	for _, row := range rows {
		out = append(out, StreamHealth{
			Attempt: int(row.Attempt), EmittedBy: row.Source, Received: int(row.Received),
			HighestSeq: row.HighestSeq, MissingCount: row.MissingCount,
			MissingSeq: row.MissingSeq, LateEvents: int(row.LateEvents), lastEventAt: row.LastEventAt.Time,
		})
	}
	return out, nil
}

func newUUID() string { return uuid.NewString() }

func sourceLabel(source string) string {
	if sources[source] {
		return source
	}
	return "unknown"
}
