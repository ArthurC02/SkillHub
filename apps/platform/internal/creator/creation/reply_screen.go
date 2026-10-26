package creation

import (
	"slices"
	"strings"
)

const (
	reasonToolUnavailable     = "tool_unavailable"
	reasonConfirmDiagramFirst = "confirm_diagram_first"
	reasonConfirmBriefFirst   = "confirm_brief_first"
	reasonSearchQueryMissing  = "search_query_missing"
	reasonFetchURLMissing     = "fetch_url_missing"
	reasonBriefMissing        = "brief_missing"
	reasonDraftMissing        = "draft_missing"
)

var screenReasons = []string{
	reasonToolUnavailable, reasonConfirmDiagramFirst, reasonConfirmBriefFirst,
	reasonSearchQueryMissing, reasonFetchURLMissing, reasonBriefMissing, reasonDraftMissing,
}

func offeredTools(s *Service, e envelope) []string {
	p := e.Snapshot
	return allowedTools(p.ToolCalls, e.Limits.MaxToolCalls, toolAvailability{fetch: s.Fetch != nil, knowledge: s.SearchKnowledge != nil, searchLeft: p.SearchRounds < MaxSearchRounds})
}

func screenReply(e envelope, r *StepResult, allowed []string) {
	if r.Reason != "" {
		return
	}
	p := e.Snapshot
	if asksToValidate(r) && r.Draft == nil {
		r.Draft, _ = draftForModel(p.Draft, e.PreviousDraft)
	}
	dropUnproposedBriefChanges(p, r)
	if reason, outcome := refusalOf(p, r, allowed); reason != "" {
		r.Reason, r.Outcome, r.Draft, r.ToolIntent = reason, outcome, nil, nil
	}
}

func asksToValidate(r *StepResult) bool {
	return r.Outcome == outcomeToolIntent && r.ToolIntent != nil && r.ToolIntent.Kind == validateDraftTool
}

func submitsDraft(r *StepResult) bool {
	return r.Outcome == outcomeDraft || asksToValidate(r)
}

func dropUnproposedBriefChanges(p Snapshot, r *StepResult) {
	if !p.BriefConfirmed || r.Outcome == outcomeConfirmBrief || (submitsDraft(r) && briefChangeIn(p, r).brief) {
		return
	}
	r.Brief, r.AcceptanceCriteria, r.SampleInput = "", nil, ""
}

func refusalOf(p Snapshot, r *StepResult, allowed []string) (reason, outcome string) {
	switch r.Outcome {
	case outcomeConfirmBrief:
		proposed := r.Brief
		if proposed == "" {
			proposed = p.Brief
		}
		if strings.TrimSpace(proposed) == "" || (p.BriefConfirmed && !briefChangeIn(p, r).any()) {
			return reasonBriefMissing, outcomeClarification
		}
	case outcomeDraft:
		return draftRefusal(p, r)
	case outcomeToolIntent:
		return toolRefusal(p, r, allowed)
	}
	return "", ""
}

func draftRefusal(p Snapshot, r *StepResult) (reason, outcome string) {
	switch {
	case !diagramSettled(p):
		return reasonConfirmDiagramFirst, outcomeClarification
	case !p.BriefConfirmed || strings.TrimSpace(p.Brief) == "" || briefChangeIn(p, r).brief:
		return reasonConfirmBriefFirst, outcomeConfirmBrief
	case r.Draft == nil:
		return reasonDraftMissing, outcomeClarification
	}
	return "", ""
}

func toolRefusal(p Snapshot, r *StepResult, allowed []string) (reason, outcome string) {
	t := r.ToolIntent
	switch {
	case t == nil || !slices.Contains(allowed, t.Kind):
		return reasonToolUnavailable, outcomeClarification
	case t.Kind == fetchURLTool && !namesAWebAddress(t.Query):
		return reasonFetchURLMissing, outcomeClarification
	case (t.Kind == searchCatalogTool || t.Kind == searchKnowledgeTool) && strings.TrimSpace(t.Query) == "":
		return reasonSearchQueryMissing, outcomeClarification
	case t.Kind == validateDraftTool:
		return draftRefusal(p, r)
	}
	return "", ""
}

func namesAWebAddress(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	return strings.HasPrefix(q, "http://") || strings.HasPrefix(q, "https://")
}
