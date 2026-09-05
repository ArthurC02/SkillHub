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
	if err := w.Svc.Recover(ctx); err != nil {
		return err
	}
	_, err := PurgeExpired(ctx, w.Svc.Pool, w.Svc.Limits.Retention)
	return err
}
func canSpend(p Snapshot, l Limits) bool {
	spent := 0.0
	if p.SpentUSD != nil {
		spent = *p.SpentUSD
	}
	return l.Valid() && p.Steps < l.MaxSteps && p.ToolCalls <= l.MaxToolCalls && spent+p.ReservedUSD+l.MaxCallCostUSD <= math.Min(p.BudgetUSD, l.MaxCostUSD)+1e-10
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
		return nil
	}
	r, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if err != nil {
		return err
	}
	if r.Status != "queued" || r.ExpectedRevision != a.Revision || row.Revision != a.Revision+1 {
		return nil
	}
	if diagram != nil && !diagramMatches(e.Snapshot, diagram) {
		return ErrInvalidCommand
	}
	if !canSpend(e.Snapshot, e.Limits) || !e.Deadline.After(time.Now()) {
		return s.failQueued(ctx, tx, row, e, a, "創作已達這次核准的限制，請開始新的創作。")
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
	callCtx, cancel := context.WithTimeout(ctx, e.Limits.CallTimeout+5*time.Second)
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
	req := llmclient.CreationStepRequest{SessionID: UUID(a.SessionID), Revision: row.Revision, Messages: e.Snapshot.Messages, Brief: e.Snapshot.Brief, BriefConfirmed: e.Snapshot.BriefConfirmed, DiagramUnderstanding: e.Snapshot.DiagramUnderstanding, DiagramConfirmed: e.Snapshot.DiagramConfirmed, Diagram: diagram, References: []llmclient.GenerateReference{}, AllowedTools: []string{"search_catalog", "validate_draft"}, TimeoutSeconds: int(e.Limits.CallTimeout.Seconds()), MaxOutputTokens: e.Limits.MaxOutputTokens}
	if e.Snapshot.Draft != nil {
		req.Draft = &e.Snapshot.Draft.Skill
	} else if e.PreviousDraft != nil {
		req.Draft = &e.PreviousDraft.Skill
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
			usage = nil // From this point onward, absence of usage can never mean zero.
			response, callErr = s.LLM.CreationStep(callCtx, req)
			if response != nil {
				usage = response.Usage
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
			e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "assistant", Content: "已達這次核准的創作限制。"})
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
func (s *Service) proposal(ctx context.Context, ws identity.Workspace, revision int64, e *envelope, r *llmclient.CreationStepResponse) (string, bool, error) {
	p := &e.Snapshot
	if r.Message == "" || utf8.RuneCountInString(r.Message) > 20000 || utf8.RuneCountInString(r.Brief) > 20000 || utf8.RuneCountInString(r.DiagramUnderstanding) > 20000 || len(p.Messages) >= 98 {
		return "", false, ErrInvalidCommand
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
	if r.Brief != "" && r.Brief != p.Brief {
		p.Brief = r.Brief
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
		if !confirmed(*p) || r.Brief != p.Brief || (p.DiagramFingerprint != "" && r.DiagramUnderstanding != p.DiagramUnderstanding) || r.Draft == nil || s.ValidateDraft == nil {
			return "", false, ErrInvalidCommand
		}
		hash, report, blocked, err := s.ValidateDraft(ctx, *r.Draft)
		if err != nil {
			return "", false, err
		}
		p.PreviousDraft = e.PreviousDraft
		p.Draft = &Draft{revision, hash, *r.Draft, report, blocked}
		p.Candidate = nil
		p.PendingAction = ""
		return "draft_ready", false, nil
	case "tool_intent":
		if r.ToolIntent == nil || p.ToolCalls >= e.Limits.MaxToolCalls {
			return "", false, ErrLimit
		}
		p.ToolCalls++
		switch r.ToolIntent.Kind {
		case "search_catalog":
			if s.SearchReferences == nil || strings.TrimSpace(r.ToolIntent.Query) == "" {
				return "", false, ErrUnavailable
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
			p.PendingAction = "confirm_references"
			return "waiting_confirmation", false, nil
		case "validate_draft":
			if r.Draft != nil && confirmed(*p) {
				p.Draft = &Draft{Skill: *r.Draft}
			}
			if p.Draft == nil || s.ValidateDraft == nil {
				return "", false, ErrInvalidCommand
			}
			hash, report, blocked, err := s.ValidateDraft(ctx, p.Draft.Skill)
			if err != nil {
				return "", false, err
			}
			p.Draft.ContentHash = hash
			p.Draft.Validation = report
			p.Draft.Blocked = blocked
			p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: fmt.Sprintf("Go 靜態驗證（不代表試跑成功）：%s", report)})
			return "queued", true, nil
		}
	}
	return "", false, ErrInvalidCommand
}
