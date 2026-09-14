package creation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type modelFunc func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error)

func (f modelFunc) CreationStep(ctx context.Context, r llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
	return f(ctx, r)
}

func TestAnAttemptIsRefusedForTheFirstLimitItHits(t *testing.T) {
	past, future := time.Now().Add(-time.Second), time.Now().Add(time.Hour)
	spent := Snapshot{Steps: testLimits().MaxSteps}
	unread := Snapshot{DiagramFingerprint: "fp", BudgetUSD: 1}
	room := Snapshot{BudgetUSD: 1}
	for _, c := range []struct {
		name       string
		e          envelope
		hasDiagram bool
		want       string
	}{
		{"deadline passed", envelope{Deadline: past, Limits: testLimits(), Snapshot: spent}, false, "創作已達這次核准的限制，請開始新的創作。"},
		{"no steps left", envelope{Deadline: future, Limits: testLimits(), Snapshot: spent}, false, "已達這次核准的步數上限，請開始新的創作。"},
		{"diagram never read", envelope{Deadline: future, Limits: testLimits(), Snapshot: unread}, false, "流程圖需要重新上傳。"},
		{"diagram arriving with the attempt", envelope{Deadline: future, Limits: testLimits(), Snapshot: unread}, true, ""},
		{"nothing in the way", envelope{Deadline: future, Limits: testLimits(), Snapshot: room}, false, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := attemptRefusal(c.e, c.hasDiagram); got != c.want {
				t.Fatalf("refusal = %q, want %q", got, c.want)
			}
		})
	}
}

func TestOnlyATransientAttemptLearnsItWasStale(t *testing.T) {
	if err := staleAttempt(true); !errors.Is(err, ErrConflict) {
		t.Errorf("transient: err = %v, want ErrConflict", err)
	}
	if err := staleAttempt(false); err != nil {
		t.Errorf("queued job: err = %v, want nil", err)
	}
}

func TestAFailedAttemptLandsWhereThePersonCanActOnIt(t *testing.T) {
	for _, c := range []struct {
		name          string
		understanding string
		hadDiagram    bool
		err, callErr  error
		want          State
		pending       string
		last          string
	}{
		{"the reply broke the rules", "", false, ErrInvalidCommand, nil, StateFailed, "", "模型的回覆不符合會話規則"},
		{"a diagram step that was never read", "", true, ErrInvalidCommand, nil, StateNeedsReupload, "", "模型的回覆不符合會話規則"},
		{"a diagram step that was read", understoodDiagram, true, ErrUnavailable, errors.New("timeout"), StateFailed, "", "平台這一側沒能完成這次模型呼叫"},
		{"a reference went away", "", false, ErrUnavailable, fmt.Errorf("resolve: %w", ErrNotFound), StateWaitingConfirmation, "confirm_references", "參考內容目前不可用，請換選後再確認。"},
		{"the balance hit the floor", "", false, ErrUnavailable, ErrCreditFloor, StateWaitingInput, "", "帳戶餘額已達可容忍的欠款上限，請充值後再繼續這場創作。"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := &Snapshot{DiagramUnderstanding: c.understanding, PendingAction: "confirm_brief", References: []Reference{{SkillID: "a", Confirmed: true, Available: true}}}
			got := failedAttempt(p, c.err, c.callErr, c.hadDiagram)
			if got != c.want || p.PendingAction != c.pending || !strings.Contains(p.Messages[len(p.Messages)-1].Content, c.last) {
				t.Fatalf("state = %s, pending = %q, messages = %+v", got, p.PendingAction, p.Messages)
			}
			refused := errors.Is(c.callErr, ErrNotFound)
			if r := p.References[0]; refused == (r.Confirmed || r.Available) {
				t.Fatalf("reference = %+v after callErr %v", r, c.callErr)
			}
		})
	}
}

func TestTheModelSeesTheCurrentDraftWithItsReportOrElseThePreviousOne(t *testing.T) {
	current := &Draft{ContentHash: "h", Blocked: true, Validation: "report", Skill: llmclient.GeneratedSkill{Name: "now"}}
	previous := &Draft{Skill: llmclient.GeneratedSkill{Name: "before"}}
	if skill, validation := draftForModel(nil, nil); skill != nil || validation != nil {
		t.Errorf("no draft at all: %+v %+v", skill, validation)
	}
	if skill, validation := draftForModel(nil, previous); skill == nil || skill.Name != "before" || validation != nil {
		t.Errorf("only a previous draft: %+v %+v", skill, validation)
	}
	skill, validation := draftForModel(current, previous)
	if skill == nil || skill.Name != "now" || validation == nil || validation.ContentHash != "h" || !validation.Blocked || validation.Report != "report" {
		t.Errorf("a current draft: %+v %+v", skill, validation)
	}
}

