package checks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

var sentenceBreak = regexp.MustCompile(`[。！？!?]|\n`)

func countChars(text string) int {
	n := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

func countSentences(text string) int {
	n := 0
	for _, part := range sentenceBreak.Split(text, -1) {
		if strings.TrimSpace(part) != "" {
			n++
		}
	}
	return n
}

type limitGrader struct {
	unit  string
	count func(string) int
}

func (g limitGrader) grade(m facts.Material, values []string) (string, string) {
	limit, err := parseLimit(values[0])
	if err != nil {
		return verdict.ResultUndetermined, fmt.Sprintf("the limit %q is not a positive whole number", values[0])
	}
	if !m.Run.Succeeded {
		return verdict.ResultFailed, "the run ended as " + m.Run.Status + " without an answer"
	}
	if strings.TrimSpace(m.Summary.FinalOutput) == "" {
		return verdict.ResultFailed, "the run gave no final output to count"
	}
	n := g.count(m.Summary.FinalOutput)
	if n > limit {
		return verdict.ResultFailed, fmt.Sprintf("the final output has %d %s, over the limit of %d", n, g.unit, limit)
	}
	return verdict.ResultPassed, fmt.Sprintf("the final output has %d %s, within the limit of %d", n, g.unit, limit)
}

func parseLimit(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%w: 上限要是正整數", ErrInvalidCheck)
	}
	return n, nil
}

func validateLimit(values []string) error {
	if strings.Contains(values[0], CheckItem) {
		return nil
	}
	_, err := parseLimit(values[0])
	return err
}

type maxCharsNode struct{}

func (maxCharsNode) kind() CheckKind {
	return CheckKind{Kind: "max_chars", Category: CategoryGrader, Label: "回答字數不超過",
		Description: "最終回答的字數（不計空白）不超過這個數", Values: Bounds{1, 1}}
}

func (maxCharsNode) validateValues(values []string) error { return validateLimit(values) }

func (n maxCharsNode) evaluate(s checkScope, c testlab.Check, path string) outcome {
	return ruleLeaf{n.kind(), limitGrader{"characters", countChars}.grade}.evaluate(s, c, path)
}

type maxSentencesNode struct{}

func (maxSentencesNode) kind() CheckKind {
	return CheckKind{Kind: "max_sentences", Category: CategoryGrader, Label: "回答句數不超過",
		Description: "最終回答的句數不超過這個數；句子以句號、問號、驚嘆號或換行分開", Values: Bounds{1, 1}}
}

func (maxSentencesNode) validateValues(values []string) error { return validateLimit(values) }

func (n maxSentencesNode) evaluate(s checkScope, c testlab.Check, path string) outcome {
	return ruleLeaf{n.kind(), limitGrader{"sentences", countSentences}.grade}.evaluate(s, c, path)
}

func calledTools(m facts.Material, names []string) (called, missing []string) {
	for _, name := range names {
		if timesCalled(m, name) > 0 {
			called = append(called, name)
		} else {
			missing = append(missing, name)
		}
	}
	return called, missing
}

func timesCalled(m facts.Material, name string) int {
	n := 0
	for tool, count := range m.Summary.ToolCalls.ByName {
		if strings.EqualFold(strings.TrimSpace(tool), name) {
			n += count
		}
	}
	return n
}

func gradeCallsTool(m facts.Material, values []string) (string, string) {
	if _, missing := calledTools(m, values); len(missing) > 0 {
		return verdict.ResultFailed, "the run never called " + strings.Join(missing, ", ")
	}
	return verdict.ResultPassed, "the run called " + strings.Join(values, ", ")
}

func gradeAvoidsTool(m facts.Material, values []string) (string, string) {
	if called, _ := calledTools(m, values); len(called) > 0 {
		return verdict.ResultFailed, "the run called " + strings.Join(called, ", ")
	}
	return verdict.ResultPassed, "the run never called " + strings.Join(values, ", ")
}

type callsToolNode struct{}

func (callsToolNode) kind() CheckKind {
	return CheckKind{Kind: "calls_tool", Category: CategoryGrader, Label: "有呼叫指定工具",
		Description: "整個 Run 至少呼叫過每個指定的工具一次；以工具名稱比對，不分大小寫", Values: Bounds{1, MaxCheckValues}}
}

func (n callsToolNode) evaluate(s checkScope, c testlab.Check, path string) outcome {
	return ruleLeaf{n.kind(), gradeCallsTool}.evaluate(s, c, path)
}

type avoidsToolNode struct{}

func (avoidsToolNode) kind() CheckKind {
	return CheckKind{Kind: "avoids_tool", Category: CategoryGrader, Label: "沒有呼叫指定工具",
		Description: "整個 Run 一次都沒有呼叫指定的工具；以工具名稱比對，不分大小寫", Values: Bounds{1, MaxCheckValues}}
}

func (n avoidsToolNode) evaluate(s checkScope, c testlab.Check, path string) outcome {
	return ruleLeaf{n.kind(), gradeAvoidsTool}.evaluate(s, c, path)
}
