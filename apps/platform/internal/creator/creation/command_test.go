package creation

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
)

const understoodDiagram = `{"nodes":["start"],"conditions":[],"branches":[],"uncertainties":[]}`

func storedSession(t *testing.T, state State, revision int64, e envelope) gen.CreationSession {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return gen.CreationSession{State: string(state), Revision: revision, Snapshot: b}
}

func openSession() envelope {
	return envelope{Limits: testLimits(), Deadline: time.Now().Add(time.Hour), Snapshot: Snapshot{Messages: []Message{}}}
}

func saveableSnapshot() Snapshot {
	return Snapshot{
		Messages:       []Message{},
		Brief:          "b",
		BriefConfirmed: true,
		Draft:          &Draft{ContentHash: "h", Skill: GeneratedSkill{Name: "summary", Description: "d", Body: "body"}},
	}
}

func attachConfirmedDiagram(p *Snapshot) {
	p.DiagramFingerprint = "fp"
	p.DiagramDescription = "起點到終點"
	p.DiagramDescriptionConfirmed = true
	p.DiagramConfirmed = true
	p.DiagramInterpretation = &DiagramInterpretation{
		Nodes:         []string{"起點"},
		Uncertainties: []DiagramUncertainty{{ID: "11111111-1111-4111-8111-111111111111", Question: "誰核准？", Answer: "主管"}},
	}
}

func materializer() func(context.Context, identity.Workspace, GeneratedSkill, Provenance, func(context.Context, pgx.Tx, Candidate) error) error {
	return func(context.Context, identity.Workspace, GeneratedSkill, Provenance, func(context.Context, pgx.Tx, Candidate) error) error {
		return nil
	}
}

func TestACommandForAnotherRevisionConflicts(t *testing.T) {
	row := storedSession(t, StateWaitingInput, 3, openSession())
	for _, expected := range []int64{2, 4} {
		if _, err := admitCommand(row, Command{Kind: "message", ExpectedRevision: expected}); !errors.Is(err, ErrConflict) {
			t.Errorf("expected revision %d against revision 3: err = %v, want ErrConflict", expected, err)
		}
	}
}

func TestAnEndedSessionAcceptsNoCommandNotEvenCancel(t *testing.T) {
	for _, ended := range []State{StateSaved, StateCancelled} {
		row := storedSession(t, ended, 1, openSession())
		if _, err := admitCommand(row, Command{Kind: "cancel", ExpectedRevision: 1}); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%s: err = %v, want ErrInvalidCommand", ended, err)
		}
	}
}

func TestASessionThatCannotBeDecodedIsReportedAsItIs(t *testing.T) {
	row := gen.CreationSession{State: string(StateWaitingInput), Revision: 1, Snapshot: []byte(`{"snapshot": 5}`)}
	_, err := admitCommand(row, Command{Kind: "message", ExpectedRevision: 1})
	if err == nil || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidCommand) || errors.Is(err, ErrDeadline) {
		t.Fatalf("err = %v, want the decoding error itself", err)
	}
}

func TestPastTheDeadlineOnlyCancelIsAccepted(t *testing.T) {
	e := openSession()
	e.Deadline = time.Now().Add(-time.Second)
	row := storedSession(t, StateWaitingInput, 1, e)
	if _, err := admitCommand(row, Command{Kind: "message", ExpectedRevision: 1}); !errors.Is(err, ErrDeadline) {
		t.Fatalf("message after the deadline: err = %v, want ErrDeadline", err)
	}
	if _, err := admitCommand(row, Command{Kind: "cancel", ExpectedRevision: 1}); err != nil {
		t.Fatalf("cancel after the deadline: err = %v, want nil", err)
	}
}

func TestWhileTheModelHoldsTheTurnOnlyCancelAndStopStepAreAccepted(t *testing.T) {
	for _, state := range []State{StateQueued, StateWorking} {
		row := storedSession(t, state, 1, openSession())
		for kind, want := range map[string]error{"message": ErrConflict, "confirm_brief": ErrConflict, "cancel": nil, "stop_step": nil} {
			if _, err := admitCommand(row, Command{Kind: kind, ExpectedRevision: 1}); !errors.Is(err, want) {
				t.Errorf("%s %s: err = %v, want %v", state, kind, err, want)
			}
		}
	}
}

