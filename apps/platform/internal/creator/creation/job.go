package creation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

type JobArgs struct {
	SessionID   pgtype.UUID `json:"session_id"`
	WorkspaceID pgtype.UUID `json:"workspace_id"`
	Revision    int64       `json:"expected_revision"`
	ReceiptID   pgtype.UUID `json:"receipt_id"`
}

func (JobArgs) Kind() string                 { return "creation_step" }
func (JobArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 1} }

type Worker struct {
	river.WorkerDefaults[JobArgs]
	Svc *Service
}

func (w *Worker) Work(ctx context.Context, j *river.Job[JobArgs]) error {
	return w.Svc.Step(ctx, j.Args, nil)
}
func (w *Worker) Timeout(*river.Job[JobArgs]) time.Duration { return 3 * time.Minute }

type ExpiryArgs struct{}

func (ExpiryArgs) Kind() string { return "creation_expiry" }

type ExpiryWorker struct {
	river.WorkerDefaults[ExpiryArgs]
	Svc *Service
}

func (w *ExpiryWorker) Work(ctx context.Context, _ *river.Job[ExpiryArgs]) error {
	recoverErr := w.Svc.Recover(ctx)
	_, purgeErr := PurgeExpired(ctx, w.Svc.Pool, w.Svc.Limits.Retention)
	return errors.Join(recoverErr, purgeErr)
}

// limitSentence names the way forward for a session that just tripped a
// ceiling: steps cannot be raised (a new creation is the only option), but a
// budget ceiling can be lifted with raise_budget (05 R-46 (raise)).
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
	return l.Valid() && p.Steps < l.MaxSteps && len(p.Messages)+2 <= MaxMessages && spent+p.ReservedUSD+l.MaxCallCostUSD <= math.Min(p.BudgetUSD, l.MaxCostUSD)+1e-10
}

// allowedTools hides the tool-call intents from the model once the session
// has already spent its tool-call budget. Python turns a disallowed tool
// intent into a clarification, so this cannot fail proposal() with ErrLimit.
func allowedTools(toolCalls, maxToolCalls int) []string {
	if toolCalls < maxToolCalls {
		return []string{"search_catalog", "validate_draft"}
	}
	return []string{}
}

