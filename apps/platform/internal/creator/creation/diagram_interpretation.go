package creation

import (
	"strings"
	"unicode/utf8"
)

const (
	maxDiagramItems     = 64   // one-number: creationMaxDiagramItems
	maxDiagramItemRunes = 2000 // one-number: creationMaxDiagramItemRunes
)

type DiagramUncertainty struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer,omitempty"`
}

type DiagramInterpretation struct {
	Nodes         []string             `json:"nodes"`
	Conditions    []string             `json:"conditions"`
	Branches      []string             `json:"branches"`
	Uncertainties []DiagramUncertainty `json:"uncertainties"`
}

type DiagramDecomposition struct {
	Nodes         []string `json:"nodes"`
	Conditions    []string `json:"conditions"`
	Branches      []string `json:"branches"`
	Uncertainties []string `json:"uncertainties"`
}

func validRequiredDiagramItems(items []string, limit int) bool {
	return len(items) > 0 && validDiagramItems(items, limit)
}

func validDiagramItems(items []string, limit int) bool {
	if len(items) > limit {
		return false
	}
	for _, item := range items {
		if !validDiagramText(item) {
			return false
		}
	}
	return true
}

func validDiagramDecomposition(value *DiagramDecomposition) bool {
	return value != nil &&
		validRequiredDiagramItems(value.Nodes, maxDiagramItems) &&
		validDiagramItems(value.Conditions, maxDiagramItems) &&
		validDiagramItems(value.Branches, maxDiagramItems*2) &&
		validDiagramItems(value.Uncertainties, maxDiagramItems)
}

func newDiagramInterpretation(value *DiagramDecomposition) *DiagramInterpretation {
	if !validDiagramDecomposition(value) {
		return nil
	}
	result := &DiagramInterpretation{
		Nodes:         append([]string{}, value.Nodes...),
		Conditions:    append([]string{}, value.Conditions...),
		Branches:      append([]string{}, value.Branches...),
		Uncertainties: []DiagramUncertainty{},
	}
	for _, question := range value.Uncertainties {
		result.Uncertainties = append(result.Uncertainties, DiagramUncertainty{ID: UUID(newID()), Question: question})
	}
	return result
}

func validDiagramInterpretation(value *DiagramInterpretation) bool {
	if value == nil || !validRequiredDiagramItems(value.Nodes, maxDiagramItems) ||
		!validDiagramItems(value.Conditions, maxDiagramItems) ||
		!validDiagramItems(value.Branches, maxDiagramItems*2) || len(value.Uncertainties) > maxDiagramItems {
		return false
	}
	seen := map[string]bool{}
	for _, uncertainty := range value.Uncertainties {
		if _, err := ParseID(uncertainty.ID); err != nil || seen[uncertainty.ID] ||
			!validDiagramText(uncertainty.Question) ||
			(uncertainty.Answer != "" && !validDiagramText(uncertainty.Answer)) {
			return false
		}
		seen[uncertainty.ID] = true
	}
	return true
}

func validDiagramText(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= maxDiagramItemRunes
}

func allDiagramUncertaintiesAnswered(value *DiagramInterpretation) bool {
	if !validDiagramInterpretation(value) {
		return false
	}
	for _, uncertainty := range value.Uncertainties {
		if !validDiagramText(uncertainty.Answer) {
			return false
		}
	}
	return true
}
