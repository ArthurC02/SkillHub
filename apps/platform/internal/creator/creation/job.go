package creation

import (
	"context"
	"encoding/json"
	"errors"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"log/slog"
	"math"
	"time"
)

type JobArgs struct {
	SessionID   pgtype.UUID `json:"session_id"`
	WorkspaceID pgtype.UUID `json:"workspace_id"`
	Revision    int64       `json:"expected_revision"`
	ReceiptID   pgtype.UUID `json:"receipt_id"`
}

func (s *Service) RecoverExpired(ctx context.Context) error {
	recoverErr := s.Recover(ctx)
	_, purgeErr := PurgeExpired(ctx, s.Pool, s.Limits.Retention)
	return errors.Join(recoverErr, purgeErr)
}

func limitSentence(p Snapshot, l Limits) string {
	if p.Steps >= l.MaxSteps {
		return "已達這次核准的步數上限，請開始新的創作。"
	}
	return "已達這次核准的預算或步數上限；可以提高預算後繼續，或開始新的創作。"
}
func canSpend(p Snapshot, l Limits) bool {
	spent := 0.0
	if p.SpentUSD != nil {
		spent = *p.SpentUSD
	}
	return l.Valid() && p.Steps < l.MaxSteps && p.hasRoomFor(2) && spent+p.ReservedUSD+l.MaxCallCostUSD <= math.Min(p.BudgetUSD, l.MaxCostUSD)+1e-10
}

func allowedTools(toolCalls, maxToolCalls int, fetch, knowledge, searchLeft bool) []string {
	if toolCalls >= maxToolCalls {
		return []string{}
	}
	tools := []string{"validate_draft"}
	if searchLeft {
		tools = append(tools, "search_catalog")
		if knowledge {
			tools = append(tools, "search_knowledge")
		}
	}
	if fetch {
		tools = append(tools, "fetch_url")
	}
	return tools
}

func callTimeoutSeconds(left time.Duration) (int, error) {
	remaining := int(math.Ceil(left.Seconds())) - 5
	if remaining < 1 {
		return 0, ErrUnavailable
	}
	return remaining, nil
}
func settleCost(p *Snapshot, reserved float64, usage *ModelUsage) {
	cost, known := knownCost(usage)
	if !known {
		p.UsageUnknown = true
		return
	}
	p.ReservedUSD = math.Max(0, p.ReservedUSD-reserved)
	if p.SpentUSD == nil {
		v := 0.0
		p.SpentUSD = &v
	}
	*p.SpentUSD += cost
}

func knownCost(usage *ModelUsage) (float64, bool) {
	if usage == nil || usage.CostUSD == nil || !finite(*usage.CostUSD) || *usage.CostUSD < 0 {
		return 0, false
	}
	return *usage.CostUSD, true
}

func knownCostUSD(usage *ModelUsage) *float64 {
	cost, known := knownCost(usage)
	if !known {
		return nil
	}
	return &cost
}

type attempt struct {
	e        envelope
	revision int64
}

func (s *Service) Step(ctx context.Context, a JobArgs, diagram *Diagram) error {
	if s.LLM == nil || s.IssueKey == nil || s.RevokeKey == nil {
		return ErrUnavailable
	}
	started, err := s.startAttempt(ctx, a, diagram)
	if err != nil || started == nil {
		return err
	}
	e := started.e
	callDeadline := time.Now().Add(e.Limits.CallTimeout + 5*time.Second)
	callCtx, cancel := context.WithDeadline(ctx, callDeadline)
	defer cancel()
	watching := s.cancelWhenSessionMoves(callCtx, cancel, a)
	req := s.stepRequest(a, started.revision, e, diagram)
	response, usage, callErr := s.callModel(callCtx, a, e, req, callDeadline)
	cancel()
	<-watching
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cleanupCancel()
	_ = s.RevokeKey(cleanupCtx, UUID(a.ReceiptID))
	return s.finish(cleanupCtx, a, response, usage, callErr, diagram != nil)
}

