package operations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type ReportItem struct {
	Status string   `json:"status"`
	Text   string   `json:"text"`
	Cites  []string `json:"cites"`
}

type DailyReportResult struct {
	Items     []ReportItem      `json:"items"`
	Proposals []ProposalRequest `json:"proposals,omitempty"`
}

const (
	ReportFine      = "fine"
	ReportAttention = "attention"
)

const factAction = "action"

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
	for i, proposal := range report.Proposals {
		if err := checkProposal(proposal, facts); err != nil {
			return fmt.Errorf("proposal %d: %w", i+1, err)
		}
	}
	return nil
}

func checkProposal(proposal ProposalRequest, facts []any) error {
	if strings.TrimSpace(proposal.Action) == "" {
		return errors.New("the proposal names no action")
	}
	if strings.TrimSpace(proposal.Reason) == "" {
		return errors.New("the proposal gives no reason")
	}
	if !slices.ContainsFunc(facts, func(fact any) bool { return offers(fact, proposal.Action) }) {
		return fmt.Errorf("it proposes %q, which no returned fact offers", proposal.Action)
	}
	return checkCites(proposal.Cites, facts)
}

func offers(node any, action string) bool {
	value, ok := node.(map[string]any)
	if !ok {
		return false
	}
	if value[factAction] == action {
		return true
	}
	for _, child := range value {
		if offers(child, action) {
			return true
		}
	}
	return false
}

func checkCites(cites []string, facts []any) error {
	if len(cites) == 0 {
		return errors.New("it cites no fact")
	}
	for _, cite := range cites {
		if _, ok := citedValue(cite, facts); !ok {
			return fmt.Errorf("it cites %q, which no tool returned", cite)
		}
	}
	return nil
}

func DailyReportProposals(result json.RawMessage) []ProposalRequest {
	var report DailyReportResult
	if json.Unmarshal(result, &report) != nil {
		return nil
	}
	return report.Proposals
}

func checkItem(item ReportItem, facts []any) error {
	if item.Status != ReportFine && item.Status != ReportAttention {
		return fmt.Errorf("status %q is neither %q nor %q", item.Status, ReportFine, ReportAttention)
	}
	if strings.TrimSpace(item.Text) == "" {
		return errors.New("the item says nothing")
	}
	return checkCites(item.Cites, facts)
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

func citedValue(pointer string, facts []any) (any, bool) {
	for _, fact := range facts {
		if value, ok := valueAt(pointer, fact); ok {
			return value, true
		}
	}
	return nil, false
}

var pointerUnescaper = strings.NewReplacer("~1", "/", "~0", "~")

// valueAt follows a JSON Pointer (RFC 6901) into a decoded document; the
// whole document ("") is not a citation, because it names no single fact.
func valueAt(pointer string, node any) (any, bool) {
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	for _, token := range strings.Split(pointer[1:], "/") {
		token = pointerUnescaper.Replace(token)
		switch value := node.(type) {
		case map[string]any:
			next, ok := value[token]
			if !ok {
				return nil, false
			}
			node = next
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(value) || token != strconv.Itoa(i) {
				return nil, false
			}
			node = value[i]
		default:
			return nil, false
		}
	}
	return node, true
}

func DailyReportSightings(result json.RawMessage, steps []StepRecord) []Sighting {
	var report DailyReportResult
	if json.Unmarshal(result, &report) != nil {
		return nil
	}
	facts := returnedFacts(steps)
	var sightings []Sighting
	for _, item := range report.Items {
		if item.Status != ReportAttention {
			continue
		}
		evidence := make(map[string]any, len(item.Cites))
		for _, cite := range item.Cites {
			if value, ok := citedValue(cite, facts); ok {
				evidence[cite] = value
			}
		}
		sightings = append(sightings, Sighting{Title: item.Text, Cites: item.Cites, Evidence: evidence})
	}
	return sightings
}
