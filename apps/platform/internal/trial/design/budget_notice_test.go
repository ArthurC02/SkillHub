package testlab

import (
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/platformbudget"
)

func TestBothSuggestionBudgetRefusalsNameTheSharedResetTime(t *testing.T) {
	for name, err := range map[string]error{
		"criteria suggestion":  ErrModelBudgetExhausted,
		"test case suggestion": ErrTestCaseBudgetExhausted,
	} {
		if !strings.Contains(err.Error(), platformbudget.ResetNotice) {
			t.Errorf("%s refusal %q does not name the platform budget reset time %q", name, err, platformbudget.ResetNotice)
		}
	}
}
