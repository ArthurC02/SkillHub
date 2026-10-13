package eval

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/judging"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

func TestRequireTestLabDoesNotInspectOwnerInternals(t *testing.T) {
	if err := (&Service{TestLab: &testlab.Service{}}).requireTestLab(); err != nil {
		t.Fatalf("injected Test Lab owner was rejected because of its private configuration: %v", err)
	}
}

const eventID = "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01"

func TestRunReadersFailClosedAndHideMissingRuns(t *testing.T) {
	ctx := context.Background()
	var svc Service
	if _, err := svc.runFacts(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, errRunReaderNotConfigured) {
		t.Errorf("run facts without owner reader: %v", err)
	}
	if _, err := svc.gather(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, errRunReaderNotConfigured) {
		t.Errorf("evaluation input without owner reader: %v", err)
	}

	svc.ReadRunFacts = func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Run, bool, error) {
		return facts.Run{}, false, nil
	}
	if _, err := svc.runFacts(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing run facts: %v", err)
	}
	svc.ReadEvaluationInput = func(context.Context, pgtype.UUID, pgtype.UUID) (facts.EvaluationInput, bool, error) {
		return facts.EvaluationInput{}, false, nil
	}
	if _, err := svc.gather(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, errRegistryReadNotConfigured) {
		t.Errorf("evaluation input without Registry readers: %v", err)
	}
	svc.ReadVersion = func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Version, bool, error) {
		return facts.Version{}, false, nil
	}
	svc.ReadSkill = func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Skill, bool, error) {
		return facts.Skill{}, false, nil
	}
	svc.ReadRuntimeCompatibility = func(context.Context, pgtype.UUID) (facts.RuntimeCompatibility, bool, error) {
		return facts.RuntimeCompatibility{}, false, nil
	}
	if _, err := svc.gather(ctx, pgtype.UUID{}, pgtype.UUID{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing evaluation input: %v", err)
	}
}

func TestOverallIsRecomputedFromTheStoredCriteria(t *testing.T) {
	res := func(results ...string) []verdict.CriterionResult {
		out := make([]verdict.CriterionResult, 0, len(results))
		for i, r := range results {
			out = append(out, verdict.CriterionResult{CriterionID: string(rune('a' + i)), Result: r})
		}
		return out
	}
	cases := []struct {
		name string
		in   []verdict.CriterionResult
		want verdict.Overall
	}{
		{"no criteria at all", nil, verdict.OverallUndetermined},
		{"all passed", res(verdict.ResultPassed, verdict.ResultPassed), verdict.OverallMet},
		{"all failed", res(verdict.ResultFailed, verdict.ResultFailed), verdict.OverallNotMet},
		{"some of each", res(verdict.ResultPassed, verdict.ResultFailed), verdict.OverallPartiallyMet},
		{"a pass and an unknown", res(verdict.ResultPassed, verdict.ResultUndetermined), verdict.OverallPartiallyMet},
		{"a failure and an unknown, nothing passed", res(verdict.ResultFailed, verdict.ResultUndetermined), verdict.OverallNotMet},
		{"nothing but unknowns", res(verdict.ResultUndetermined, verdict.ResultUndetermined), verdict.OverallUndetermined},
	}
	for _, c := range cases {
		if got := verdict.OverallFrom(c.in); got != c.want {
			t.Errorf("%s: overallFrom = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEvaluateLeavesARunThatHasNotFinishedAlone(t *testing.T) {
	id := gen.Workspace{}.ID
	for _, tc := range []struct {
		name     string
		finished bool
		wantErr  bool
	}{
		{"a run still going is skipped", false, false},
		{"a finished run goes on to read its test case", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				ReadEvaluationInput: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.EvaluationInput, bool, error) {
					return facts.EvaluationInput{Run: facts.Run{Terminal: tc.finished}}, true, nil
				},
				ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Version, bool, error) {
					return facts.Version{}, true, nil
				},
				ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Skill, bool, error) {
					return facts.Skill{}, true, nil
				},
				ReadRuntimeCompatibility: func(context.Context, pgtype.UUID) (facts.RuntimeCompatibility, bool, error) {
					return facts.RuntimeCompatibility{}, false, nil
				},
			}
			if err := svc.Evaluate(t.Context(), id, id); (err != nil) != tc.wantErr {
				t.Fatalf("Evaluate error = %v, want an error: %v", err, tc.wantErr)
			}
		})
	}
}

func TestTheStartedEventRecordsAMissingRubricAsNull(t *testing.T) {
	if got := rubricVersion(nil); got != nil {
		t.Errorf("the started event says null, never an empty string, got %#v", got)
	}
	if got := rubricVersion(&testlab.Rubric{Version: "v1"}); got != "v1" {
		t.Errorf("rubricVersion = %#v, want v1", got)
	}
}

type rolesAsked struct {
	mu    sync.Mutex
	roles []string
	req   modelport.JudgeRequest
}

func (r *rolesAsked) JudgeRun(_ context.Context, req modelport.JudgeRequest) (*modelport.Judgement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles, r.req = append(r.roles, req.ModelRole), req
	cost := 0.25
	return &modelport.Judgement{
		Model: "m", PromptVersion: "p",
		Criteria: []modelport.CriterionVerdict{{CriterionID: "c1", Result: verdict.ResultPassed, Reason: "said so",
			Citations: []modelport.Citation{{Kind: verdict.KindAgentOutput, Quote: "Removed 17 duplicate rows"}}}},
		Usage: &modelport.ModelUsage{CostUSD: &cost, CostReported: true},
	}, nil
}

func TestTheEvaluationAsksTheJudgesItsServiceWasGiven(t *testing.T) {
	judge := &rolesAsked{}
	on := func(context.Context) (bool, error) { return true, nil }
	m := facts.Material{
		Criteria: []testlab.Criterion{{ID: "c1", Text: "the duplicates are removed"}},
		Advanced: trace.AdvancedView{Complete: true},
		Summary:  trace.Summary{FinalOutput: "Removed 17 duplicate rows"},
	}
	ev := gen.Evaluation{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}

	v, err := (&Service{Judge: judge, PanelEnabled: on}).judge(context.Background(), m, ev)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(judge.roles)
	if !slices.Equal(judge.roles, judging.PanelRoles) {
		t.Errorf("asked %q, want the whole panel", judge.roles)
	}
	if judge.req.EvaluationID != pgconv.UUIDString(ev.ID) || judge.req.Within != time.Duration(JudgeBudget.Ceiling())*time.Second {
		t.Errorf("request carried evaluation %q within %v", judge.req.EvaluationID, judge.req.Within)
	}
	if v.model != "m + m + m" || v.costUSD == nil || *v.costUSD != 0.75 || v.usage == nil {
		t.Errorf("assessment = %+v, want the panel members and summed cost", v)
	}
}
