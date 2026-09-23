package creation

import (
	"strings"
	"unicode/utf8"
)

const maxDiagramItems = 64

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

func validDiagramDescription(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= 2000
}

func validDiagramItems(items []string, limit int, required bool) bool {
	if (required && len(items) == 0) || len(items) > limit {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == "" || utf8.RuneCountInString(item) > 2000 {
			return false
		}
	}
	return true
}

func validDiagramDecomposition(value *DiagramDecomposition) bool {
	return value != nil &&
		validDiagramItems(value.Nodes, maxDiagramItems, true) &&
		validDiagramItems(value.Conditions, maxDiagramItems, false) &&
		validDiagramItems(value.Branches, maxDiagramItems*2, false) &&
		validDiagramItems(value.Uncertainties, maxDiagramItems, false)
}

func newDiagramInterpretation(value *DiagramDecomposition) *DiagramInterpretation {
	if !validDiagramDecomposition(value) {
		return nil
	}
	result := &DiagramInterpretation{
		Nodes:      append([]string(nil), value.Nodes...),
		Conditions: append([]string(nil), value.Conditions...),
		Branches:   append([]string(nil), value.Branches...),
	}
	for _, question := range value.Uncertainties {
		result.Uncertainties = append(result.Uncertainties, DiagramUncertainty{ID: UUID(newID()), Question: question})
	}
	return result
}

func validDiagramInterpretation(value *DiagramInterpretation) bool {
	if value == nil || !validDiagramItems(value.Nodes, maxDiagramItems, true) ||
		!validDiagramItems(value.Conditions, maxDiagramItems, false) ||
		!validDiagramItems(value.Branches, maxDiagramItems*2, false) || len(value.Uncertainties) > maxDiagramItems {
		return false
	}
	seen := map[string]bool{}
	for _, uncertainty := range value.Uncertainties {
		if _, err := ParseID(uncertainty.ID); err != nil || seen[uncertainty.ID] ||
			!validDiagramDescription(uncertainty.Question) ||
			(uncertainty.Answer != "" && !validDiagramAnswer(uncertainty.Answer)) {
			return false
		}
		seen[uncertainty.ID] = true
	}
	return true
}

func validDiagramAnswer(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= 2000
}

func allDiagramUncertaintiesAnswered(value *DiagramInterpretation) bool {
	return validDiagramInterpretation(value) &&
		all(value.Uncertainties, func(uncertainty DiagramUncertainty) bool { return validDiagramAnswer(uncertainty.Answer) })
}

func all[T any](values []T, predicate func(T) bool) bool {
	for _, value := range values {
		if !predicate(value) {
			return false
		}
	}
	return true
}
