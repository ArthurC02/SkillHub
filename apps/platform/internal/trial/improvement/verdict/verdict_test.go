package verdict

import (
	"encoding/json"
	"testing"
)

func TestTheStoredShapeOfACriterionResultAndAFindingDoesNotChange(t *testing.T) {
	evidence := EvidenceRef{
		Kind: KindArtifact, Match: MatchExact, ReattributedFrom: "trace_event", TraceEventID: "e1",
		OccurredAt: "2026-10-13T00:00:00Z", ArtifactPath: "out.md", ByteRange: &Range{Start: 1, End: 4},
		CharRange: &Range{Start: 1, End: 3}, Excerpt: "abc", ExcerptTruncated: true, Available: true,
	}
	result := CriterionResult{
		CriterionID: "c1", Text: "says hello", Result: ResultPassed, Source: SourceRule,
		Evidence: []EvidenceRef{evidence}, Reason: "it did",
		Steps: []CheckStep{{Path: "0", Kind: "contains", Result: ResultPassed, Reason: "found"}},
	}
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"criterion result", result, `{"criterion_id":"c1","text":"says hello","result":"passed","source":"rule",` +
			`"evidence":[{"kind":"artifact","match":"exact","reattributed_from":"trace_event","trace_event_id":"e1",` +
			`"occurred_at":"2026-10-13T00:00:00Z","artifact_path":"out.md","byte_range":{"start":1,"end":4},` +
			`"char_range":{"start":1,"end":3},"excerpt":"abc","excerpt_truncated":true,"available":true}],` +
			`"reason":"it did","steps":[{"path":"0","kind":"contains","result":"passed","reason":"found"}]}`},
		{"result without steps or optional evidence fields", CriterionResult{
			CriterionID: "c2", Result: ResultUndetermined, Source: SourceModel, Evidence: []EvidenceRef{{Kind: KindAgentOutput}},
		}, `{"criterion_id":"c2","text":"","result":"undetermined","source":"model",` +
			`"evidence":[{"kind":"agent_output","excerpt":"","excerpt_truncated":false,"available":false}],"reason":""}`},
		{"finding without evidence", NewFinding(CategoryCost, SeverityWarning, "costly"),
			`{"category":"cost","severity":"warning","message":"costly","evidence":[]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := json.Marshal(c.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("stored as\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}
