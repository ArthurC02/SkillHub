package creation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/jackc/pgx/v5"
)

func aChallengeCase() ChallengeCase {
	return ChallengeCase{Name: "日期不存在", Prompt: "請算 2 月 30 日入住的房價", Criteria: []string{"指出 2 月 30 日不存在"}}
}

func challengeCasesWith(n int) []ChallengeCase {
	cases := make([]ChallengeCase, n)
	for i := range cases {
		cases[i] = aChallengeCase()
	}
	return cases
}

func criteriaOf(n int) []string {
	criteria := make([]string, n)
	for i := range criteria {
		criteria[i] = "輸出列出總價"
	}
	return criteria
}

func TestChallengeCasesAreAdmittedUpToEachLimitAndRefusedOnePast(t *testing.T) {
	withCase := func(edit func(*ChallengeCase)) []ChallengeCase {
		c := aChallengeCase()
		edit(&c)
		return []ChallengeCase{c}
	}
	cases := []struct {
		name  string
		cases []ChallengeCase
		rule  string
	}{
		{"no challenge cases", nil, ""},
		{"three cases", challengeCasesWith(MaxChallengeCases), ""},
		{"four cases", challengeCasesWith(MaxChallengeCases + 1), "has 4 challenge cases, more than 3"},
		{"name at the limit", withCase(func(c *ChallengeCase) { c.Name = strings.Repeat("名", MaxChallengeNameRunes) }), ""},
		{"name one past the limit", withCase(func(c *ChallengeCase) { c.Name = strings.Repeat("名", MaxChallengeNameRunes+1) }), "empty or over-long name or prompt"},
		{"blank name", withCase(func(c *ChallengeCase) { c.Name = "  " }), "empty or over-long name or prompt"},
		{"prompt at the limit", withCase(func(c *ChallengeCase) { c.Prompt = strings.Repeat("入", MaxSampleInputRunes) }), ""},
		{"prompt one past the limit", withCase(func(c *ChallengeCase) { c.Prompt = strings.Repeat("入", MaxSampleInputRunes+1) }), "empty or over-long name or prompt"},
		{"blank prompt", withCase(func(c *ChallengeCase) { c.Prompt = "" }), "empty or over-long name or prompt"},
		{"no criteria", withCase(func(c *ChallengeCase) { c.Criteria = nil }), "with 0 criteria, not 1 to 4"},
		{"four criteria", withCase(func(c *ChallengeCase) { c.Criteria = criteriaOf(MaxChallengeCriteria) }), ""},
		{"five criteria", withCase(func(c *ChallengeCase) { c.Criteria = criteriaOf(MaxChallengeCriteria + 1) }), "with 5 criteria, not 1 to 4"},
		{"a blank criterion", withCase(func(c *ChallengeCase) { c.Criteria = []string{" "} }), "empty or over-long acceptance criterion"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := admitReply(&StepResult{Message: "請確認", ChallengeCases: c.cases}, Snapshot{})
			if c.rule == "" {
				if err != nil {
					t.Fatalf("admitReply = %v, want admitted", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidCommand) || !strings.Contains(err.Error(), c.rule) {
				t.Fatalf("admitReply = %v, want ErrInvalidCommand naming %q", err, c.rule)
			}
		})
	}
}

func TestARevisedBriefTakesTheChallengeCasesOfTheReplyThatRevisedIt(t *testing.T) {
	earlier := []ChallengeCase{{Name: "舊的", Prompt: "舊輸入", Criteria: []string{"舊條件"}}}
	cases := []struct {
		name  string
		reply []ChallengeCase
		want  []ChallengeCase
	}{
		{"the reply brings new cases", []ChallengeCase{aChallengeCase()}, []ChallengeCase{aChallengeCase()}},
		{"the reply brings none", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Snapshot{Brief: "舊需求", ChallengeCases: earlier, BriefConfirmed: true}
			r := &StepResult{Brief: "新需求", ChallengeCases: c.reply}
			state := reviseBrief(&p, r, briefChangeIn(p, r))
			if state != StateWaitingConfirmation || p.PendingAction != PendingBriefConfirmation {
				t.Fatalf("state = %s pending = %q, want the brief waiting for confirmation", state, p.PendingAction)
			}
			if !reflect.DeepEqual(p.ChallengeCases, c.want) {
				t.Fatalf("challenge cases = %+v, want %+v", p.ChallengeCases, c.want)
			}
		})
	}
}

