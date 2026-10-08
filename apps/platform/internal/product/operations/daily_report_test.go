package operations

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type dailyReportEvals struct {
	Snapshot json.RawMessage `json:"snapshot"`
	Cases    []struct {
		Name   string          `json:"name"`
		Passes bool            `json:"passes"`
		Report json.RawMessage `json:"report"`
	} `json:"cases"`
}

func TestDailyReportEvalsAcceptOnlyReportsThatCiteTheSnapshot(t *testing.T) {
	raw, err := os.ReadFile("testdata/daily_report_evals.json")
	if err != nil {
		t.Fatal(err)
	}
	var evals dailyReportEvals
	if err := json.Unmarshal(raw, &evals); err != nil {
		t.Fatal(err)
	}
	if len(evals.Cases) == 0 {
		t.Fatal("the eval file has no cases")
	}
	steps := []StepRecord{{ToolCall: ToolCall{Tool: ToolMaintenanceReport, Arguments: "{}"}, Result: string(evals.Snapshot)}}
	for _, tc := range evals.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			err := CitesOnlyReturnedFacts(tc.Report, steps)
			if (err == nil) != tc.Passes {
				t.Errorf("check returned %v, want passes=%v", err, tc.Passes)
			}
		})
	}
}

func TestACiteResolvesOnlyToAFactTheDocumentHolds(t *testing.T) {
	var doc any
	if err := json.Unmarshal([]byte(`{"a":{"b/c":1,"d~e":2,"list":[10,20]},"null":null}`), &doc); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		pointer string
		want    bool
	}{
		{"/a", true},
		{"/null", true},
		{"/a/b~1c", true},
		{"/a/d~0e", true},
		{"/a/list/0", true},
		{"/a/list/1", true},
		{"/a/list/2", false},
		{"/a/list/-1", false},
		{"/a/list/01", false},
		{"/a/list/x", false},
		{"/a/b/c", false},
		{"/a/missing", false},
		{"/a/list/0/deeper", false},
		{"", false},
		{"a", false},
	}
	for _, tc := range cases {
		if _, got := valueAt(tc.pointer, doc); got != tc.want {
			t.Errorf("valueAt(%q) found = %v, want %v", tc.pointer, got, tc.want)
		}
	}
}

func TestADailyReportMustBeWellFormedBeforeItsCitesCount(t *testing.T) {
	steps := []StepRecord{
		{ToolCall: ToolCall{Tool: ToolMaintenanceReport}, Result: "not json"},
		{ToolCall: ToolCall{Tool: ToolMaintenanceReport}, Result: `{"x":1}`},
	}
	cases := []struct {
		name   string
		result string
		want   string
	}{
		{"cites the second tool answer", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}]}`, ""},
		{"attention is a status", `{"items":[{"status":"attention","text":"look","cites":["/x"]}]}`, ""},
		{"no items", `{"items":[]}`, "no items"},
		{"missing items", `{}`, "no items"},
		{"unknown status", `{"items":[{"status":"warning","text":"ok","cites":["/x"]}]}`, `status "warning"`},
		{"blank text", `{"items":[{"status":"fine","text":"  ","cites":["/x"]}]}`, "says nothing"},
		{"no cites", `{"items":[{"status":"fine","text":"ok"}]}`, "cites no fact"},
		{"unknown field", `{"items":[],"summary":"x"}`, "not a daily report"},
		{"not an object", `[]`, "not a daily report"},
		{"second item is wrong", `{"items":[{"status":"fine","text":"ok","cites":["/x"]},{"status":"fine","text":"ok","cites":["/y"]}]}`, "item 2"},
		{"a proposal citing a fact", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-a","reason":"late","cites":["/x"]}]}`, ""},
		{"a proposal with no action", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":" ","reason":"late","cites":["/x"]}]}`, "names no action"},
		{"a proposal with no reason", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-a","reason":"","cites":["/x"]}]}`, "gives no reason"},
		{"a proposal citing nothing", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-a","reason":"late"}]}`, "proposal 1"},
		{"a proposal citing a missing fact", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-a","reason":"late","cites":["/y"]}]}`, `cites "/y"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CitesOnlyReturnedFacts(json.RawMessage(tc.result), steps)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("got %v, want it accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error naming %q", err, tc.want)
			}
		})
	}
}
