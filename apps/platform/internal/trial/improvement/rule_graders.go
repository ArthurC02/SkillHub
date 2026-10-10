package eval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

const runSucceeded = "succeeded"

type ruleGrader func(m material, values []string) (result, reason string)

var ruleGraders = map[string]ruleGrader{
	testlab.CheckContainsValues: gradeContainsValues,
	testlab.CheckAsks:           gradeAsks,
	testlab.CheckDoesNotAsk:     gradeDoesNotAsk,
}

func splitByRule(criteria []testlab.Criterion) (ruled, judged []testlab.Criterion) {
	for _, c := range criteria {
		if c.Check != nil && ruleGraders[c.Check.Kind] != nil {
			ruled = append(ruled, c)
		} else {
			judged = append(judged, c)
		}
	}
	return ruled, judged
}

func gradeByRule(m material, criteria []testlab.Criterion) []CriterionResult {
	out := make([]CriterionResult, 0, len(criteria))
	for _, c := range criteria {
		result, reason := ruleGraders[c.Check.Kind](m, c.Check.Values)
		if result == ResultPassed && !m.evidenceComplete() {
			result = ResultUndetermined
			reason += "; the run's evidence is incomplete, so this is not recorded as passed"
		}
		out = append(out, CriterionResult{
			CriterionID: c.ID, Text: c.Text, Result: result,
			Source: SourceRule, Evidence: []EvidenceRef{}, Reason: reason,
		})
	}
	return out
}

func gradeContainsValues(m material, values []string) (string, string) {
	missing := []string{}
	for _, v := range values {
		if !mentions(m.summary.FinalOutput, v) {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		return ResultFailed, "the final output does not mention " + strings.Join(missing, ", ")
	}
	return ResultPassed, "the final output mentions " + strings.Join(values, ", ")
}

func gradeAsks(m material, values []string) (string, string) {
	if len(m.summary.Questions) == 0 {
		return ResultFailed, "the run ended without asking the person anything"
	}
	asked := questionText(m.summary.Questions)
	missing := []string{}
	for _, v := range values {
		if !mentions(asked, v) {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		return ResultFailed, fmt.Sprintf("the run asked %d question(s), none about %s",
			len(m.summary.Questions), strings.Join(missing, ", "))
	}
	return ResultPassed, fmt.Sprintf("the run asked %d question(s)", len(m.summary.Questions))
}

func gradeDoesNotAsk(m material, _ []string) (string, string) {
	if m.run.Status != runSucceeded {
		return ResultFailed, "the run ended as " + m.run.Status + " without an answer"
	}
	if n := len(m.summary.Questions); n > 0 {
		return ResultFailed, fmt.Sprintf("the run asked %d question(s) instead of answering", n)
	}
	return ResultPassed, "the run answered without asking the person"
}

func questionText(questions []trace.Question) string {
	var b strings.Builder
	for _, q := range questions {
		b.WriteString(q.Question)
		b.WriteString("\n")
		b.WriteString(q.Why)
		b.WriteString("\n")
		b.WriteString(strings.Join(q.Options, "\n"))
		b.WriteString("\n")
	}
	return b.String()
}

func inCriteriaOrder(criteria []testlab.Criterion, groups ...[]CriterionResult) []CriterionResult {
	byID := map[string]CriterionResult{}
	for _, g := range groups {
		for _, r := range g {
			byID[r.CriterionID] = r
		}
	}
	out := make([]CriterionResult, 0, len(byID))
	for _, c := range criteria {
		if r, ok := byID[c.ID]; ok {
			out = append(out, r)
		}
	}
	return out
}

var (
	isoDate = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	clock   = regexp.MustCompile(`^(\d{1,3}):(\d{2})$`)
	decimal = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)
	spaces  = regexp.MustCompile(`\s+`)
)

func mentions(output, value string) bool {
	text := spaces.ReplaceAllString(output, "")
	for _, form := range writtenForms(value) {
		if containsStandalone(text, form) {
			return true
		}
	}
	return false
}

func writtenForms(value string) []string {
	if m := isoDate.FindStringSubmatch(value); m != nil {
		month, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		return []string{
			value,
			fmt.Sprintf("%s/%02d/%02d", m[1], month, day),
			fmt.Sprintf("%s/%d/%d", m[1], month, day),
			fmt.Sprintf("%s年%d月%d日", m[1], month, day),
			fmt.Sprintf("%d月%d日", month, day),
			fmt.Sprintf("%d/%d", month, day),
			fmt.Sprintf("%02d/%02d", month, day),
		}
	}
	if m := clock.FindStringSubmatch(value); m != nil {
		hours, _ := strconv.Atoi(m[1])
		minutes, _ := strconv.Atoi(m[2])
		forms := []string{value, fmt.Sprintf("%d小時%d分", hours, minutes)}
		if minutes == 0 {
			forms = append(forms, fmt.Sprintf("%d小時", hours))
		}
		return forms
	}
	if decimal.MatchString(value) {
		return []string{value, grouped(value)}
	}
	return []string{value}
}

const digitsPerGroup = 3

func grouped(number string) string {
	sign, digits := "", number
	if strings.HasPrefix(number, "-") {
		sign, digits = "-", number[1:]
	}
	whole, fraction, hasFraction := strings.Cut(digits, ".")
	for i := len(whole) - digitsPerGroup; i > 0; i -= digitsPerGroup {
		whole = whole[:i] + "," + whole[i:]
	}
	if hasFraction {
		whole += "." + fraction
	}
	return sign + whole
}

// A numeric form must not be part of a longer number: "12" is not in "120"
// or "3.12", and "1,200" is not in "1,200.5".
func containsStandalone(text, form string) bool {
	numericStart := form[0] == '-' || isDigit(form[0])
	numericEnd := isDigit(form[len(form)-1])
	for from := 0; ; {
		i := strings.Index(text[from:], form)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(form)
		joined := (numericStart && joinsBefore(text, start)) || (numericEnd && joinsAfter(text, end))
		if !joined {
			return true
		}
		from = start + 1
	}
}

func joinsBefore(text string, start int) bool {
	if start == 0 {
		return false
	}
	c := text[start-1]
	return isDigit(c) || c == '.' || c == ','
}

func joinsAfter(text string, end int) bool {
	if end >= len(text) {
		return false
	}
	if isDigit(text[end]) {
		return true
	}
	return (text[end] == '.' || text[end] == ',') && end+1 < len(text) && isDigit(text[end+1])
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
