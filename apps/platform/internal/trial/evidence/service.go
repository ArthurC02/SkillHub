package trace

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

var ErrNotFound = errors.New("run not found")

type Service struct {
	Pool               *pgxpool.Pool
	Signer             *Signer
	ReadRunState       func(context.Context, pgtype.UUID, pgtype.UUID) (RunState, bool, error)
	ReadIngestRunState func(context.Context, pgtype.UUID) (IngestRunState, bool, error)
	ReadRunTransitions func(context.Context, pgtype.UUID, pgtype.UUID) ([]RunTransition, error)

	Credits func(usd float64) (credits int64, ok bool)
}

type RunState struct {
	Status       string
	StatusReason *string
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
	for i := range events {
		event := &events[i]
		if err := s.ingestOne(ctx, run, grant, masker, event); err != nil {
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
	masker *Masker, event *Event,
) error {
	if err := event.Validate(); err != nil {
		return err
	}

	if event.EmittedBy != SourceSandbox {
		return fmt.Errorf("%w: ingestion token only permits sandbox events", ErrInvalid)
	}

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

		SchemaVersion: event.SchemaVersion,

		Masked:       true,
		MaskedFields: fields,
		Payload:      masked.Payload,
	})
	if err != nil {

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

type StreamHealth struct {
	Attempt      int     `json:"attempt"`
	EmittedBy    string  `json:"emitted_by"`
	Received     int     `json:"received"`
	HighestSeq   int64   `json:"highest_seq"`
	MissingCount int64   `json:"missing_count"`
	MissingSeq   []int64 `json:"missing_seq,omitempty"`
	LateEvents   int     `json:"late_events"`
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
		Steps:    []ProgressStep{},
	}
	if run.StatusReason != nil {
		summary.StatusReason = *run.StatusReason
	}
	if err := json.Unmarshal(folded, &summary); err != nil {
		return Summary{}, err
	}

	var foldCost struct {
		Usage *struct {
			CostUSD *float64 `json:"cost_usd"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(folded, &foldCost); err != nil {
		return Summary{}, err
	}
	if summary.Usage != nil && foldCost.Usage != nil {
		summary.Usage.CostUSD = foldCost.Usage.CostUSD
		if c := summary.Usage.CostUSD; c != nil && s.Credits != nil {
			if credits, ok := s.Credits(*c); ok {
				summary.Usage.CostCredits = &credits
			}
		}
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

	transitions, err := s.ReadRunTransitions(ctx, workspaceID, runID)
	if err != nil {
		return Summary{}, err
	}
	for _, t := range transitions {
		step := ProgressStep{Status: t.ToStatus}
		if t.Reason != nil && *t.Reason != "" {
			step.Reason = *t.Reason
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

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {

		panic("crypto/rand: " + err.Error())
	}
	// Set the RFC 4122 version (4) and variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func sourceLabel(source string) string {
	if sources[source] {
		return source
	}
	return "unknown"
}