func (s *Service) startAttempt(ctx context.Context, a JobArgs, diagram *Diagram) (*attempt, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	row, err := q.LockCreationSession(ctx, gen.LockCreationSessionParams{ID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e, err := decode(row)
	if err != nil {
		return nil, err
	}
	if State(row.State) != StateQueued || e.ActiveReceipt != a.ReceiptID || !live(row) {
		return nil, staleAttempt(diagram != nil)
	}
	r, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if err != nil {
		return nil, err
	}
	if r.Status != "queued" || r.ExpectedRevision != a.Revision {
		return nil, staleAttempt(diagram != nil)
	}
	if diagram != nil && !diagramMatches(e.Snapshot, diagram) {
		return nil, ErrInvalidCommand
	}
	if refusal := refuseAttempt(e, diagram != nil); refusal != "" {
		return nil, s.failQueued(ctx, tx, row, e, a, refusal)
	}
	if _, err = q.ClaimCreationReceipt(ctx, gen.ClaimCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID}); err != nil {
		return nil, err
	}
	s.fetchPending(ctx, &e.Snapshot)
	e.Snapshot.Steps++
	e.Snapshot.ReservedUSD += e.Limits.MaxCallCostUSD
	e.ActiveDeadline = time.Now().Add(e.Limits.CallTimeout + 10*time.Second)
	if row, err = s.advance(ctx, tx, row, StateWorking, "attempt_started", e); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &attempt{e: e, revision: row.Revision}, nil
}

func staleAttempt(transient bool) error {
	if transient {
		return ErrConflict
	}
	return nil
}

type attemptRefusal string

const (
	refusedPastDeadline  attemptRefusal = "past_deadline"
	refusedOverLimit     attemptRefusal = "over_limit"
	refusedDiagramUnread attemptRefusal = "diagram_unread"
)

func refuseAttempt(e envelope, hasDiagram bool) attemptRefusal {
	switch {
	case !e.Deadline.After(time.Now()):
		return refusedPastDeadline
	case !canSpend(e.Snapshot, e.Limits):
		return refusedOverLimit
	case !hasDiagram && diagramUnread(e.Snapshot):
		return refusedDiagramUnread
	}
	return ""
}

func (r attemptRefusal) sentence(p Snapshot, l Limits) string {
	switch r {
	case refusedPastDeadline:
		return "創作已達這次核准的限制，請開始新的創作。"
	case refusedOverLimit:
		return limitSentence(p, l)
	case refusedDiagramUnread:
		return "流程圖需要重新上傳。"
	}
	return ""
}

func diagramUnread(p Snapshot) bool {
	return p.DiagramFingerprint != "" && p.DiagramUnderstanding == ""
}

func abandonedState(p Snapshot) State {
	if diagramUnread(p) {
		return StateNeedsReupload
	}
	return StateFailed
}

func (s *Service) fetchPending(ctx context.Context, p *Snapshot) {
	url := p.PendingFetchURL
	if url == "" || s.Fetch == nil {
		return
	}
	rec, text := s.Fetch(ctx, url)
	p.PendingFetchURL = ""
	p.Fetches = append(p.Fetches, rec)
	p.Messages = append(p.Messages, Message{Role: "tool", Content: fetchObservation(rec, s.masked(text))})
}

func (s *Service) cancelWhenSessionMoves(ctx context.Context, cancel context.CancelFunc, a JobArgs) <-chan struct{} {
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				current, err := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: a.SessionID, WorkspaceID: a.WorkspaceID})
				if sessionMoved(current, err) {
					cancel()
					return
				}
			}
		}
	}()
	return stopped
}

func sessionMoved(current gen.CreationSession, err error) bool {
	if err != nil {
		return errors.Is(err, pgx.ErrNoRows)
	}
	return State(current.State) != StateWorking || !live(current)
}

func (s *Service) stepRequest(a JobArgs, revision int64, e envelope, diagram *Diagram) StepRequest {
	p := e.Snapshot
	req := StepRequest{SessionID: UUID(a.SessionID), Revision: revision, Messages: p.Messages, Brief: p.Brief, AcceptanceCriteria: p.AcceptanceCriteria, SampleInput: p.SampleInput, BriefConfirmed: p.BriefConfirmed, DiagramUnderstanding: p.DiagramUnderstanding, DiagramConfirmed: p.DiagramConfirmed, Diagram: diagram, References: []ReferenceSkill{}, AllowedTools: allowedTools(p.ToolCalls, e.Limits.MaxToolCalls, s.Fetch != nil, s.SearchKnowledge != nil, p.SearchRounds < MaxSearchRounds), MaxOutputTokens: e.Limits.MaxOutputTokens}
	req.Draft, req.DraftValidation = draftForModel(p.Draft, e.PreviousDraft)
	return req
}

func draftForModel(current, previous *Draft) (*GeneratedSkill, *DraftValidation) {
	if current == nil {
		if previous == nil {
			return nil, nil
		}
		return &previous.Skill, nil
	}
	return &current.Skill, &DraftValidation{ContentHash: current.ContentHash, Blocked: current.Blocked, Report: truncatedReport(current.Validation)}
}

