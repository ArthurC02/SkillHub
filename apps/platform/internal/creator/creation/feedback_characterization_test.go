package creation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrialQuestionsPreserveUncertaintyWithoutInventingItsCause(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason string
	}{
		{"unverifiable evidence", "evidence_unverifiable: the quote cited from the agent's final output is not in it"},
		{"missing scenario", "樣本沒有這個情境"},
		{"no reason", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, err := json.Marshal(tc.reason)
			if err != nil {
				t.Fatal(err)
			}
			observation := `{"evaluation":{"evaluation_available":true,"status":"completed","criterion_results":[{"text":"總額為400","result":"undetermined","reason":` + string(reason) + `}]}}`
			want := "這次試跑有條件沒過：\n- 「總額為400」：尚無法判定"
			if tc.reason != "" {
				want += "——" + tc.reason
			}
			want += "\n要照這些條件改草稿、還是改條件或範例輸入？也可以直接說你要它改哪裡。"
			if got := trialQuestions(observation); got != want {
				t.Fatalf("trialQuestions() = %q, want %q", got, want)
			}
		})
	}
}

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