func TestTheValidationReportIsCutToTheTextLimitWithAMarker(t *testing.T) {
	fits := strings.Repeat("字", MaxTextRunes)
	if got := truncatedReport(fits); got != fits {
		t.Fatalf("a report at the limit was changed")
	}
	got := []rune(truncatedReport(fits + "字"))
	if len(got) != MaxTextRunes || !strings.HasSuffix(string(got), "\n[findings truncated]") {
		t.Fatalf("a report one rune over: %d runes, suffix %q", len(got), string(got[len(got)-25:]))
	}
}

func TestOnlyAFiniteNonNegativeCostIsKnown(t *testing.T) {
	cost := func(v float64) *llmclient.GatewayUsage { return &llmclient.GatewayUsage{CostUSD: &v} }
	for _, c := range []struct {
		name  string
		usage *llmclient.GatewayUsage
		want  *int64
	}{
		{"no usage", nil, nil},
		{"no cost", &llmclient.GatewayUsage{}, nil},
		{"not a number", cost(math.NaN()), nil},
		{"infinite", cost(math.Inf(1)), nil},
		{"negative", cost(-.01), nil},
		{"zero", cost(0), new(int64)},
		{"positive", cost(.0123), func() *int64 { v := int64(12300); return &v }()},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := knownCostMicros(c.usage)
			if (got == nil) != (c.want == nil) || got != nil && *got != *c.want {
				t.Fatalf("micros = %v, want %v", got, c.want)
			}
		})
	}
}

func TestTheStepRequestAlwaysCarriesAReferenceList(t *testing.T) {
	s := &Service{}
	e := envelope{Limits: testLimits(), Snapshot: Snapshot{Brief: "b"}}
	req := s.stepRequest(JobArgs{}, 7, e, nil)
	if req.References == nil || len(req.References) != 0 || req.Revision != 7 || req.Brief != "b" || req.MaxOutputTokens != testLimits().MaxOutputTokens {
		t.Fatalf("request = %+v", req)
	}
}

func callerFor(t *testing.T, calls *int) *Service {
	t.Helper()
	cost := .03
	return &Service{
		LLM: modelFunc(func(_ context.Context, r llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
			*calls++
			if r.GatewayKey != "key" || r.TimeoutSeconds < 1 {
				t.Errorf("request = %+v", r)
			}
			return &llmclient.CreationStepResponse{Outcome: "clarification", Usage: &llmclient.GatewayUsage{CostUSD: &cost}}, nil
		}),
		IssueKey: func(context.Context, string, string, float64, time.Duration) (string, error) { return "key", nil },
		ResolveReference: func(_ context.Context, _ identity.Workspace, id, _ string) (Reference, llmclient.GenerateReference, error) {
			if id == "gone" {
				return Reference{}, llmclient.GenerateReference{}, errors.New("gone")
			}
			return Reference{}, llmclient.GenerateReference{Name: id}, nil
		},
	}
}

func TestAModelCallCarriesTheConfirmedReferencesAndReportsItsUsage(t *testing.T) {
	calls := 0
	s := callerFor(t, &calls)
	var sent []string
	inner := s.LLM
	s.LLM = modelFunc(func(ctx context.Context, r llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		for _, ref := range r.References {
			sent = append(sent, ref.Name)
		}
		return inner.CreationStep(ctx, r)
	})
	e := envelope{Limits: testLimits(), Snapshot: Snapshot{References: []Reference{{SkillID: "a", Confirmed: true}, {SkillID: "b", Confirmed: true}}}}
	response, usage, err := s.callModel(context.Background(), JobArgs{}, e, s.stepRequest(JobArgs{}, 1, e, nil), time.Now().Add(time.Minute))
	if err != nil || response == nil || usage == nil || *usage.CostUSD != .03 || calls != 1 || strings.Join(sent, ",") != "a,b" {
		t.Fatalf("response = %+v, usage = %+v, calls = %d, sent = %v, err = %v", response, usage, calls, sent, err)
	}
}

