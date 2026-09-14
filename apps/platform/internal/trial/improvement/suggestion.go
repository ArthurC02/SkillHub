package eval

type SuggestionCategory string

const (
	SuggestionSkill   SuggestionCategory = "skill"
	SuggestionRuntime SuggestionCategory = "runtime"
	SuggestionMCP     SuggestionCategory = "mcp"
	SuggestionTool    SuggestionCategory = "tool"
	SuggestionDataset SuggestionCategory = "dataset"
)

func AllSuggestionCategories() []SuggestionCategory {
	return []SuggestionCategory{SuggestionSkill, SuggestionRuntime, SuggestionMCP, SuggestionTool, SuggestionDataset}
}

func (c SuggestionCategory) actionable() bool {
	switch c {
	case SuggestionSkill, SuggestionRuntime, SuggestionTool, SuggestionDataset:
		return true
	}
	return false
}

type Decision string

const (
	DecisionPending  Decision = "pending"
	DecisionAccepted Decision = "accepted"
	DecisionRejected Decision = "rejected"
)

func AllDecisions() []Decision {
	return []Decision{DecisionPending, DecisionAccepted, DecisionRejected}
}

func (d Decision) chosen() bool { return d == DecisionAccepted || d == DecisionRejected }
