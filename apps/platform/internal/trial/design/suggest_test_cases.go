package testlab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/platformbudget"
)

var (
	ErrTestCaseSuggestUnavailable = errors.New("目前無法自動建議測試題，請自己手動寫")
	ErrTestCaseCreditThreshold    = errors.New("點數不足，無法開始建議測試題，沒有呼叫模型。請聯絡管理者為這個帳號加點。")
	ErrTestCaseModelCallsHalted   = errors.New("平台暫停中，這次沒有呼叫模型、也沒有扣點。測試題可以自己手動寫，或稍後再試。")
	ErrTestCaseBudgetExhausted    = errors.New("平台的模型總額度暫時用完，這次沒有扣點。測試題可以自己手動寫，或在" + platformbudget.ResetNotice + "額度恢復後再試。")
)

var SuggestTestCasesBudget = modelbudget.Endpoint{Kind: "suggest-test-cases", Deadline: suggestTestCasesTimeout}

const (
	suggestTestCasesTimeout = 70 * time.Second // budget-over: app.SUGGEST_TEST_CASES_TIMEOUT_SECONDS

	maxExistingNamesSent = 50
)

type TestCasesRequest struct {
	SkillName     string
	SkillSummary  string
	Focus         string
	ExistingNames []string

	Within time.Duration
}

type ProposedTestCase struct {
	Name       string
	UserPrompt string
	Criteria   []string
}

type TestCasesProposal struct {
	TestCases     []ProposedTestCase
	PromptVersion string
	Usage         *ModelUsage
}

type TestCaseSuggester interface {
	SuggestTestCases(ctx context.Context, req TestCasesRequest) (*TestCasesProposal, error)
}

type TestCaseSuggestions struct {
	Lab     *Service
	LLM     TestCaseSuggester
	Billing SuggestBilling
}

type TestCaseSuggestion struct {
	Name       string   `json:"name"`
	UserPrompt string   `json:"user_prompt"`
	Criteria   []string `json:"criteria"`
}

func (s *TestCaseSuggestions) Suggest(
	ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, focus string,
) ([]TestCaseSuggestion, error) {
	focus = strings.TrimSpace(focus)
	if utf8.RuneCountInString(focus) > limits.SuggestTestCasesBodyFocusMaxLength {
		return nil, fmt.Errorf("%w: 想測的方向最多 %d 個字", ErrInvalid, limits.SuggestTestCasesBodyFocusMaxLength)
	}
	if s.Lab == nil || s.Lab.ReadSkill == nil {
		return nil, errRegistryReadNotConfigured
	}
	skill, found, err := s.Lab.ReadSkill(ctx, ws.ID, skillID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	if s.LLM == nil {
		return nil, ErrTestCaseSuggestUnavailable
	}
	existing, err := gen.New(s.Lab.Pool).ListTestCasesForSkill(ctx, gen.ListTestCasesForSkillParams{
		SkillID: skillID, WorkspaceID: ws.ID,
	})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(existing))
	for _, tc := range existing {
		names = append(names, tc.Name)
	}

	if err := s.Billing.admit(ctx, ws.ID); err != nil {
		return nil, testCaseAdmitError(err)
	}
	callCtx, cancel := context.WithTimeout(ctx, suggestTestCasesTimeout)
	defer cancel()
	proposal, err := s.LLM.SuggestTestCases(callCtx, TestCasesRequest{
		SkillName:     skill.Name,
		SkillSummary:  truncate(derefString(skill.Summary), maxSkillSummaryBytes),
		Focus:         focus,
		ExistingNames: names[max(0, len(names)-maxExistingNamesSent):],
		Within:        s.Lab.Budgets.Within(ctx, SuggestTestCasesBudget),
	})
	if errors.Is(err, ErrModelBudgetExhausted) {
		return nil, ErrTestCaseBudgetExhausted
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTestCaseSuggestUnavailable, err)
	}
	s.Billing.charge(ctx, ws.ID, proposal.Usage)
	slog.Info("testlab: test cases suggested", "prompt_version", proposal.PromptVersion, "proposed", len(proposal.TestCases))

	return keptTestCaseSuggestions(names, proposal.TestCases), nil
}

func testCaseAdmitError(err error) error {
	switch {
	case errors.Is(err, ErrCreditThreshold):
		return ErrTestCaseCreditThreshold
	case errors.Is(err, ErrModelCallsHalted):
		return ErrTestCaseModelCallsHalted
	}
	return err
}

func keptTestCaseSuggestions(existing []string, proposed []ProposedTestCase) []TestCaseSuggestion {
	taken := make(map[string]bool, len(existing)+len(proposed))
	for _, name := range existing {
		taken[strings.TrimSpace(name)] = true
	}
	out := make([]TestCaseSuggestion, 0, len(proposed))
	for _, p := range proposed {
		if len(out) == limits.TestCaseSuggestionsSuggestionsMaxItems {
			break
		}
		name, prompt, err := validateDraft(p.Name, p.UserPrompt)
		if err != nil || taken[name] {
			continue
		}
		taken[name] = true
		out = append(out, TestCaseSuggestion{Name: name, UserPrompt: prompt, Criteria: keptCriteria(p.Criteria)})
	}
	return out
}

func keptCriteria(proposed []string) []string {
	seen := make(map[string]bool, len(proposed))
	out := make([]string, 0, len(proposed))
	for _, c := range proposed {
		if len(out) == limits.TestCaseSuggestionsSuggestionsItemCriteriaMaxItems {
			break
		}
		text, err := validateCriterion(c)
		if err != nil || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}