func TestAModelCallThatNeverStartsSettlesAsAKnownZero(t *testing.T) {
	for _, c := range []struct {
		name     string
		refs     []Reference
		change   func(*Service)
		deadline time.Duration
		want     error
	}{
		{"an unconfirmed reference", []Reference{{SkillID: "a"}}, func(*Service) {}, time.Minute, ErrNotFound},
		{"a reference that no longer resolves", []Reference{{SkillID: "gone", Confirmed: true}}, func(*Service) {}, time.Minute, ErrNotFound},
		{"no resolver for a reference", []Reference{{SkillID: "a", Confirmed: true}}, func(s *Service) { s.ResolveReference = nil }, time.Minute, ErrNotFound},
		{"the balance is at the floor", nil, func(s *Service) {
			s.CreditReserve = func(context.Context, pgtype.UUID, int64) (bool, error) { return false, nil }
		}, time.Minute, ErrCreditFloor},
		{"the reservation failed", nil, func(s *Service) {
			s.CreditReserve = func(context.Context, pgtype.UUID, int64) (bool, error) { return false, errKeyRefused }
		}, time.Minute, errKeyRefused},
		{"the gateway key was refused", nil, func(s *Service) {
			s.IssueKey = func(context.Context, string, string, float64, time.Duration) (string, error) {
				return "", errKeyRefused
			}
		}, time.Minute, errKeyRefused},
		{"too little time left", nil, func(*Service) {}, 5 * time.Second, ErrUnavailable},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			s := callerFor(t, &calls)
			c.change(s)
			e := envelope{Limits: testLimits(), Snapshot: Snapshot{References: c.refs}}
			response, usage, err := s.callModel(context.Background(), JobArgs{}, e, s.stepRequest(JobArgs{}, 1, e, nil), time.Now().Add(c.deadline))
			if !errors.Is(err, c.want) || response != nil || calls != 0 {
				t.Fatalf("response = %+v, calls = %d, err = %v, want %v", response, calls, err, c.want)
			}
			if usage == nil || usage.CostUSD == nil || *usage.CostUSD != 0 {
				t.Fatalf("usage = %+v, want a known zero", usage)
			}
		})
	}
}

var errKeyRefused = errors.New("refused")

func TestAModelCallThatBringsBackNothingSettlesAsUnknownCost(t *testing.T) {
	calls := 0
	s := callerFor(t, &calls)
	s.LLM = modelFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		return nil, context.DeadlineExceeded
	})
	e := envelope{Limits: testLimits()}
	_, usage, err := s.callModel(context.Background(), JobArgs{}, e, s.stepRequest(JobArgs{}, 1, e, nil), time.Now().Add(time.Minute))
	if !errors.Is(err, context.DeadlineExceeded) || usage != nil {
		t.Fatalf("usage = %+v, err = %v", usage, err)
	}
}

func TestAPendingFetchIsCarriedOutOnceAndReported(t *testing.T) {
	var fetched []string
	s := &Service{
		Fetch: func(_ context.Context, url string) (Fetch, string) {
			fetched = append(fetched, url)
			return Fetch{URL: url, Status: "fetched"}, "secret page"
		},
		Mask: func(v string) string { return strings.ReplaceAll(v, "secret", "***") },
	}
	p := &Snapshot{PendingFetchURL: "https://example.com/a"}
	s.fetchPending(context.Background(), p)
	s.fetchPending(context.Background(), p)
	if strings.Join(fetched, ",") != "https://example.com/a" || p.PendingFetchURL != "" || len(p.Fetches) != 1 {
		t.Fatalf("fetched = %v, snapshot = %+v", fetched, p)
	}
	if want := fetchObservation(p.Fetches[0], "*** page"); len(p.Messages) != 1 || p.Messages[0].Role != "tool" || p.Messages[0].Content != want {
		t.Fatalf("messages = %+v", p.Messages)
	}
}

func TestWithoutAFetcherAPendingFetchWaits(t *testing.T) {
	p := &Snapshot{PendingFetchURL: "https://example.com/a"}
	(&Service{}).fetchPending(context.Background(), p)
	if p.PendingFetchURL != "https://example.com/a" || len(p.Messages) != 0 {
		t.Fatalf("snapshot = %+v", p)
	}
}

