package creation

import (
	"context"
	"fmt"
	"strings"
	"time"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func (s *Service) useTool(ctx context.Context, revision int64, e *envelope, r *StepResult, found *searchAnswer) (State, bool, error) {
	run := s.toolFor(ctx, revision, e, r, found)
	if run == nil {
		return "", false, ErrInvalidCommand
	}
	if e.Snapshot.ToolCalls >= e.Limits.MaxToolCalls {
		return "", false, ErrLimit
	}
	e.Snapshot.ToolCalls++
	return run()
}

func (s *Service) toolFor(ctx context.Context, revision int64, e *envelope, r *StepResult, found *searchAnswer) func() (State, bool, error) {
	if r.ToolIntent == nil {
		return nil
	}
	p := &e.Snapshot
	switch r.ToolIntent.Kind {
	case searchCatalogTool, searchKnowledgeTool:
		return func() (State, bool, error) { return s.searchCatalog(p, r.ToolIntent, found) }
	case "fetch_url":
		return func() (State, bool, error) { return s.holdFetch(p, r.ToolIntent.Query) }
	case "validate_draft":
		return func() (State, bool, error) { return s.validateRequestedDraft(ctx, revision, e, r) }
	}
	return nil
}

func (s *Service) searchCatalog(p *Snapshot, intent *ToolIntent, found *searchAnswer) (State, bool, error) {
	if !s.canSearch() {
		return "", false, ErrUnavailable
	}
	if strings.TrimSpace(intent.Query) == "" {
		p.appendMessage("tool", "目錄搜尋需要關鍵字；這次沒有搜尋。")
		return StateQueued, true, nil
	}
	if p.SearchRounds >= MaxSearchRounds {
		p.appendMessage("tool", "目錄已搜過兩回都沒有相近的 Skill；請直接依需求起草。")
		return StateQueued, true, nil
	}
	refs, err := spendOn(p, found)
	if err != nil {
		return "", false, err
	}
	if len(refs) == 0 {
		p.SearchRounds++
		p.appendMessage("tool", emptySearchNote(p.SearchRounds))
		return StateQueued, true, nil
	}
	p.References = shortlist(refs)
	invalidate(p)
	p.BriefConfirmed = false
	p.PendingAction = PendingReferenceChoice
	return StateWaitingConfirmation, false, nil
}

func searchQueries(intent *ToolIntent) []string {
	queries := []string{strings.TrimSpace(intent.Query)}
	for _, q := range intent.Queries {
		if q = strings.TrimSpace(q); q != "" && !containsString(queries, q) && len(queries) < 4 {
			queries = append(queries, q)
		}
	}
	return queries
}

const (
	searchCatalogTool   = "search_catalog"
	searchKnowledgeTool = "search_knowledge"
)

type searchAnswer struct {
	refs []Reference
	cost float64
	err  error
}

const searchAheadTimeout = 20 * time.Second

func (s *Service) searchAhead(ctx context.Context, ws identity.Workspace, r *StepResult) *searchAnswer {
	if r == nil || r.Outcome != outcomeToolIntent || r.ToolIntent == nil || !s.canSearch() {
		return nil
	}
	intent := r.ToolIntent
	if (intent.Kind != searchCatalogTool && intent.Kind != searchKnowledgeTool) || strings.TrimSpace(intent.Query) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), searchAheadTimeout)
	defer cancel()
	return s.lookUp(ctx, ws, searchQueries(intent))
}

func (s *Service) canSearch() bool {
	return s.SearchKnowledge != nil || s.SearchReferences != nil
}

func spendOn(p *Snapshot, answer *searchAnswer) ([]Reference, error) {
	if answer == nil {
		return nil, ErrUnavailable
	}
	if answer.err == nil {
		addSpend(p, answer.cost)
	}
	return answer.refs, answer.err
}

func (s *Service) lookUp(ctx context.Context, ws identity.Workspace, queries []string) *searchAnswer {
	if s.SearchKnowledge == nil {
		refs, err := s.SearchReferences(ctx, ws, queries[0])
		return &searchAnswer{refs: refs, err: err}
	}
	refs, cost, err := s.SearchKnowledge(ctx, ws, queries)
	return &searchAnswer{refs: refs, cost: cost, err: err}
}

func emptySearchNote(round int) string {
	if round >= MaxSearchRounds {
		return "目錄搜了兩回都沒有相近的 Skill：沒有可參考的，請直接依需求起草。"
	}
	return fmt.Sprintf("目錄裡沒有符合的 Skill（第 %d／%d 回）；換個說法、加一個關鍵詞或另一種語言再搜一次，或直接起草。", round, MaxSearchRounds)
}

func (s *Service) holdFetch(p *Snapshot, query string) (State, bool, error) {
	if s.Fetch == nil {
		return "", false, ErrUnavailable
	}
	clean, allowed := fetchableURL(query)
	if !allowed {
		p.appendMessage("tool", "這個網址不符合規則（只接受公開的 http／https 網址，不含帳號密碼）；這次沒有連網。")
		return StateQueued, true, nil
	}
	p.PendingFetchURL = clean
	p.PendingAction = PendingFetchPermission
	return StateWaitingConfirmation, false, nil
}

func fetchableURL(requested string) (string, bool) {
	clean, err := validateFetchURL(requested)
	return clean, err == nil
}

func (s *Service) validateRequestedDraft(ctx context.Context, revision int64, e *envelope, r *StepResult) (State, bool, error) {
	p := &e.Snapshot
	if !draftFollowsConfirmation(*p, r) || s.ValidateDraft == nil {
		return "", false, ErrInvalidCommand
	}
	hash, report, blocked, err := s.ValidateDraft(ctx, *r.Draft)
	if err != nil {
		return "", false, err
	}
	revisedAfterUnmetRun := p.RunUnmet && (p.Draft == nil || p.Draft.ContentHash != hash)
	if p.Draft != nil && p.Draft.ContentHash != hash {
		e.PreviousDraft = p.Draft
	}
	forgetTrialOfReplacedDraft(p, hash)
	if p.Draft != nil && p.Draft.ContentHash == hash && !p.Draft.Blocked && !blocked {
		p.PendingAction = NothingPending
		p.appendMessage("tool", "這份草稿已通過同一次驗證；試跑由人從候選啟動，模型不能自己跑。草稿就緒。")
		return StateDraftReady, false, nil
	}
	p.PreviousDraft = e.PreviousDraft
	storeDraft(p, &Draft{revision, hash, *r.Draft, report, blocked})
	p.appendMessage("tool", fmt.Sprintf("Go 靜態驗證完成，blocked=%t；完整 finding 隨 draft_validation 提供，不代表試跑成功。", blocked))
	if revisedAfterUnmetRun && !blocked {
		return StateDraftReady, false, nil
	}
	return StateQueued, true, nil
}

func renamedOnly(prev, cur *Draft) bool {
	return prev != nil && cur != nil && prev.Skill.Name != cur.Skill.Name &&
		prev.Skill.Description == cur.Skill.Description && prev.Skill.Body == cur.Skill.Body
}

func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