// callTimeoutSeconds converts a call's absolute deadline into the seconds Python
// should be told to use, leaving a 5s headroom for Go's own cleanup after the
// call returns. It fails closed when too little time is left to make a call at all.
func callTimeoutSeconds(deadline time.Time) (int, error) {
	remaining := int(time.Until(deadline).Seconds()) - 5
	if remaining < 1 {
		return 0, ErrUnavailable
	}
	return remaining, nil
}
func settleCost(p *Snapshot, reserved float64, usage *llmclient.GatewayUsage) {
	if usage == nil || usage.CostUSD == nil || !finite(*usage.CostUSD) || *usage.CostUSD < 0 {
		p.UsageUnknown = true
		return
	}
	p.ReservedUSD = math.Max(0, p.ReservedUSD-reserved)
	if p.SpentUSD == nil {
		v := 0.0
		p.SpentUSD = &v
	}
	*p.SpentUSD += *usage.CostUSD
}
func (s *Service) Step(ctx context.Context, a JobArgs, diagram *llmclient.GenerateDiagram) error {
	if s.LLM == nil || s.IssueKey == nil || s.RevokeKey == nil {
		return ErrUnavailable
	}
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
	e, err := decode(row)
	if err != nil {
		return err
	}
	if row.State != "queued" || e.ActiveReceipt != a.ReceiptID || !live(row) {
		if diagram != nil {
			return ErrConflict
		}
		return nil
	}
	r, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if err != nil {
		return err
	}
	if r.Status != "queued" || r.ExpectedRevision != a.Revision {
		if diagram != nil {
			return ErrConflict
		}
		return nil
	}
	if diagram != nil && !diagramMatches(e.Snapshot, diagram) {
		return ErrInvalidCommand
	}
	if !e.Deadline.After(time.Now()) {
		return s.failQueued(ctx, tx, row, e, a, "創作已達這次核准的限制，請開始新的創作。")
	}
	if !canSpend(e.Snapshot, e.Limits) {
		return s.failQueued(ctx, tx, row, e, a, limitSentence(e.Snapshot, e.Limits))
	}
	if diagram == nil && e.Snapshot.DiagramFingerprint != "" && e.Snapshot.DiagramUnderstanding == "" {
		return s.failQueued(ctx, tx, row, e, a, "流程圖需要重新上傳。")
	}
	_, err = q.ClaimCreationReceipt(ctx, gen.ClaimCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if err != nil {
		return err
	}
	e.Snapshot.Steps++
	e.Snapshot.ReservedUSD += e.Limits.MaxCallCostUSD
	e.ActiveDeadline = time.Now().Add(e.Limits.CallTimeout + 10*time.Second)
	row, err = advance(ctx, tx, row, "working", "attempt_started", e)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	callDeadline := time.Now().Add(e.Limits.CallTimeout + 5*time.Second)
	callCtx, cancel := context.WithDeadline(ctx, callDeadline)
	defer cancel()
	// Cross-process cancellation: the durable row is authoritative, not this goroutine.
	stopWatch := make(chan struct{})
	go func() {
		defer close(stopWatch)
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-callCtx.Done():
				return
			case <-tick.C:
				current, getErr := gen.New(s.Pool).GetCreationSession(callCtx, gen.GetCreationSessionParams{ID: a.SessionID, WorkspaceID: a.WorkspaceID})
				if getErr != nil || current.State != "working" || !live(current) {
					cancel()
					return
				}
			}
		}
	}()
	ws := identity.Workspace{ID: a.WorkspaceID}
	req := llmclient.CreationStepRequest{SessionID: UUID(a.SessionID), Revision: row.Revision, Messages: e.Snapshot.Messages, Brief: e.Snapshot.Brief, AcceptanceCriteria: e.Snapshot.AcceptanceCriteria, BriefConfirmed: e.Snapshot.BriefConfirmed, DiagramUnderstanding: e.Snapshot.DiagramUnderstanding, DiagramConfirmed: e.Snapshot.DiagramConfirmed, Diagram: diagram, References: []llmclient.GenerateReference{}, AllowedTools: allowedTools(e.Snapshot.ToolCalls, e.Limits.MaxToolCalls), MaxOutputTokens: e.Limits.MaxOutputTokens}
	draft := e.Snapshot.Draft
	// A correction after a validated draft falls back to PreviousDraft: send it
	// as the working draft, but never its (now stale) validation result, or
	// Python's _observe would treat this as "review" and hand back the
	// pre-correction draft as finished.
	sendValidation := draft != nil
	if draft == nil {
		draft = e.PreviousDraft
	}
	if draft != nil {
		req.Draft = &draft.Skill
		if sendValidation {
			report := []rune(draft.Validation)
			marker := []rune("\n[findings truncated]")
			if len(report) > MaxTextRunes {
				report = append(report[:MaxTextRunes-len(marker)], marker...)
			}
			req.DraftValidation = &llmclient.CreationDraftValidation{ContentHash: draft.ContentHash, Blocked: draft.Blocked, Report: string(report)}
		}
	}
	var response *llmclient.CreationStepResponse
	var callErr error
	var knownZero llmclient.GatewayUsage
	zero := 0.0
	knownZero.CostUSD = &zero
	usage := &knownZero
	for _, ref := range e.Snapshot.References {
		if !ref.Confirmed || s.ResolveReference == nil {
			callErr = ErrNotFound
			break
		}
		_, content, err := s.ResolveReference(callCtx, ws, ref.SkillID, ref.VersionID)
		if err != nil {
			callErr = ErrNotFound
			break
		}
		req.References = append(req.References, content)
	}
	if callErr == nil {
		req.GatewayKey, callErr = s.IssueKey(callCtx, UUID(a.SessionID), UUID(a.ReceiptID), e.Limits.MaxCallCostUSD, e.Limits.CallTimeout+10*time.Second)
		if callErr == nil {
			var remaining int
			if remaining, callErr = callTimeoutSeconds(callDeadline); callErr == nil {
				req.TimeoutSeconds = remaining
				usage = nil // From this point onward, absence of usage can never mean zero.
				response, callErr = s.LLM.CreationStep(callCtx, req)
				if response != nil {
					usage = response.Usage
				}
			}
		}
	}
	cancel()
	<-stopWatch
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cleanupCancel()
	_ = s.RevokeKey(cleanupCtx, UUID(a.ReceiptID))
	return s.finish(cleanupCtx, a, response, usage, callErr, diagram != nil)
}
func (s *Service) failQueued(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e envelope, a JobArgs, message string) error {
	state := "failed"
	if e.Snapshot.DiagramFingerprint != "" && e.Snapshot.DiagramUnderstanding == "" {
		state = "needs_reupload"
	}
	e.ActiveReceipt = pgtype.UUID{}
	e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "assistant", Content: message})
	if _, err := advance(ctx, tx, row, state, "attempt_refused", e); err != nil {
		return err
	}
	_, err := gen.New(tx).FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID, Status: "failed", Result: []byte("{}"), Usage: []byte("{}")})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) finish(ctx context.Context, a JobArgs, response *llmclient.CreationStepResponse, usage *llmclient.GatewayUsage, callErr error, hadDiagram bool) error {
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
		// Recovery already declared this attempt's spend unknown and moved the
		// session on. Record the usage on its own receipt only: no revision
		// bump, no event row, the snapshot stays untouched — a bump here is what
		// used to drop the queued attempt that replaced this one.
		// A receipt still "running" while ActiveReceipt points elsewhere is a
		// cancellation mid-call; that one falls through so settleCost can mark
		// the spend unknown on the snapshot, without proposing anything.
		u, _ := json.Marshal(usage)
		if _, err := q.FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID, Status: "finished", Result: []byte("{}"), Usage: u}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	settleCost(&e.Snapshot, e.Limits.MaxCallCostUSD, usage)
	state := row.State
	next := false
	if state == "working" && e.ActiveReceipt == a.ReceiptID && receipt.Status == "running" {
		e.ActiveReceipt = pgtype.UUID{}
		if callErr == nil && response != nil && live(row) && e.Deadline.After(time.Now()) {
			if hadDiagram && response.DiagramUnderstanding == "" {
				err = ErrInvalidCommand
			} else {
				state, next, err = s.proposal(ctx, identity.Workspace{ID: a.WorkspaceID}, row.Revision+1, &e, response)
			}
		} else {
			err = ErrUnavailable
		}
		if err != nil {
			state = "failed"
			if hadDiagram && e.Snapshot.DiagramUnderstanding == "" {
				state = "needs_reupload"
			}
			e.Snapshot.PendingAction = ""
			e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "assistant", Content: "這一步未完成；已保留進度與實際可取得的費用。請檢查後再繼續。"})
			if errors.Is(callErr, ErrNotFound) {
				state = "waiting_confirmation"
				e.Snapshot.PendingAction = "confirm_references"
				for i := range e.Snapshot.References {
					e.Snapshot.References[i].Available = false
					e.Snapshot.References[i].Confirmed = false
				}
				e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "assistant", Content: "參考內容目前不可用，請換選後再確認。"})
			}
			next = false
		}
	}
	if next {
		if canSpend(e.Snapshot, e.Limits) {
			state = "queued"
			if _, err = s.enqueue(ctx, tx, row, &e, false); err != nil {
				return err
			}
		} else {
			state = "waiting_input"
			e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "assistant", Content: limitSentence(e.Snapshot, e.Limits)})
		}
	}
	_, err = advance(ctx, tx, row, state, "attempt_settled", e)
	if err != nil {
		return err
	}
	u, _ := json.Marshal(usage)
	_, err = q.FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID, Status: "finished", Result: []byte("{}"), Usage: u})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// reasonSentences names the Go-owned sentence for each of Python's guard-rail
