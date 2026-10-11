package run

import (
	"errors"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

var twoQuestions = []trace.Question{{Question: "小孩算不算人數？"}, {Question: "幾點出發？"}}

func TestEveryQuestionTakesExactlyOneUsableAnswer(t *testing.T) {
	cases := []struct {
		name    string
		answers []string
		ok      bool
	}{
		{"one answer each", []string{"算", "八點"}, true},
		{"an answer at the byte limit", []string{strings.Repeat("a", MaxAnswerBytes), "八點"}, true},
		{"an answer one byte past the limit", []string{strings.Repeat("a", MaxAnswerBytes+1), "八點"}, false},
		{"one answer short", []string{"算"}, false},
		{"one answer too many", []string{"算", "八點", "多"}, false},
		{"a blank answer", []string{"算", " \n"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := pairAnswers(twoQuestions, c.answers)
			if c.ok && (err != nil || len(got) != 2) {
				t.Fatalf("want two answered questions, got %v, %v", got, err)
			}
			if !c.ok && !errors.Is(err, ErrAnswersDoNotFit) {
				t.Fatalf("want ErrAnswersDoNotFit, got %v", err)
			}
		})
	}
}

func TestTheContinuedPromptCarriesEveryQuestionAndItsTrimmedAnswer(t *testing.T) {
	answered, err := pairAnswers(twoQuestions, []string{" 算 ", "八點"})
	if err != nil {
		t.Fatal(err)
	}
	got := continuedPrompt("規劃家庭旅遊\n", answered)
	const want = "規劃家庭旅遊\n\n你先前提出的問題與我的回答：" +
		"\n1. 問：小孩算不算人數？\n   答：算" +
		"\n2. 問：幾點出發？\n   答：八點"
	if got != want {
		t.Errorf("prompt = %q\nwant %q", got, want)
	}
}