func TestTheDraftIsCopiedAsThePreviousDraftBeforeTheCommandRuns(t *testing.T) {
	e := openSession()
	e.Snapshot.Draft = &Draft{Revision: 4, ContentHash: "h"}
	got, err := admitCommand(storedSession(t, StateDraftReady, 5, e), Command{Kind: "message", ExpectedRevision: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviousDraft == nil || got.PreviousDraft == got.Snapshot.Draft || got.PreviousDraft.ContentHash != "h" || got.PreviousDraft.Revision != 4 {
		t.Fatalf("previous draft = %+v, want a separate copy of the current draft", got.PreviousDraft)
	}
}

func TestWithoutADraftTheStoredPreviousDraftIsKept(t *testing.T) {
	e := openSession()
	e.PreviousDraft = &Draft{ContentHash: "earlier"}
	got, err := admitCommand(storedSession(t, StateWaitingInput, 2, e), Command{Kind: "message", ExpectedRevision: 2})
	if err != nil || got.PreviousDraft == nil || got.PreviousDraft.ContentHash != "earlier" {
		t.Fatalf("previous draft = %+v, err = %v", got.PreviousDraft, err)
	}
}

func TestAMessageIsMaskedAndQueuesAStep(t *testing.T) {
	s := &Service{Mask: func(v string) string { return "masked:" + v }}
	p := &Snapshot{PendingAction: "confirm_brief"}
	got, err := s.acceptMessage(p, "hello")
	if err != nil || !got.queueStep || got.transient {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
	if last := p.Messages[len(p.Messages)-1]; last.Role != "user" || last.Content != "masked:hello" || p.PendingAction != "" {
		t.Fatalf("snapshot = %+v", p)
	}
}

func TestAMessageMustHaveTextFitTheLimitAndLeaveRoom(t *testing.T) {
	for _, c := range []struct {
		name     string
		message  string
		messages int
		ok       bool
	}{
		{"blank", "  ", 0, false},
		{"at the rune limit", strings.Repeat("字", maxPersonMessageRunes), 0, true},
		{"one rune over", strings.Repeat("字", maxPersonMessageRunes+1), 0, false},
		{"at the message ceiling", "hi", MaxMessages, false},
		{"one below the ceiling", "hi", MaxMessages - 1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := &Snapshot{Messages: make([]Message, c.messages)}
			_, err := (&Service{}).acceptMessage(p, c.message)
			if c.ok && err != nil || !c.ok && !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("err = %v, want accepted=%v", err, c.ok)
			}
		})
	}
}

func TestConfirmingTheBriefNeedsTheQuestionAndABrief(t *testing.T) {
	for name, p := range map[string]Snapshot{"nobody asked": {Brief: "b"}, "blank brief": {PendingAction: "confirm_brief", Brief: " "}} {
		if _, err := confirmBrief(&p); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%s: err = %v, want ErrInvalidCommand", name, err)
		}
	}
	p := Snapshot{PendingAction: "confirm_brief", Brief: "b", ModelChanged: &ModelChange{Brief: "a"}}
	got, err := confirmBrief(&p)
	if err != nil || !got.queueStep || !p.BriefConfirmed || p.PendingAction != "" || p.ModelChanged != nil {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
}

func TestDiagramCheckpointsRequireDescriptionAnswersAndFinalConfirmation(t *testing.T) {
	for name, p := range map[string]Snapshot{"nobody asked": {DiagramDescription: "start"}, "already decomposed": {PendingAction: PendingDiagramDescription, DiagramDescription: "start", DiagramInterpretation: &DiagramInterpretation{}}} {
		if _, err := confirmDiagramDescription(&p); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%s: err = %v, want ErrInvalidCommand", name, err)
		}
	}
	p := Snapshot{DiagramFingerprint: "digest", PendingAction: PendingDiagramDescription, DiagramDescription: "開始處理資料"}
	got, err := confirmDiagramDescription(&p)
	if err != nil || !got.queueStep || !p.DiagramDescriptionConfirmed || p.DiagramConfirmed || p.PendingAction != "" {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
	p.DiagramInterpretation = &DiagramInterpretation{Nodes: []string{"開始"}, Uncertainties: []DiagramUncertainty{{ID: "11111111-1111-4111-8111-111111111111", Question: "誰核准？"}}}
	p.PendingAction = PendingDiagramAnswers
	if _, err = answerDiagramUncertainty(&p, "missing", "主管"); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("unknown uncertainty = %v, want ErrInvalidCommand", err)
	}
	if _, err = answerDiagramUncertainty(&p, "11111111-1111-4111-8111-111111111111", " "); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("blank answer = %v, want ErrInvalidCommand", err)
	}
	if got, err = answerDiagramUncertainty(&p, "11111111-1111-4111-8111-111111111111", "主管"); err != nil || got.state != StateWaitingConfirmation || p.PendingAction != PendingDiagramInterpretation {
		t.Fatalf("answer outcome = %+v, err = %v", got, err)
	}
	if got, err = confirmDiagramInterpretation(&p); err != nil || !got.queueStep || !p.DiagramConfirmed || p.PendingAction != "" {
		t.Fatalf("interpretation outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
}

func TestSelectingReferencesResolvesEachAndAsksForConfirmation(t *testing.T) {
	var asked []string
	s := &Service{ResolveReference: func(_ context.Context, _ identity.Workspace, id, version string) (Reference, ReferenceSkill, error) {
		asked = append(asked, id+"@"+version)
		return Reference{SkillID: id, Confirmed: true}, ReferenceSkill{}, nil
	}}
	p := &Snapshot{BriefConfirmed: true, Draft: &Draft{}, PendingAction: "confirm_brief"}
	got, err := s.selectReferences(context.Background(), identity.Workspace{}, p, Command{ReferenceSkillIDs: []string{"a", "b", "c"}, Message: "use these"})
	if err != nil || got.state != StateWaitingConfirmation || got.queueStep {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
	if strings.Join(asked, ",") != "a@,b@,c@" || len(p.References) != 3 || p.References[0].Confirmed || p.References[1].Confirmed || p.References[2].Confirmed {
		t.Fatalf("asked = %v, references = %+v", asked, p.References)
	}
	if p.PendingAction != "confirm_references" || p.BriefConfirmed || p.Draft != nil || p.Messages[len(p.Messages)-1].Content != "use these" {
		t.Fatalf("snapshot = %+v", p)
	}
}

func TestSelectingReferencesRefusesWhatItCannotResolve(t *testing.T) {
	resolve := func(_ context.Context, _ identity.Workspace, id, _ string) (Reference, ReferenceSkill, error) {
		if id == "gone" {
			return Reference{}, ReferenceSkill{}, errors.New("no such skill")
		}
		return Reference{SkillID: id}, ReferenceSkill{}, nil
	}
	for _, c := range []struct {
		name    string
		resolve func(context.Context, identity.Workspace, string, string) (Reference, ReferenceSkill, error)
		ids     []string
		want    error
	}{
		{"more than three", resolve, []string{"a", "b", "c", "d"}, ErrInvalidCommand},
		{"no resolver", nil, []string{"a"}, ErrInvalidCommand},
		{"the same skill twice", resolve, []string{"a", "a"}, ErrInvalidCommand},
		{"a skill that does not resolve", resolve, []string{"a", "gone"}, ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := &Service{ResolveReference: c.resolve}
			if _, err := s.selectReferences(context.Background(), identity.Workspace{}, &Snapshot{}, Command{ReferenceSkillIDs: c.ids}); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestAdoptingAListedSkillSavesTheSessionAsThatSkill(t *testing.T) {
	s := &Service{Adopt: func(_ context.Context, _ identity.Workspace, id string) (Candidate, error) {
		return Candidate{SkillID: id, VersionID: "v1"}, nil
	}}
	e := &envelope{Snapshot: Snapshot{PendingAction: "confirm_duplicate", PendingMaterialize: "finalize", Duplicates: []Reference{{SkillID: "dup"}}}}
	got, err := s.adoptReference(context.Background(), identity.Workspace{}, e, []string{"dup"})
	p := e.Snapshot
	if err != nil || got.state != StateSaved || got.queueStep {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
	if p.Candidate == nil || p.Candidate.SkillID != "dup" || !p.Adopted || p.PendingAction != "" || p.PendingMaterialize != "" || e.ExistingSkillID != "dup" {
		t.Fatalf("envelope = %+v", e)
	}
}

func TestAdoptingNeedsOneListedSkillAndAnAdopter(t *testing.T) {
	adopt := func(_ context.Context, _ identity.Workspace, id string) (Candidate, error) {
		if id == "gone" {
			return Candidate{}, errors.New("no such skill")
		}
		return Candidate{SkillID: id}, nil
	}
	listed := Snapshot{PendingAction: "confirm_references", References: []Reference{{SkillID: "a"}, {SkillID: "gone"}}}
	for _, c := range []struct {
		name    string
		adopt   func(context.Context, identity.Workspace, string) (Candidate, error)
		pending PendingAction
		ids     []string
		want    error
	}{
		{"no adopter", nil, "confirm_references", []string{"a"}, ErrInvalidCommand},
		{"two skills", adopt, "confirm_references", []string{"a", "gone"}, ErrInvalidCommand},
		{"nobody asked", adopt, "", []string{"a"}, ErrInvalidCommand},
		{"not listed", adopt, "confirm_references", []string{"elsewhere"}, ErrInvalidCommand},
		{"adoption fails", adopt, "confirm_references", []string{"gone"}, ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &envelope{Snapshot: listed}
			e.Snapshot.PendingAction = c.pending
			if _, err := (&Service{Adopt: c.adopt}).adoptReference(context.Background(), identity.Workspace{}, e, c.ids); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestDecliningReferencesEmptiesThemAndTellsTheModel(t *testing.T) {
	p := &Snapshot{PendingAction: "confirm_references", References: []Reference{{SkillID: "a"}}}
	got, err := declineReferences(p)
	if err != nil || !got.queueStep || p.References == nil || len(p.References) != 0 || p.PendingAction != "" {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
	if last := p.Messages[len(p.Messages)-1]; last.Role != "tool" || last.Content != "使用者不採用目錄裡的 Skill；請依需求撰寫。" {
		t.Fatalf("last message = %+v", last)
	}
	for name, q := range map[string]Snapshot{"nobody asked": {}, "at the message ceiling": {PendingAction: "confirm_references", Messages: make([]Message, MaxMessages)}} {
		if _, err := declineReferences(&q); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%s: err = %v, want ErrInvalidCommand", name, err)
		}
	}
}

func TestConfirmingReferencesMarksEveryOneThatStillResolves(t *testing.T) {
	var asked []string
	s := &Service{ResolveReference: func(_ context.Context, _ identity.Workspace, id, version string) (Reference, ReferenceSkill, error) {
		asked = append(asked, id+"@"+version)
		return Reference{}, ReferenceSkill{}, nil
	}}
	p := &Snapshot{PendingAction: "confirm_references", References: []Reference{{SkillID: "a", VersionID: "1"}, {SkillID: "b", VersionID: "2"}}}
	got, err := s.confirmReferences(context.Background(), identity.Workspace{}, p)
	if err != nil || !got.queueStep || p.PendingAction != "" || strings.Join(asked, ",") != "a@1,b@2" {
		t.Fatalf("outcome = %+v, asked = %v, err = %v", got, asked, err)
	}
	for _, r := range p.References {
		if !r.Confirmed || !r.Available {
			t.Fatalf("reference = %+v, want confirmed and available", r)
		}
	}
}

func TestConfirmingReferencesRefusesWithoutTheQuestionAResolverOrAResolvableSkill(t *testing.T) {
	failing := func(context.Context, identity.Workspace, string, string) (Reference, ReferenceSkill, error) {
		return Reference{}, ReferenceSkill{}, errors.New("gone")
	}
	refs := []Reference{{SkillID: "a"}}
	for _, c := range []struct {
		name    string
		s       *Service
		pending PendingAction
		want    error
	}{
		{"nobody asked", &Service{ResolveReference: failing}, "", ErrInvalidCommand},
		{"no resolver", &Service{}, "confirm_references", ErrInvalidCommand},
		{"the skill no longer resolves", &Service{ResolveReference: failing}, "confirm_references", ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.s.confirmReferences(context.Background(), identity.Workspace{}, &Snapshot{PendingAction: c.pending, References: refs}); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestADiagramMustBeANonEmptyImageWithinTheSizeLimit(t *testing.T) {
	encoded := func(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }
	for _, c := range []struct {
		name    string
		diagram *Diagram
		ok      bool
	}{
		{"missing", nil, false},
		{"not base64", &Diagram{MediaType: "image/png", Data: "%%"}, false},
		{"empty", &Diagram{MediaType: "image/png", Data: ""}, false},
		{"at the size limit", &Diagram{MediaType: "image/png", Data: encoded(MaxDiagramBytes)}, true},
		{"one byte over", &Diagram{MediaType: "image/png", Data: encoded(MaxDiagramBytes + 1)}, false},
		{"jpeg", &Diagram{MediaType: "image/jpeg", Data: encoded(1)}, true},
		{"webp", &Diagram{MediaType: "image/webp", Data: encoded(1)}, true},
		{"gif", &Diagram{MediaType: "image/gif", Data: encoded(1)}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			image, err := diagramImage(c.diagram)
			if c.ok && (err != nil || len(image) == 0) || !c.ok && !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("image %d bytes, err = %v, want accepted=%v", len(image), err, c.ok)
			}
		})
	}
}

func TestAnAttachedDiagramReplacesTheUnderstandingAndRunsATransientStep(t *testing.T) {
	image := []byte("png-bytes")
	p := &Snapshot{Messages: []Message{{Role: "user", Content: "hi"}}, DiagramUnderstanding: understoodDiagram, DiagramConfirmed: true, BriefConfirmed: true, Draft: &Draft{}}
	got, err := (&Service{}).attachDiagram(p, Command{Message: "see the flow", Diagram: &Diagram{MediaType: "image/png", Data: base64.StdEncoding.EncodeToString(image)}})
	if err != nil || !got.queueStep || !got.transient {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
	sum := sha256.Sum256(image)
	if p.DiagramFingerprint != hex.EncodeToString(sum[:]) || p.DiagramMediaType != "image/png" || p.DiagramBytes != len(image) {
		t.Fatalf("diagram = %q %q %d", p.DiagramFingerprint, p.DiagramMediaType, p.DiagramBytes)
	}
	if p.DiagramUnderstanding != "" || p.DiagramConfirmed || p.BriefConfirmed || p.Draft != nil {
		t.Fatalf("the old understanding, confirmations or draft survived: %+v", p)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].MessageIndex != 1 || p.Attachments[0].SHA256 != p.DiagramFingerprint || p.Messages[1].Content != "see the flow" {
		t.Fatalf("attachments = %+v, messages = %+v", p.Attachments, p.Messages)
	}
}

const unmetRun = `{"evaluation":{"evaluation_available":true,"status":"completed","overall":"not_met","criterion_results":[{"text":"has totals","result":"failed","reason":"no totals"}]}}`

func TestAnUnmetRunWithQuestionsHandsTheTurnToThePerson(t *testing.T) {
	s := &Service{ReadRun: func(context.Context, identity.Workspace, string, Candidate) (string, error) { return unmetRun, nil }}
	p := &Snapshot{Candidate: &Candidate{SkillID: "s"}, PendingAction: "confirm_brief"}
	got, err := s.attachRun(context.Background(), identity.Workspace{}, p, "run-1")
	if err != nil || got.state != StateWaitingInput || got.queueStep {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
	if p.Candidate.RunID != "run-1" || !p.RunUnmet || !strings.Contains(p.EvaluationText, "no totals") || p.PendingAction != "" {
		t.Fatalf("snapshot = %+v", p)
	}
	if len(p.Messages) != 2 || p.Messages[0].Role != "tool" || p.Messages[0].Content != unmetRun || p.Messages[1].Role != "assistant" || !strings.Contains(p.Messages[1].Content, "「has totals」：沒過") {
		t.Fatalf("messages = %+v", p.Messages)
	}
}

func TestAMetRunGoesBackToTheModel(t *testing.T) {
	met := `{"evaluation":{"evaluation_available":true,"status":"completed","overall":"met","criterion_results":[]}}`
	s := &Service{ReadRun: func(context.Context, identity.Workspace, string, Candidate) (string, error) { return met, nil }}
	p := &Snapshot{Candidate: &Candidate{SkillID: "s"}}
	got, err := s.attachRun(context.Background(), identity.Workspace{}, p, "run-1")
	if err != nil || !got.queueStep || p.RunUnmet || len(p.Messages) != 1 {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
}

func TestWhetherARunWasMetIsReadBeforeMaskingAndTheQuestionsAfter(t *testing.T) {
	s := &Service{
		ReadRun: func(context.Context, identity.Workspace, string, Candidate) (string, error) { return unmetRun, nil },
		Mask:    func(string) string { return "[masked]" },
	}
	p := &Snapshot{Candidate: &Candidate{SkillID: "s"}}
	got, err := s.attachRun(context.Background(), identity.Workspace{}, p, "run-1")
	if err != nil || !got.queueStep || !p.RunUnmet || p.Messages[0].Content != "[masked]" || p.EvaluationText != "" {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
}

func TestAttachingARunNeedsACandidateAReaderARunAndRoom(t *testing.T) {
	read := func(_ context.Context, _ identity.Workspace, id string, _ Candidate) (string, error) {
		if id == "gone" {
			return "", errors.New("no such run")
		}
		return unmetRun, nil
	}
	for _, c := range []struct {
		name string
		read func(context.Context, identity.Workspace, string, Candidate) (string, error)
		p    Snapshot
		run  string
		want error
	}{
		{"no candidate", read, Snapshot{}, "run-1", ErrInvalidCommand},
		{"no reader", nil, Snapshot{Candidate: &Candidate{}}, "run-1", ErrInvalidCommand},
		{"no run", read, Snapshot{Candidate: &Candidate{}}, "", ErrInvalidCommand},
		{"at the message ceiling", read, Snapshot{Candidate: &Candidate{}, Messages: make([]Message, MaxMessages)}, "run-1", ErrInvalidCommand},
		{"the run cannot be read", read, Snapshot{Candidate: &Candidate{}}, "gone", ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := (&Service{ReadRun: c.read}).attachRun(context.Background(), identity.Workspace{}, &c.p, c.run); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestAFetchIsConfirmedOrDeclinedOnlyWhileOneIsPending(t *testing.T) {
	for name, p := range map[string]Snapshot{"nobody asked": {PendingFetchURL: "https://example.com"}, "no address": {PendingAction: "confirm_fetch"}} {
		if _, err := confirmFetch(&p); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("confirm, %s: err = %v", name, err)
		}
		if _, err := declineFetch(&p); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("decline, %s: err = %v", name, err)
		}
	}
}

func TestAConfirmedFetchKeepsTheAddressForTheNextStep(t *testing.T) {
	p := &Snapshot{PendingAction: "confirm_fetch", PendingFetchURL: "https://example.com/a"}
	got, err := confirmFetch(p)
	if err != nil || !got.queueStep || p.PendingAction != "" || p.PendingFetchURL != "https://example.com/a" || len(p.Fetches) != 0 {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
}

func TestADeclinedFetchIsRecordedAndTheModelIsTold(t *testing.T) {
	p := &Snapshot{PendingAction: "confirm_fetch", PendingFetchURL: "https://example.com/a"}
	got, err := declineFetch(p)
	rec := Fetch{URL: "https://example.com/a", Status: "declined"}
	if err != nil || !got.queueStep || p.PendingAction != "" || p.PendingFetchURL != "" || len(p.Fetches) != 1 || p.Fetches[0] != rec {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
	if last := p.Messages[len(p.Messages)-1]; last.Role != "tool" || last.Content != fetchObservation(rec, "") {
		t.Fatalf("last message = %+v", last)
	}
}

func TestRaisingTheBudgetMustGoUpAndStayWithinTheLimit(t *testing.T) {
	for name, budget := range map[string]float64{"not a number": math.NaN(), "the same": .5, "lower": .4, "over the limit": 1.01} {
		p := Snapshot{BudgetUSD: .5}
		if _, err := raiseBudget(&p, testLimits(), StateWaitingInput, budget); !errors.Is(err, ErrBudgetOutOfBand) || p.BudgetUSD != .5 {
			t.Errorf("%s: err = %v, budget = %v", name, err, p.BudgetUSD)
		}
	}
	p := Snapshot{BudgetUSD: .5}
	got, err := raiseBudget(&p, testLimits(), StateDraftReady, 1)
	if err != nil || got.state != StateDraftReady || got.queueStep || p.BudgetUSD != 1 {
		t.Fatalf("raising to the limit: outcome = %+v, budget = %v, err = %v", got, p.BudgetUSD, err)
	}
}

func TestRaisingTheBudgetRevivesAFailedSession(t *testing.T) {
	p := Snapshot{BudgetUSD: .5}
	got, err := raiseBudget(&p, testLimits(), StateFailed, .8)
	if err != nil || got.state != StateWaitingInput || got.queueStep {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
}

func TestTheStopStepSentenceSaysWhetherTheCallWentOut(t *testing.T) {
	if got := stopStepNote(true); got != "你在模型呼叫發出前喊停，這一步沒有花到錢。" {
		t.Errorf("before sending: %q", got)
	}
	if got := stopStepNote(false); got != "你在這一步完成前喊停。模型呼叫已經發出，費用照計；它交回來的內容沒有採用。" {
		t.Errorf("after sending: %q", got)
	}
}

func TestSavingNeedsAConfirmedUnblockedDraftWithTheSameHash(t *testing.T) {
	for name, change := range map[string]func(*Snapshot){
		"no draft":          func(p *Snapshot) { p.Draft = nil },
		"blocked":           func(p *Snapshot) { p.Draft.Blocked = true },
		"no hash":           func(p *Snapshot) { p.Draft.ContentHash = "" },
		"another hash":      func(p *Snapshot) { p.Draft.ContentHash = "other" },
		"unconfirmed brief": func(p *Snapshot) { p.BriefConfirmed = false },
		"unanswered diagram uncertainty": func(p *Snapshot) {
			attachConfirmedDiagram(p)
			p.DiagramInterpretation.Uncertainties[0].Answer = ""
		},
		"unconfirmed diagram description": func(p *Snapshot) {
			attachConfirmedDiagram(p)
			p.DiagramDescriptionConfirmed = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := saveableSnapshot()
			change(&p)
			s := &Service{Materialize: materializer()}
			if _, err := s.save(context.Background(), identity.Workspace{}, &p, Command{Kind: "materialize", ContentHash: "h"}); !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("err = %v, want ErrInvalidCommand", err)
			}
		})
	}
}

func TestADiagramSessionWithEveryUncertaintyAnsweredIsSaveable(t *testing.T) {
	p := saveableSnapshot()
	attachConfirmedDiagram(&p)

	got, err := (&Service{Materialize: materializer()}).save(
		context.Background(), identity.Workspace{}, &p, Command{Kind: "materialize", ContentHash: "h"})

	if err != nil || got.materialize != "materialize" {
		t.Fatalf("a diagram session with every uncertainty answered was refused: outcome=%+v err=%v", got, err)
	}
}

func TestSavingNeedsEveryReferenceToStillResolve(t *testing.T) {
	p := saveableSnapshot()
	p.References = []Reference{{SkillID: "a", Confirmed: true, Available: true}}
	if _, err := (&Service{Materialize: materializer()}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: "materialize", ContentHash: "h"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no resolver: err = %v, want ErrUnavailable", err)
	}
	s := &Service{Materialize: materializer(), ResolveReference: func(context.Context, identity.Workspace, string, string) (Reference, ReferenceSkill, error) {
		return Reference{}, ReferenceSkill{}, errors.New("gone")
	}}
	if _, err := s.save(context.Background(), identity.Workspace{}, &p, Command{Kind: "materialize", ContentHash: "h"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a reference that no longer resolves: err = %v, want ErrNotFound", err)
	}
}

func TestAnExistingCandidateIsSavedWithoutMaterializingAgain(t *testing.T) {
	for kind, want := range map[string]State{"materialize": StateCandidateReady, "finalize": StateSaved} {
		p := saveableSnapshot()
		p.Candidate = &Candidate{SkillID: "s"}
		got, err := (&Service{}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: kind, ContentHash: "h"})
		if err != nil || got.state != want || got.materialize != "" || got.queueStep {
			t.Errorf("%s: outcome = %+v, err = %v", kind, got, err)
		}
	}
}

func TestWithoutAMaterializerANewCandidateCannotBeSaved(t *testing.T) {
	p := saveableSnapshot()
	if _, err := (&Service{}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: "materialize", ContentHash: "h"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestDuplicatesHoldTheSaveForThePerson(t *testing.T) {
	var query string
	s := &Service{Materialize: materializer(), DuplicateCheck: func(_ context.Context, _ identity.Workspace, q string) ([]Reference, float64, error) {
		query = q
		return []Reference{{SkillID: "1", Confirmed: true}, {SkillID: "2"}, {SkillID: "3"}, {SkillID: "4"}}, .02, nil
	}}
	p := saveableSnapshot()
	zero := 0.0
	p.SpentUSD = &zero
	got, err := s.save(context.Background(), identity.Workspace{}, &p, Command{Kind: "finalize", ContentHash: "h"})
	if err != nil || got.state != StateWaitingConfirmation || got.materialize != "" {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
	if query != "summary\nd" || len(p.Duplicates) != 3 || p.Duplicates[0].Confirmed || p.PendingMaterialize != "finalize" || p.PendingAction != "confirm_duplicate" || *p.SpentUSD != .02 {
		t.Fatalf("query = %q, snapshot = %+v", query, p)
	}
}

func TestTheDuplicateCheckDecidesWhetherTheSaveCountsAsAcknowledged(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want bool
	}{"nothing found": {nil, true}, "the check failed": {errors.New("search down"), false}} {
		s := &Service{Materialize: materializer(), DuplicateCheck: func(context.Context, identity.Workspace, string) ([]Reference, float64, error) {
			return nil, 0, c.err
		}}
		p := saveableSnapshot()
		got, err := s.save(context.Background(), identity.Workspace{}, &p, Command{Kind: "materialize", ContentHash: "h"})
		if err != nil || got.materialize != "materialize" || p.DuplicateAcknowledged != c.want {
			t.Errorf("%s: outcome = %+v, acknowledged = %v, err = %v", name, got, p.DuplicateAcknowledged, err)
		}
	}
}

func TestAnAcknowledgedDuplicateIsNotCheckedAgain(t *testing.T) {
	s := &Service{Materialize: materializer(), DuplicateCheck: func(context.Context, identity.Workspace, string) ([]Reference, float64, error) {
		t.Fatal("the check ran again")
		return nil, 0, nil
	}}
	p := saveableSnapshot()
	p.DuplicateAcknowledged = true
	if got, err := s.save(context.Background(), identity.Workspace{}, &p, Command{Kind: "materialize", ContentHash: "h"}); err != nil || got.materialize != "materialize" {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
}

func TestConfirmingADuplicateSavesThePendingKind(t *testing.T) {
	p := saveableSnapshot()
	p.PendingAction, p.PendingMaterialize = "confirm_duplicate", "finalize"
	p.Duplicates = []Reference{{Name: "someone else's"}}
	got, err := (&Service{Materialize: materializer()}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: "confirm_duplicate", ContentHash: "h"})
	if err != nil || got.materialize != "finalize" || !p.DuplicateAcknowledged || p.PendingAction != "" || p.PendingMaterialize != "" {
		t.Fatalf("outcome = %+v, snapshot = %+v, err = %v", got, p, err)
	}
}

func TestConfirmingADuplicateWithTheSameNameAsksTheModelToRename(t *testing.T) {
	p := saveableSnapshot()
	p.PendingAction, p.PendingMaterialize = "confirm_duplicate", "materialize"
	p.Duplicates = []Reference{{Name: "Summary"}}
	got, err := (&Service{Materialize: materializer()}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: "confirm_duplicate", ContentHash: "h"})
	if err != nil || !got.queueStep || got.materialize != "" {
		t.Fatalf("outcome = %+v, err = %v", got, err)
	}
	if last := p.Messages[len(p.Messages)-1]; last.Role != "tool" || !strings.Contains(last.Content, "「Summary」") {
		t.Fatalf("last message = %+v", last)
	}
}

func TestConfirmingADuplicateNeedsTheQuestion(t *testing.T) {
	for name, pending := range map[string][2]string{"nobody asked": {"", "materialize"}, "nothing to save": {"confirm_duplicate", ""}} {
		p := saveableSnapshot()
		p.PendingAction, p.PendingMaterialize = PendingAction(pending[0]), pending[1]
		if _, err := (&Service{Materialize: materializer()}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: "confirm_duplicate", ContentHash: "h"}); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestConfirmingADuplicateWithNoDraftIsRefused(t *testing.T) {
	p := Snapshot{PendingAction: "confirm_duplicate", PendingMaterialize: "finalize", Duplicates: []Reference{{SkillID: "dup"}}}
	if _, err := (&Service{Materialize: materializer()}).save(context.Background(), identity.Workspace{}, &p, Command{Kind: "confirm_duplicate", ContentHash: "h"}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("err = %v, want ErrInvalidCommand", err)
	}
}
