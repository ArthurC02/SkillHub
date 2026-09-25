package creation

import (
	"encoding/json"
	"fmt"
	"strings"
)

func trialQuestions(observation string) string {
	var o struct {
		Evaluation struct {
			Available bool             `json:"evaluation_available"`
			Status    evaluationStatus `json:"status"`
			Results   []struct {
				Text   string `json:"text"`
				Result string `json:"result"`
				Reason string `json:"reason"`
			} `json:"criterion_results"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil || !o.Evaluation.Available || o.Evaluation.Status != evaluationCompleted {
		return ""
	}
	var lines []string
	for _, r := range o.Evaluation.Results {
		if r.Result != "failed" && r.Result != "undetermined" {
			continue
		}
		label := "沒過"
		if r.Result == "undetermined" {
			label = "尚無法判定"
		}
		line := fmt.Sprintf("- 「%s」：%s", truncateRunes(r.Text, 200), label)
		if r.Reason != "" {
			line += "——" + truncateRunes(r.Reason, 300)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return "這次試跑有條件沒過：\n" + strings.Join(lines, "\n") + "\n要照這些條件改草稿、還是改條件或範例輸入？也可以直接說你要它改哪裡。"
}

func evaluationFreeText(observation string) string {
	var o struct {
		Evaluation struct {
			Summary string `json:"summary"`
			Results []struct {
				Reason string `json:"reason"`
			} `json:"criterion_results"`
			Findings []struct {
				Message string `json:"message"`
			} `json:"deterministic_findings"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil {
		return ""
	}
	parts := []string{o.Evaluation.Summary}
	for _, r := range o.Evaluation.Results {
		parts = append(parts, r.Reason)
	}
	for _, f := range o.Evaluation.Findings {
		parts = append(parts, f.Message)
	}
	return strings.Join(parts, "\n")
}

func runUnmet(observation string) bool {
	var o struct {
		Evaluation struct {
			Available bool              `json:"evaluation_available"`
			Status    evaluationStatus  `json:"status"`
			Overall   evaluationOverall `json:"overall"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil || !o.Evaluation.Available {
		return false
	}
	return o.Evaluation.Status == evaluationCompleted && o.Evaluation.Overall != "" && o.Evaluation.Overall != overallMet
}