func truncatedReport(report string) string {
	runes := []rune(report)
	marker := []rune("\n[findings truncated]")
	if len(runes) > MaxTextRunes {
		runes = append(runes[:MaxTextRunes-len(marker)], marker...)
	}
	return string(runes)
}

func (s *Service) callModel(ctx context.Context, a JobArgs, e envelope, req StepRequest, deadline time.Time) (*StepResult, *ModelUsage, error) {
	zero := 0.0
	knownZero := &ModelUsage{CostUSD: &zero}
	references, err := s.referencedContent(ctx, identity.Workspace{ID: a.WorkspaceID}, e.Snapshot.References)
	if err != nil {
		return nil, knownZero, err
	}
	req.References = append(req.References, references...)
	if err = s.reserveCall(ctx, a.WorkspaceID, e.Limits.MaxCallCostUSD); err != nil {
		return nil, knownZero, err
	}
	if req.GatewayKey, err = s.IssueKey(ctx, UUID(a.SessionID), UUID(a.ReceiptID), e.Limits.MaxCallCostUSD, e.Limits.CallTimeout+10*time.Second); err != nil {
		return nil, knownZero, err
	}
	if req.TimeoutSeconds, err = callTimeoutSeconds(time.Until(deadline)); err != nil {
		return nil, knownZero, err
	}
	response, err := s.LLM.CreationStep(ctx, req)
	if response == nil {
		// A call that went out but brought back no response settles as
		// unknown cost, never as the known zero above.
		return nil, nil, err
	}
	return response, response.Usage, err
}

func (s *Service) referencedContent(ctx context.Context, ws identity.Workspace, refs []Reference) ([]ReferenceSkill, error) {
	var contents []ReferenceSkill
	for _, ref := range refs {
		if !ref.Confirmed || s.ResolveReference == nil {
			return nil, ErrNotFound
		}
		_, content, err := s.ResolveReference(ctx, ws, ref.SkillID, ref.VersionID)
		if err != nil {
			return nil, ErrNotFound
		}
		contents = append(contents, content)
	}
	return contents, nil
}

func (s *Service) reserveCall(ctx context.Context, workspaceID pgtype.UUID, maxCallCostUSD float64) error {
	if s.Billing == nil {
		return nil
	}
	ok, err := s.Billing.Reserve(ctx, workspaceID, maxCallCostUSD)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCreditFloor
	}
	return nil
}

func stepFailureMessage(err, callErr error) string {
	switch {
	case errors.Is(err, ErrInvalidCommand):
		return "這一步未完成：模型的回覆不符合會話規則，已保留進度與實際可取得的費用。請檢查後再繼續。"
	case callErr != nil && !errors.Is(callErr, ErrNotFound) && !errors.Is(callErr, ErrCreditFloor):
		return "這一步未完成：平台這一側沒能完成這次模型呼叫，不是你寫的內容的問題。已保留進度與實際可取得的費用。請檢查後再繼續。"
	}
	return "這一步未完成；已保留進度與實際可取得的費用。請檢查後再繼續。"
}

