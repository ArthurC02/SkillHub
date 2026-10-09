package operations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/agentloop"
)

type agentEvals struct {
	Snapshot json.RawMessage `json:"snapshot"`
	Cases    []struct {
		Name   string          `json:"name"`
		Passes bool            `json:"passes"`
		Report json.RawMessage `json:"report"`
	} `json:"cases"`
}

const agentEvalsDir = "../../../../../contracts/agents"

func evalsPath(agent string) string {
	return filepath.Join(agentEvalsDir, agent+".evals.json")
}

func TestEveryDefinedAgentHasItsFixedEvals(t *testing.T) {
	for _, def := range Definitions() {
		if _, err := os.Stat(evalsPath(def.Name)); err != nil {
			t.Errorf("agent %q has no fixed evals: %v", def.Name, err)
		}
	}
	files, err := filepath.Glob(filepath.Join(agentEvalsDir, "*.evals.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no evals found in %s: %v", agentEvalsDir, err)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".evals.json")
		if _, ok := Lookup(name); !ok {
			t.Errorf("%s has fixed evals but no agent is defined under that name", name)
		}
	}
}

func TestEachAgentsEvalsAcceptOnlyReportsThatCiteTheSnapshot(t *testing.T) {
	for _, def := range Definitions() {
		t.Run(def.Name, func(t *testing.T) {
			raw, err := os.ReadFile(evalsPath(def.Name))
			if err != nil {
				t.Fatal(err)
			}
			var evals agentEvals
			if err := json.Unmarshal(raw, &evals); err != nil {
				t.Fatal(err)
			}
			if len(evals.Cases) == 0 {
				t.Fatal("the eval file has no cases")
			}
			steps := []agentloop.StepRecord{{ToolCall: agentloop.ToolCall{Tool: def.Tools[0], Arguments: "{}"}, Result: string(evals.Snapshot)}}
			for _, tc := range evals.Cases {
				t.Run(tc.Name, func(t *testing.T) {
					err := def.CheckResult(tc.Report, steps)
					if (err == nil) != tc.Passes {
						t.Errorf("check returned %v, want passes=%v", err, tc.Passes)
					}
				})
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
	steps := []agentloop.StepRecord{
		{ToolCall: agentloop.ToolCall{Tool: ToolMaintenanceReport}, Result: "not json"},
		{ToolCall: agentloop.ToolCall{Tool: ToolMaintenanceReport}, Result: `{"x":1,"jobs":{"a":{"action":"run-a"}},"actions":["run-b"]}`},
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
		{"unknown field", `{"items":[],"summary":"x"}`, "not a report of cited items"},
		{"not an object", `[]`, "not a report of cited items"},
		{"second item is wrong", `{"items":[{"status":"fine","text":"ok","cites":["/x"]},{"status":"fine","text":"ok","cites":["/y"]}]}`, "item 2"},
		{"a proposal citing a fact", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-a","reason":"late","cites":["/x"]}]}`, ""},
		{"a proposal with no action", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":" ","reason":"late","cites":["/x"]}]}`, "names no action"},
		{"a proposal with no reason", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-a","reason":"","cites":["/x"]}]}`, "gives no reason"},
		{"a proposal citing nothing", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-a","reason":"late"}]}`, "proposal 1"},
		{"a proposal no fact offers", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-c","reason":"late","cites":["/x"]}]}`, `proposes "run-c", which no returned fact offers`},
		{"a proposal whose name is a fact but not an offer", `{"items":[{"status":"fine","text":"ok","cites":["/x"]}],"proposals":[{"action":"run-b","reason":"late","cites":["/x"]}]}`, `proposes "run-b"`},
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
