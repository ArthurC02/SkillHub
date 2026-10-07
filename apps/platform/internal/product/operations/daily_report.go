package operations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type ReportItem struct {
	Status string   `json:"status"`
	Text   string   `json:"text"`
	Cites  []string `json:"cites"`
}

type DailyReportResult struct {
	Items []ReportItem `json:"items"`
}

const (
	ReportFine      = "fine"
	ReportAttention = "attention"
)

var errNoItems = errors.New("the report has no items")

func CitesOnlyReturnedFacts(result json.RawMessage, steps []StepRecord) error {
	dec := json.NewDecoder(bytes.NewReader(result))
	dec.DisallowUnknownFields()
	var report DailyReportResult
	if err := dec.Decode(&report); err != nil {
		return fmt.Errorf("the report is not a daily report: %w", err)
	}
	if len(report.Items) == 0 {
		return errNoItems
	}
	facts := returnedFacts(steps)
	for i, item := range report.Items {
		if err := checkItem(item, facts); err != nil {
			return fmt.Errorf("item %d: %w", i+1, err)
		}
	}
	return nil
}

func checkItem(item ReportItem, facts []any) error {
	if item.Status != ReportFine && item.Status != ReportAttention {
		return fmt.Errorf("status %q is neither %q nor %q", item.Status, ReportFine, ReportAttention)
	}
	if strings.TrimSpace(item.Text) == "" {
		return errors.New("the item says nothing")
	}
	if len(item.Cites) == 0 {
		return errors.New("the item cites no fact")
	}
	for _, cite := range item.Cites {
		if !citedInAny(cite, facts) {
			return fmt.Errorf("it cites %q, which no tool returned", cite)
		}
	}
	return nil
}

func returnedFacts(steps []StepRecord) []any {
	var facts []any
	for _, step := range steps {
		var decoded any
		if json.Unmarshal([]byte(step.Result), &decoded) == nil {
			facts = append(facts, decoded)
		}
	}
	return facts
}

func citedInAny(pointer string, facts []any) bool {
	for _, fact := range facts {
		if resolves(pointer, fact) {
			return true
		}
	}
	return false
}

// resolves follows a JSON Pointer (RFC 6901) into a decoded document; the
// whole document ("") is not a citation, because it names no single fact.
func resolves(pointer string, node any) bool {
	if !strings.HasPrefix(pointer, "/") {
		return false
	}
	for _, token := range strings.Split(pointer[1:], "/") {
		token = strings.NewReplacer("~1", "/", "~0", "~").Replace(token)
		switch value := node.(type) {
		case map[string]any:
			next, ok := value[token]
			if !ok {
				return false
			}
			node = next
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(value) || token != strconv.Itoa(i) {
				return false
			}
			node = value[i]
		default:
			return false
		}
	}
	return true
}