func (s *Service) failQueued(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e envelope, a JobArgs, refusal attemptRefusal) error {
	e.ActiveReceipt = pgtype.UUID{}
	e.Snapshot.Messages = append(e.Snapshot.Messages, Message{Role: "assistant", Content: refusal.sentence(e.Snapshot, e.Limits)})
	if _, err := s.advance(ctx, tx, row, abandonedState(e.Snapshot), "attempt_refused", e); err != nil {
		return err
	}
	_, err := gen.New(tx).FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID, Status: "failed", Result: []byte("{}"), Usage: []byte("{}")})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) finish(ctx context.Context, a JobArgs, response *StepResult, usage *ModelUsage, callErr error, hadDiagram bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	row, err := q.LockCreationSession(ctx, gen.LockCreationSessionParams{ID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	receipt, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if err != nil {
		return err
	}
	if receipt.Status != "running" && receipt.Status != "unknown" {
		return nil
	}
	e, err := decode(row)
	if err != nil {
		return err
	}
	if receipt.Status == "unknown" {
		return s.settleAbandonedAttempt(ctx, tx, a, e.Limits, usage)
	}
	settleCost(&e.Snapshot, e.Limits.MaxCallCostUSD, usage)
	if err = s.settleCredit(ctx, tx, a, e.Limits, usage); err != nil {
		return err
	}
	state, next := State(row.State), false
	if state == StateWorking && e.ActiveReceipt == a.ReceiptID && receipt.Status == "running" {
		e.ActiveReceipt = pgtype.UUID{}
		state, next = s.concludeAttempt(ctx, a, row, &e, response, callErr, hadDiagram)
	}
	if next {
		if state, err = s.queueNextStep(ctx, tx, row, &e); err != nil {
			return err
		}
	}
	if _, err = s.advance(ctx, tx, row, state, "attempt_settled", e); err != nil {
		return err
	}
	if err = finishAttemptReceipt(ctx, tx, a, usage); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) settleAbandonedAttempt(ctx context.Context, tx pgx.Tx, a JobArgs, l Limits, usage *ModelUsage) error {
	if err := finishAttemptReceipt(ctx, tx, a, usage); err != nil {
		return err
	}
	if err := s.settleCredit(ctx, tx, a, l, usage); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func finishAttemptReceipt(ctx context.Context, tx pgx.Tx, a JobArgs, usage *ModelUsage) error {
	u, _ := json.Marshal(usage)
	_, err := gen.New(tx).FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID, Status: "finished", Result: []byte("{}"), Usage: u})
	return err
}

func (s *Service) settleCredit(ctx context.Context, tx pgx.Tx, a JobArgs, l Limits, usage *ModelUsage) error {
	if s.Billing == nil {
		return nil
	}
	return s.Billing.Settle(ctx, tx, a.WorkspaceID, a.SessionID, a.Revision, knownCostUSD(usage), l.MaxCallCostUSD)
}

func (s *Service) concludeAttempt(ctx context.Context, a JobArgs, row gen.CreationSession, e *envelope, response *StepResult, callErr error, hadDiagram bool) (State, bool) {
	state, next, err := s.attemptOutcome(ctx, a, row, e, response, callErr, hadDiagram)
	if err == nil {
		return state, next
	}
	s.logStepFailure(a, callErr)
	return failedAttempt(&e.Snapshot, err, callErr, hadDiagram), false
}

func (s *Service) attemptOutcome(ctx context.Context, a JobArgs, row gen.CreationSession, e *envelope, response *StepResult, callErr error, hadDiagram bool) (State, bool, error) {
	if callErr != nil || response == nil || !live(row) || !e.Deadline.After(time.Now()) {
		return "", false, ErrUnavailable
	}
	if hadDiagram && response.DiagramUnderstanding == "" {
		return "", false, ErrInvalidCommand
	}
	return s.proposal(ctx, identity.Workspace{ID: a.WorkspaceID}, row.Revision+1, e, response)
}

func failedAttempt(p *Snapshot, err, callErr error, hadDiagram bool) State {
	state := StateFailed
	if hadDiagram && p.DiagramUnderstanding == "" {
		state = StateNeedsReupload
	}
	p.PendingAction = NothingPending
	p.Messages = append(p.Messages, Message{Role: "assistant", Content: stepFailureMessage(err, callErr)})
	if errors.Is(callErr, ErrNotFound) {
		state = StateWaitingConfirmation
		p.PendingAction = PendingReferenceChoice
		for i := range p.References {
			p.References[i].Available = false
			p.References[i].Confirmed = false
		}
		p.Messages = append(p.Messages, Message{Role: "assistant", Content: "參考內容目前不可用，請換選後再確認。"})
	}
	if errors.Is(callErr, ErrCreditFloor) {
		state = StateWaitingInput
		p.PendingAction = NothingPending
		p.Messages = append(p.Messages, Message{Role: "assistant", Content: "帳戶餘額已達可容忍的欠款上限，請充值後再繼續這場創作。"})
	}
	return state
}

func (s *Service) logStepFailure(a JobArgs, callErr error) {
	if callErr == nil {
		return
	}
	slog.Warn("creation: step failed", "session", UUID(a.SessionID), "revision", a.Revision, "error", s.masked(callErr.Error()))
}

func (s *Service) queueNextStep(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e *envelope) (State, error) {
	if !canSpend(e.Snapshot, e.Limits) {
		e.Snapshot.Messages = append(e.Snapshot.Messages, Message{Role: "assistant", Content: limitSentence(e.Snapshot, e.Limits)})
		return StateWaitingInput, nil
	}
	if _, err := s.enqueue(ctx, tx, row, e, false); err != nil {
		return "", err
	}
	return StateQueued, nil
}