func liveRow(revision int64) gen.CreationSession {
	return gen.CreationSession{Revision: revision, State: string(StateWorking), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}}
}

func TestAnAttemptWithoutAUsableResponseIsUnavailable(t *testing.T) {
	ok := &llmclient.CreationStepResponse{Outcome: "clarification", Message: "?"}
	expired := liveRow(2)
	expired.ExpiresAt.Time = time.Now().Add(-time.Second)
	for _, c := range []struct {
		name     string
		row      gen.CreationSession
		deadline time.Time
		response *llmclient.CreationStepResponse
		callErr  error
	}{
		{"the call failed", liveRow(2), time.Now().Add(time.Hour), ok, errors.New("boom")},
		{"no response", liveRow(2), time.Now().Add(time.Hour), nil, nil},
		{"the session expired", expired, time.Now().Add(time.Hour), ok, nil},
		{"the deadline passed", liveRow(2), time.Now().Add(-time.Second), ok, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &envelope{Deadline: c.deadline, Limits: testLimits(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}}}
			if _, _, err := (&Service{}).attemptOutcome(context.Background(), JobArgs{}, c.row, e, c.response, c.callErr, false); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestADiagramAttemptMustComeBackWithAnUnderstanding(t *testing.T) {
	e := &envelope{Deadline: time.Now().Add(time.Hour), Limits: testLimits()}
	response := &llmclient.CreationStepResponse{Outcome: "clarification", Message: "?"}
	if _, _, err := (&Service{}).attemptOutcome(context.Background(), JobArgs{}, liveRow(2), e, response, nil, true); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("err = %v, want ErrInvalidCommand", err)
	}
}

func TestAUsableResponseIsJudgedAtTheNextRevision(t *testing.T) {
	s := &Service{ValidateDraft: func(context.Context, llmclient.GeneratedSkill) (string, string, bool, error) {
		return "h", "{}", false, nil
	}}
	e := &envelope{Deadline: time.Now().Add(time.Hour), Limits: testLimits(), Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, Brief: "b", BriefConfirmed: true, BudgetUSD: 1}}
	response := &llmclient.CreationStepResponse{Outcome: "draft", Message: "draft", Brief: "b", Draft: &llmclient.GeneratedSkill{Name: "x", Body: "body"}}
	state, next, err := s.attemptOutcome(context.Background(), JobArgs{}, liveRow(6), e, response, nil, false)
	if err != nil || state != StateDraftReady || next || e.Snapshot.Draft == nil || e.Snapshot.Draft.Revision != 7 {
		t.Fatalf("state = %s, next = %v, draft = %+v, err = %v", state, next, e.Snapshot.Draft, err)
	}
}

func TestAnAttemptThatCannotBeJudgedFailsAndHandsTheTurnBack(t *testing.T) {
	e := &envelope{Deadline: time.Now().Add(time.Hour), Limits: testLimits(), Snapshot: Snapshot{PendingAction: "confirm_brief"}}
	state, next := (&Service{}).concludeAttempt(context.Background(), JobArgs{}, liveRow(2), e, nil, ErrCreditFloor, false)
	if state != StateWaitingInput || next || e.Snapshot.PendingAction != "" {
		t.Fatalf("state = %s, next = %v, snapshot = %+v", state, next, e.Snapshot)
	}
}

func TestOnlyASessionThatReallyMovedStopsTheModelCall(t *testing.T) {
	session := func(state State, expires time.Duration) gen.CreationSession {
		return gen.CreationSession{State: string(state), ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(expires), Valid: true}}
	}
	for _, c := range []struct {
		name    string
		current gen.CreationSession
		err     error
		want    bool
	}{
		{"still working", session(StateWorking, time.Hour), nil, false},
		{"a read that failed", gen.CreationSession{}, errors.New("connection reset"), false},
		{"the session is gone", gen.CreationSession{}, pgx.ErrNoRows, true},
		{"cancelled", session(StateCancelled, time.Hour), nil, true},
		{"expired", session(StateWorking, -time.Second), nil, true},
	} {
		if got := sessionMoved(c.current, c.err); got != c.want {
			t.Errorf("%s: moved = %v, want %v", c.name, got, c.want)
		}
	}
}
