package creation

import (
	"strings"
	"testing"
)

func TestFeedbackCharacterizationEvaluationFreeTextKeepsJudgeFieldsInOrder(t *testing.T) {
	observation := `{"evaluation":{"summary":"summary","criterion_results":[{"text":"criterion one","reason":"first reason"},{"text":"criterion two","reason":"second reason"}],"deterministic_findings":[{"message":"first finding"},{"message":"second finding"}]}}`

	got := evaluationFreeText(observation)
	want := "summary\nfirst reason\nsecond reason\nfirst finding\nsecond finding"
	if got != want {
		t.Fatalf("evaluationFreeText() = %q, want %q", got, want)
	}
}

func TestFeedbackCharacterizationTrialQuestionsTruncatesCriterionAndReasonAtRuneLimits(t *testing.T) {
	criterion := strings.Repeat("界", 201)
	reason := strings.Repeat("理", 301)
	observation := `{"evaluation":{"evaluation_available":true,"status":"completed","criterion_results":[{"text":"` + criterion + `","result":"failed","reason":"` + reason + `"}]}}`

	got := trialQuestions(observation)
	want := "這次試跑有條件沒過：\n- 「" + strings.Repeat("界", 200) + "…」：沒過——" + strings.Repeat("理", 300) + "…\n要照這些條件改草稿、還是改條件或範例輸入？也可以直接說你要它改哪裡。"
	if got != want {
		t.Fatalf("trialQuestions() = %q, want %q", got, want)
	}
}

func TestFeedbackCharacterizationToolsNotRequestedDeduplicatesAddedToolsCaseInsensitively(t *testing.T) {
	got := toolsNotRequested("Read bash", "read BASH Python", "please use bash")
	want := []string{"Python"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("toolsNotRequested() = %v, want %v", got, want)
	}
}