// reason codes (05 R-46 (c)); Python sends the code, never the wording.
var reasonSentences = map[string]string{
	"tool_unavailable":       "目前無法使用這項工具，請補充需求或選擇可用的參考。",
	"confirm_diagram_first":  "請先確認流程圖的理解；確認後再依它建立草稿。",
	"confirm_brief_first":    "請先確認這份需求與驗收條件，再建立草稿。",
	"validation_unavailable": "目前無法驗證草稿，請補充需求或稍後再試。",
	"diagram_incomplete":     "請補充流程圖的節點、條件、分支與不確定處，或重新上傳流程圖。",
	"search_query_missing":   "請告訴我要在目錄中搜尋什麼關鍵字。",
}

// reasonSentence looks up the sentence for a guard-rail reason code; an
// unknown code is refused rather than shown to the user verbatim.
func reasonSentence(code string) (string, error) {
	s, ok := reasonSentences[code]
	if !ok {
		return "", ErrInvalidCommand
	}
	return s, nil
}

// validateCriteria enforces the same bounds as llm-internal.yaml's
// acceptance_criteria array (MaxAcceptanceCriteria items, each non-blank and
// at most MaxCriterionRunes runes).
func validateCriteria(criteria []string) error {
	if len(criteria) > MaxAcceptanceCriteria {
		return ErrInvalidCommand
	}
	for _, c := range criteria {
		if strings.TrimSpace(c) == "" || utf8.RuneCountInString(c) > MaxCriterionRunes {
			return ErrInvalidCommand
		}
	}
	return nil
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func (s *Service) proposal(ctx context.Context, ws identity.Workspace, revision int64, e *envelope, r *llmclient.CreationStepResponse) (string, bool, error) {
	p := &e.Snapshot
	if r.Reason != "" {
		sentence, err := reasonSentence(r.Reason)
		if err != nil {
			return "", false, err
		}
		r.Message = sentence
	}
	if r.DiagramUnderstanding != "" && !validDiagramInterpretation(r.DiagramUnderstanding) {
		return "", false, ErrInvalidCommand
	}
	if r.Message == "" || utf8.RuneCountInString(r.Message) > MaxTextRunes || utf8.RuneCountInString(r.Brief) > MaxTextRunes || utf8.RuneCountInString(r.DiagramUnderstanding) > MaxTextRunes || len(p.Messages) >= MaxMessages {
		return "", false, ErrInvalidCommand
	}
	if err := validateCriteria(r.AcceptanceCriteria); err != nil {
		return "", false, err
	}
	p.Model = r.Model
	p.PromptVersion = r.PromptVersion
	p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "assistant", Content: r.Message})
	// Go independently binds confirmations; a compromised provider cannot change
	// either confirmed input and smuggle a draft or a tool call past that review.
	if r.DiagramUnderstanding != "" && r.DiagramUnderstanding != p.DiagramUnderstanding {
		p.DiagramUnderstanding = r.DiagramUnderstanding
		p.DiagramConfirmed = false
		invalidate(p)
		p.PendingAction = "confirm_diagram"
		return "waiting_confirmation", false, nil
	}
	briefChanged := r.Brief != "" && r.Brief != p.Brief
	criteriaChanged := len(r.AcceptanceCriteria) > 0 && !equalStrings(r.AcceptanceCriteria, p.AcceptanceCriteria)
	if briefChanged || criteriaChanged {
		if briefChanged {
			p.Brief = r.Brief
		}
		if criteriaChanged {
			p.AcceptanceCriteria = r.AcceptanceCriteria
		}
		p.BriefConfirmed = false
		invalidate(p)
		p.PendingAction = "confirm_brief"
		return "waiting_confirmation", false, nil
	}
	switch r.Outcome {
	case "clarification":
		p.PendingAction = ""
		return "waiting_input", false, nil
	case "confirm_brief":
		if strings.TrimSpace(p.Brief) == "" {
			return "", false, ErrInvalidCommand
		}
		p.BriefConfirmed = false
		p.PendingAction = "confirm_brief"
		return "waiting_confirmation", false, nil
	case "confirm_diagram":
		if p.DiagramUnderstanding == "" {
			return "", false, ErrInvalidCommand
		}
		p.DiagramConfirmed = false
		p.PendingAction = "confirm_diagram"
		return "waiting_confirmation", false, nil
	case "draft":
		if !confirmed(*p) || r.Brief != p.Brief || (len(r.AcceptanceCriteria) > 0 && !equalStrings(r.AcceptanceCriteria, p.AcceptanceCriteria)) || (p.DiagramFingerprint != "" && r.DiagramUnderstanding != p.DiagramUnderstanding) || r.Draft == nil || s.ValidateDraft == nil {
			return "", false, ErrInvalidCommand
		}
		hash, report, blocked, err := s.ValidateDraft(ctx, *r.Draft)
		if err != nil {
			return "", false, err
		}
		p.PreviousDraft = e.PreviousDraft
		if p.Draft == nil || p.Draft.ContentHash != hash {
			p.Candidate = nil
		}
		p.Draft = &Draft{revision, hash, *r.Draft, report, blocked}
		p.PendingAction = ""
		return "draft_ready", false, nil
	case "tool_intent":
		if r.ToolIntent == nil || p.ToolCalls >= e.Limits.MaxToolCalls {
			return "", false, ErrLimit
		}
		p.ToolCalls++
		switch r.ToolIntent.Kind {
		case "search_catalog":
			if s.SearchReferences == nil {
				return "", false, ErrUnavailable
			}
			if strings.TrimSpace(r.ToolIntent.Query) == "" {
				p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: "目錄搜尋需要關鍵字；這次沒有搜尋。"})
				return "queued", true, nil
			}
			refs, err := s.SearchReferences(ctx, ws, r.ToolIntent.Query)
			if err != nil {
				return "", false, err
			}
			if len(refs) > 3 {
				refs = refs[:3]
			}
			if len(refs) == 0 {
				p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: "目錄文字搜尋沒有符合的可用參考；未呼叫額外模型。"})
				return "queued", true, nil
			}
			for i := range refs {
				refs[i].Confirmed = false
			}
			p.References = refs
			invalidate(p)
			p.BriefConfirmed = false
			p.PendingAction = "confirm_references"
			return "waiting_confirmation", false, nil
		case "validate_draft":
			if !confirmed(*p) || r.Brief != p.Brief || (len(r.AcceptanceCriteria) > 0 && !equalStrings(r.AcceptanceCriteria, p.AcceptanceCriteria)) || (p.DiagramFingerprint != "" && r.DiagramUnderstanding != p.DiagramUnderstanding) || r.Draft == nil || s.ValidateDraft == nil {
				return "", false, ErrInvalidCommand
			}
			hash, report, blocked, err := s.ValidateDraft(ctx, *r.Draft)
			if err != nil {
				return "", false, err
			}
			if p.Draft != nil && p.Draft.ContentHash != hash {
				e.PreviousDraft = p.Draft
			}
			if p.Draft == nil || p.Draft.ContentHash != hash {
				p.Candidate = nil
			}
			p.PreviousDraft = e.PreviousDraft
			p.Draft = &Draft{revision, hash, *r.Draft, report, blocked}
			p.PendingAction = ""
			p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: fmt.Sprintf("Go 靜態驗證完成，blocked=%t；完整 finding 隨 draft_validation 提供，不代表試跑成功。", blocked)})
			return "queued", true, nil
		}
	}
	return "", false, ErrInvalidCommand
}
