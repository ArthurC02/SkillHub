package eval

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

type Reflection struct {
	row gen.BatchReflection
	suggestionLedger
	events []Event
}

func (r *Reflection) Decide(suggestionID pgtype.UUID, to Decision) {
	r.settle(r.decide(suggestionID, to))
}

func (r *Reflection) RecordApplied(versionID pgtype.UUID, suggestionIDs []pgtype.UUID) {
	r.settle(r.recordApplied(versionID, suggestionIDs))
}

func (r *Reflection) Refusal() (Refusal, bool) {
	for _, event := range r.events {
		if refused, ok := event.(Refused); ok {
			return refused.Reason, true
		}
	}
	return "", false
}

func (r *Reflection) settle(event Event, refusal Refusal) {
	if refusal != "" {
		event = Refused{Reason: refusal}
	}
	r.events = append(r.events, cloneEvent(event))
}

func reflectionEventType(event Event) string {
	switch event.(type) {
	case SuggestionDecided:
		return outbox.BatchReflectionSuggestionDecided
	case SuggestionsApplied:
		return outbox.BatchReflectionSuggestionsApplied
	}
	return ""
}

func loadReflection(ctx context.Context, q *gen.Queries, workspaceID, id pgtype.UUID) (*Reflection, error) {
	row, err := q.GetBatchReflection(ctx, gen.GetBatchReflectionParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return &Reflection{row: row}, nil
}

func loadReflectionWithSuggestions(
	ctx context.Context, q *gen.Queries, workspaceID, reflectionID pgtype.UUID, suggestionIDs []pgtype.UUID,
) (*Reflection, error) {
	r, err := loadReflection(ctx, q, workspaceID, reflectionID)
	if err != nil {
		return nil, err
	}
	r.suggestions = map[pgtype.UUID]gen.EvaluationSuggestion{}
	for _, id := range suggestionIDs {
		suggestion, err := q.LockEvaluationSuggestion(ctx, gen.LockEvaluationSuggestionParams{
			ID: id, WorkspaceID: workspaceID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if suggestion.ReflectionID == r.row.ID {
			r.suggestions[id] = suggestion
		}
	}
	rows, err := q.ListReflectionSuggestionApplications(ctx, gen.ListReflectionSuggestionApplicationsParams{
		ReflectionID: reflectionID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	applications := make([]suggestionApplication, len(rows))
	for i, row := range rows {
		applications[i] = suggestionApplication{suggestionID: row.SuggestionID, versionID: row.SkillVersionID}
	}
	r.loadApplications(applications)
	return r, nil
}

func saveReflectionUnlessRefused(ctx context.Context, tx pgx.Tx, r *Reflection) error {
	if reason, refused := r.Refusal(); refused {
		return reason.err()
	}
	q := gen.New(tx)
	for _, event := range r.events {
		if err := writeSuggestionEvent(ctx, q, r.row.WorkspaceID, &r.suggestionLedger, event); err != nil {
			return err
		}
		if err := outbox.Insert(ctx, tx, outbox.NewEvent{
			EventType: reflectionEventType(event), EventVersion: outbox.EventVersion1,
			CorrelationID: r.row.BatchID, WorkspaceID: r.row.WorkspaceID,
			AggregateType: outbox.AggregateBatchReflection, AggregateID: r.row.ID, Payload: event,
		}); err != nil {
			return err
		}
	}
	return nil
}

const MaxDigestChars = maxDigestChars

type RunFailure struct {
	RunID pgtype.UUID
	v     assessment
}

func FailureOfRun(ctx context.Context, s *Service, workspaceID, runID pgtype.UUID) (RunFailure, bool, error) {
	ev, err := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{RunID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return RunFailure{}, false, nil
	}
	if err != nil {
		return RunFailure{}, false, err
	}
	return failureOf(runID, ev)
}

func failureOf(runID pgtype.UUID, ev gen.Evaluation) (RunFailure, bool, error) {
	if Status(ev.Status) != StatusCompleted {
		return RunFailure{}, false, nil
	}
	v := assessment{overall: verdict.Overall(ev.Overall)}
	if ev.Summary != nil {
		v.summary = *ev.Summary
	}
	if len(ev.CriterionResults) > 0 {
		if err := json.Unmarshal(ev.CriterionResults, &v.results); err != nil {
			return RunFailure{}, false, err
		}
	}
	if len(ev.DeterministicFindings) > 0 {
		if err := json.Unmarshal(ev.DeterministicFindings, &v.findings); err != nil {
			return RunFailure{}, false, err
		}
	}
	return RunFailure{RunID: runID, v: v}, worthSuggesting(v), nil
}

type FailureReading struct {
	Sections    []string
	Refs        []verdict.EvidenceRef
	FileTree    []string
	TargetFiles []modelport.TargetFile
}

func ReadFailures(ctx context.Context, s *Service, workspaceID pgtype.UUID, failures []RunFailure, sectionChars int) (FailureReading, error) {
	var out FailureReading
	for i, failure := range failures {
		m, err := s.gather(ctx, workspaceID, failure.RunID)
		if err != nil {
			return FailureReading{}, err
		}
		if i == 0 {
			out.FileTree, out.TargetFiles = s.packageFiles(ctx, m)
		}
		digest, refs := suggestionDigest(m, failure.v)
		digest = withUnopenedFiles(digest, m.Advanced, out.FileTree)
		for _, ref := range refs {
			ref.RunID = pgconv.UUIDString(failure.RunID)
			out.Refs = append(out.Refs, ref)
		}
		out.Sections = append(out.Sections, firstChars(digest, sectionChars))
	}
	return out, nil
}

type ReflectionRecord struct {
	Row         gen.BatchReflection
	Suggestions []Suggestion
}

type NewReflection struct {
	WorkspaceID, Actor, BatchID, SkillVersionID pgtype.UUID
	DevTestCaseIDs                              []pgtype.UUID
	Outcome                                     SuggestionOutcome
	RunsRead, RunsNotRead                       int
	Proposals                                   []modelport.ImprovementProposal
	Refs                                        []verdict.EvidenceRef
}

func RecordReflection(ctx context.Context, s *Service, n NewReflection) (ReflectionRecord, error) {
	var kept []keptProposal
	if len(n.Proposals) > 0 {
		keepProposals(proposalOwner{key: "batch_id", id: pgconv.UUIDString(n.BatchID)}, n.Proposals, n.Refs,
			func(k keptProposal) error { kept = append(kept, k); return nil })
		n.Outcome = SuggestionsNoneKept
		if len(kept) > 0 {
			n.Outcome = SuggestionsKept
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ReflectionRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	row, err := q.CreateBatchReflection(ctx, gen.CreateBatchReflectionParams{
		WorkspaceID: n.WorkspaceID, BatchID: n.BatchID, SkillVersionID: n.SkillVersionID, DevTestCaseIds: n.DevTestCaseIDs,
		Outcome: string(n.Outcome), RunsRead: int32(n.RunsRead), RunsNotRead: int32(n.RunsNotRead), CreatedBy: n.Actor,
	})
	if err != nil {
		return ReflectionRecord{}, err
	}
	out := ReflectionRecord{Row: row}
	for _, k := range kept {
		sug, err := q.CreateReflectionSuggestion(ctx, gen.CreateReflectionSuggestionParams{
			WorkspaceID: n.WorkspaceID, ReflectionID: row.ID, Category: k.Category, Problem: k.Problem,
			Evidence: k.Evidence, TargetPath: k.TargetPath, ProposedContent: k.ProposedContent, ExpectedImpact: k.ExpectedImpact,
		})
		if err != nil {
			return ReflectionRecord{}, err
		}
		out.Suggestions = append(out.Suggestions, suggestionOf(sug))
	}
	return out, tx.Commit(ctx)
}

func ListReflections(ctx context.Context, s *Service, workspaceID, batchID pgtype.UUID, limit int32) ([]ReflectionRecord, error) {
	q := s.queries()
	rows, err := q.ListBatchReflections(ctx, gen.ListBatchReflectionsParams{
		WorkspaceID: workspaceID, BatchID: batchID, RowLimit: limit,
	})
	if err != nil || len(rows) == 0 {
		return []ReflectionRecord{}, err
	}
	ids := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	suggestions, err := q.ListReflectionSuggestions(ctx, gen.ListReflectionSuggestionsParams{ReflectionIds: ids, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	byReflection := make(map[pgtype.UUID][]Suggestion, len(rows))
	for _, sug := range suggestions {
		byReflection[sug.ReflectionID] = append(byReflection[sug.ReflectionID], suggestionOf(sug))
	}
	out := make([]ReflectionRecord, len(rows))
	for i, row := range rows {
		out[i] = ReflectionRecord{Row: row, Suggestions: byReflection[row.ID]}
	}
	return out, nil
}

func ReflectionSuggestionViews(ctx context.Context, s *Service, workspaceID pgtype.UUID, suggestions []Suggestion) ([]SuggestionView, error) {
	views := make([]SuggestionView, 0, len(suggestions))
	byRun := map[string][][]verdict.EvidenceRef{}
	for _, sug := range suggestions {
		v := toSuggestionView(sug)
		views = append(views, v)
		for i := range v.Evidence {
			run := v.Evidence[i].RunID
			byRun[run] = append(byRun[run], v.Evidence[i:i+1])
		}
	}
	for run, sets := range byRun {
		if err := markRunEvidence(ctx, s, workspaceID, run, sets); err != nil {
			return nil, err
		}
	}
	return views, nil
}

func markRunEvidence(ctx context.Context, s *Service, workspaceID pgtype.UUID, run string, sets [][]verdict.EvidenceRef) error {
	var runID pgtype.UUID
	if err := runID.Scan(run); err != nil {
		for _, refs := range sets {
			refs[0].Available = false
		}
		return nil
	}
	live, err := s.resolveEvidence(ctx, workspaceID, runID, sets...)
	if err != nil {
		return err
	}
	for _, refs := range sets {
		markAvailability(refs, live)
		refs[0].RunID = ""
	}
	return nil
}