type createdTestCase struct {
	name, prompt string
	criteria     []string
}

func TestMaterializingCreatesOneTestCasePerChallengeCaseAfterTheAcceptanceOne(t *testing.T) {
	var created []createdTestCase
	s := &Service{CreateAcceptanceTestCase: func(_ context.Context, _ pgx.Tx, _ identity.Workspace, _, name, prompt string, criteria []string) (string, error) {
		created = append(created, createdTestCase{name, prompt, criteria})
		return "tc-" + string(rune('0'+len(created))), nil
	}}
	p := Snapshot{
		Brief: "算房價", SampleInput: "兩晚", AcceptanceCriteria: []string{"列出總價"},
		ChallengeCases: []ChallengeCase{aChallengeCase(), {Name: "改寫說法", Prompt: "住兩個晚上多少錢", Criteria: []string{"總價同兩晚"}}},
	}
	candidate := Candidate{SkillID: "skill"}

	if err := s.attachAcceptanceTestCase(context.Background(), nil, identity.Workspace{}, p, &candidate); err != nil {
		t.Fatal(err)
	}

	want := []createdTestCase{
		{"創作驗收條件", "兩晚", []string{"列出總價"}},
		{"刁難試跑：日期不存在", "請算 2 月 30 日入住的房價", []string{"指出 2 月 30 日不存在"}},
		{"刁難試跑：改寫說法", "住兩個晚上多少錢", []string{"總價同兩晚"}},
	}
	if !reflect.DeepEqual(created, want) {
		t.Fatalf("created test cases = %+v, want %+v", created, want)
	}
	if candidate.TestCaseID != "tc-1" || !reflect.DeepEqual(candidate.ChallengeTestCaseIDs, []string{"tc-2", "tc-3"}) {
		t.Fatalf("candidate = %+v, want tc-1 for the acceptance case and tc-2, tc-3 for the challenges", candidate)
	}
}

func TestAFailedChallengeTestCaseFailsTheMaterialize(t *testing.T) {
	refused := errors.New("refused")
	s := &Service{CreateAcceptanceTestCase: func(_ context.Context, _ pgx.Tx, _ identity.Workspace, _, name, _ string, _ []string) (string, error) {
		if strings.HasPrefix(name, "刁難試跑") {
			return "", refused
		}
		return "tc", nil
	}}
	p := Snapshot{Brief: "算房價", AcceptanceCriteria: []string{"列出總價"}, ChallengeCases: []ChallengeCase{aChallengeCase()}}

	err := s.attachAcceptanceTestCase(context.Background(), nil, identity.Workspace{}, p, &Candidate{})

	if !errors.Is(err, refused) {
		t.Fatalf("err = %v, want the refusal of the challenge test case", err)
	}
}

func TestChallengeCasesCrossTheWireBothWays(t *testing.T) {
	var sent llmclient.CreationStepRequest
	wire := []llmclient.CreationChallengeCase{{Name: "日期不存在", Prompt: "請算 2 月 30 日入住的房價", Criteria: []string{"指出 2 月 30 日不存在"}}}
	model := stepOverAWire(t, &sent, llmclient.CreationStepResponse{Outcome: "confirm_brief", ChallengeCases: wire})
	req := aStepRequest()
	req.ChallengeCases = []ChallengeCase{aChallengeCase()}

	got, err := model.CreationStep(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(sent.ChallengeCases, wire) {
		t.Errorf("challenge cases sent = %+v, want %+v", sent.ChallengeCases, wire)
	}
	if !reflect.DeepEqual(got.ChallengeCases, []ChallengeCase{aChallengeCase()}) {
		t.Errorf("challenge cases received = %+v, want %+v", got.ChallengeCases, aChallengeCase())
	}
}
