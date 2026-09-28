package creation

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"time"
	"unicode/utf8"
)

type Command struct {
	ID                   pgtype.UUID `json:"command_id"`
	ExpectedRevision     int64       `json:"expected_revision"`
	Kind                 string      `json:"kind"`
	Message              string      `json:"message,omitempty"`
	ReferenceSkillIDs    []string    `json:"reference_skill_ids,omitempty"`
	ContentHash          string      `json:"content_hash,omitempty"`
	Diagram              *Diagram    `json:"diagram,omitempty"`
	DiagramUncertaintyID string      `json:"diagram_uncertainty_id,omitempty"`
	DiagramAnswer        string      `json:"diagram_answer,omitempty"`
	RunID                string      `json:"run_id,omitempty"`

	BudgetUSD float64 `json:"budget_usd,omitempty"`
}

const (
	commandCancel                       = "cancel"
	commandStopStep                     = "stop_step"
	commandMessage                      = "message"
	commandConfirmBrief                 = "confirm_brief"
	commandConfirmDiagram               = "confirm_diagram"
	commandAnswerDiagramUncertainty     = "answer_diagram_uncertainty"
	commandConfirmDiagramInterpretation = "confirm_diagram_interpretation"
	commandSelectReferences             = "select_references"
	commandAdoptReference               = "adopt_reference"
	commandDeclineReferences            = "decline_references"
	commandConfirmReferences            = "confirm_references"
	commandDiagram                      = "diagram"
	commandAttachRun                    = "attach_run"
	commandConfirmFetch                 = "confirm_fetch"
	commandDeclineFetch                 = "decline_fetch"
	commandRaiseBudget                  = "raise_budget"
	commandMaterialize                  = "materialize"
	commandFinalize                     = "finalize"
	commandConfirmDuplicate             = "confirm_duplicate"
)

const (
	receiptQueued    = "queued"
	receiptRunning   = "running"
	receiptUnknown   = "unknown"
	receiptFinished  = "finished"
	receiptFailed    = "failed"
	receiptCancelled = "cancelled"
)

const maxPersonMessageRunes = 4000

const (
	uuidVersionMask    = 0x0f
	uuidVersion4       = 0x40
	uuidVariantMask    = 0x3f
	uuidVariantRFC4122 = 0x80
)

func newID() pgtype.UUID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & uuidVersionMask) | uuidVersion4
	b[8] = (b[8] & uuidVariantMask) | uuidVariantRFC4122
	return pgtype.UUID{Bytes: b, Valid: true}
}
func (s *Service) enqueue(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e *envelope) error {
	if s.Insert == nil {
		return ErrUnavailable
	}
	a, err := openAttemptReceipt(ctx, tx, row, e)
	if err != nil {
		return err
	}
	return s.Insert(ctx, tx, a)
}

func openAttemptReceipt(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e *envelope) (JobArgs, error) {
	id := newID()
	a := JobArgs{SessionID: row.ID, WorkspaceID: row.WorkspaceID, Revision: row.Revision, ReceiptID: id}
	_, err := gen.New(tx).InsertCreationReceipt(ctx, gen.InsertCreationReceiptParams{ID: id, SessionID: row.ID, WorkspaceID: row.WorkspaceID, Kind: "attempt", Status: receiptQueued, ExpectedRevision: row.Revision, RequestHash: digest(a), Result: []byte("{}")})
	if err != nil {
		return a, err
	}
	e.ActiveReceipt = id
	return a, nil
}

func (s *Service) queueStep(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e *envelope, outcome commandOutcome) (*JobArgs, error) {
	if !outcome.transient {
		return nil, s.enqueue(ctx, tx, row, e)
	}
	a, err := openAttemptReceipt(ctx, tx, row, e)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
func addSpend(p *Snapshot, cost float64) {
	if cost > 0 && p.SpentUSD != nil {
		spent := *p.SpentUSD + cost
		p.SpentUSD = &spent
	}
}

func shortlist(refs []Reference) []Reference {
	if len(refs) > MaxReferences {
		refs = refs[:MaxReferences]
	}
	for i := range refs {
		refs[i].Confirmed = false
	}
	return refs
}
func replay(ctx context.Context, tx pgx.Tx, ws, id pgtype.UUID, c Command) (View, bool, error) {
	r, err := gen.New(tx).GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: c.ID, SessionID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, false, nil
	}
	if err != nil {
		return View{}, false, err
	}
	if r.RequestHash != digest(c) {
		return View{}, true, ErrReplayMismatch
	}
	var v View
	err = json.Unmarshal(r.Result, &v)
	return v, true, err
}
func record(ctx context.Context, tx pgx.Tx, session gen.CreationSession, c Command, v View) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = gen.New(tx).InsertCreationReceipt(ctx, gen.InsertCreationReceiptParams{ID: c.ID, SessionID: session.ID, WorkspaceID: session.WorkspaceID, Kind: "command", Status: receiptFinished, ExpectedRevision: c.ExpectedRevision, RequestHash: digest(c), Result: b})
	return err
}
func confirmed(p Snapshot) bool {
	if !p.BriefConfirmed || strings.TrimSpace(p.Brief) == "" {
		return false
	}
	if p.DiagramFingerprint != "" &&
		(!p.DiagramDescriptionConfirmed || !p.DiagramConfirmed || !validDiagramInterpretation(p.DiagramInterpretation) || !allDiagramUncertaintiesAnswered(p.DiagramInterpretation)) {
		return false
	}
	for _, r := range p.References {
		if !r.Confirmed || !r.Available {
			return false
		}
	}
	return true
}
func invalidate(p *Snapshot) {
	p.Draft = nil
	p.Candidate = nil
	p.PendingAction = NothingPending
	clearDuplicateCheck(p)
}

func clearDuplicateCheck(p *Snapshot) {
	p.Duplicates = nil
	p.PendingMaterialize = ""
	p.DuplicateAcknowledged = false
}

func (s *Service) masked(text string) string {
	if s.Mask == nil {
		return text
	}
	return s.Mask(text)
}

func (s *Service) attachNote(p *Snapshot, note string) error {
	if strings.TrimSpace(note) == "" {
		return nil
	}
	if utf8.RuneCountInString(note) > maxPersonMessageRunes || !p.hasRoomFor(1) {
		return ErrInvalidCommand
	}
	p.appendMessage("user", s.masked(note))
	return nil
}

func nameCollides(name string, dups []Reference) (string, bool) {
	for _, d := range dups {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(d.Name)) {
			return d.Name, true
		}
	}
	return "", false
}

func duplicateQuery(skill GeneratedSkill) string {
	return strings.TrimSpace(skill.Name + "\n" + skill.Description)
}

type commandOutcome struct {
	state       State
	queueStep   bool
	transient   bool
	materialize string
}

func settledIn(state State) commandOutcome { return commandOutcome{state: state} }

func stepQueued() commandOutcome { return commandOutcome{queueStep: true} }

type admittedCommand struct {
	command  Command
	envelope envelope
}

func (s *Service) Act(ctx context.Context, ws identity.Workspace, id pgtype.UUID, c Command) (View, *JobArgs, error) {
	if !c.ID.Valid || c.ExpectedRevision < 1 {
		return View{}, nil, ErrInvalidCommand
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := lockLiveSession(ctx, tx, ws, id)
	if err != nil {
		return View{}, nil, err
	}
	if v, found, err := replay(ctx, tx, ws.ID, id, c); found || err != nil {
		return v, nil, err
	}
	e, err := admitCommand(row, c)
	if err != nil {
		return View{}, nil, err
	}
	admitted := admittedCommand{command: c, envelope: e}
	if savesTheDraft(c.Kind) {
		if err := tx.Rollback(ctx); err != nil {
			return View{}, nil, err
		}
		return s.saveCommand(ctx, ws, row, admitted)
	}
	if readsOutsideTheSessionLock(c.Kind) {
		if err := tx.Rollback(ctx); err != nil {
			return View{}, nil, err
		}
		return s.readCommand(ctx, ws, row, admitted)
	}
	outcome, err := s.apply(ctx, tx, ws, row, &admitted)
	if err != nil {
		return View{}, nil, err
	}
	if outcome.materialize != "" {
		_ = tx.Rollback(ctx)
		return s.materialize(ctx, ws, row, admitted, outcome.materialize)
	}
	return s.commitCommand(ctx, tx, row, admitted, outcome)
}

func lockLiveSession(ctx context.Context, tx pgx.Tx, ws identity.Workspace, id pgtype.UUID) (gen.CreationSession, error) {
	row, err := gen.New(tx).LockCreationSession(ctx, gen.LockCreationSessionParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !live(row)) {
		return row, ErrNotFound
	}
	return row, err
}

func savesTheDraft(kind string) bool {
	return kind == commandMaterialize || kind == commandFinalize || kind == commandConfirmDuplicate
}

func readsOutsideTheSessionLock(kind string) bool {
	return kind == commandSelectReferences || kind == commandConfirmReferences || kind == commandAttachRun
}

func admitCommand(row gen.CreationSession, c Command) (envelope, error) {
	if row.Revision != c.ExpectedRevision {
		return envelope{}, ErrConflict
	}
	current := State(row.State)
	if current.HasEnded() {
		return envelope{}, ErrInvalidCommand
	}
	e, err := decode(row)
	if err != nil {
		return envelope{}, err
	}
	if c.Kind != commandCancel && !e.Deadline.After(time.Now()) {
		return envelope{}, ErrDeadline
	}
	if current.AwaitsTheModel() && c.Kind != commandCancel && c.Kind != commandStopStep {
		return envelope{}, ErrConflict
	}
	if e.Snapshot.Draft != nil {
		kept := *e.Snapshot.Draft
		e.PreviousDraft = &kept
	}
	return e, nil
}

func (s *Service) apply(ctx context.Context, tx pgx.Tx, ws identity.Workspace, row gen.CreationSession, admitted *admittedCommand) (commandOutcome, error) {
	c, e := admitted.command, &admitted.envelope
	p := &e.Snapshot
	switch c.Kind {
	case commandCancel:
		return cancelSession(ctx, tx, row, e)
	case commandStopStep:
		return stopStep(ctx, tx, row, e)
	case commandMessage:
		return s.acceptMessage(p, c.Message)
	case commandConfirmBrief:
		return confirmBrief(p)
	case commandDiagram, commandConfirmDiagram, commandAnswerDiagramUncertainty, commandConfirmDiagramInterpretation:
		return s.applyDiagramCommand(p, c)
	case commandSelectReferences:
		return s.selectReferences(ctx, ws, p, c)
	case commandAdoptReference:
		return s.adoptReference(ctx, ws, e, c.ReferenceSkillIDs)
	case commandDeclineReferences:
		return declineReferences(p)
	case commandConfirmReferences:
		return s.confirmReferences(ctx, ws, p)
	case commandAttachRun:
		return s.attachRun(ctx, ws, p, c.RunID)
	case commandConfirmFetch:
		return confirmFetch(p)
	case commandDeclineFetch:
		return declineFetch(p)
	case commandRaiseBudget:
		return raiseBudget(p, e.Limits, State(row.State), c.BudgetUSD)
	case commandMaterialize, commandFinalize, commandConfirmDuplicate:
		return s.save(ctx, ws, p, c)
	}
	return commandOutcome{}, ErrInvalidCommand
}

func (s *Service) applyDiagramCommand(p *Snapshot, c Command) (commandOutcome, error) {
	switch c.Kind {
	case commandDiagram:
		return s.attachDiagram(p, c)
	case commandConfirmDiagram:
		return confirmDiagramDescription(p)
	case commandAnswerDiagramUncertainty:
		return answerDiagramUncertainty(p, c.DiagramUncertaintyID, c.DiagramAnswer)
	case commandConfirmDiagramInterpretation:
		return confirmDiagramInterpretation(p)
	}
	return commandOutcome{}, ErrInvalidCommand
}

func cancelSession(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e *envelope) (commandOutcome, error) {
	e.Snapshot.PendingAction = NothingPending
	if e.ActiveReceipt.Valid {
		if _, err := withdrawAttempt(ctx, tx, row, e.ActiveReceipt); err != nil {
			return commandOutcome{}, err
		}
	}
	e.ActiveReceipt = pgtype.UUID{}
	return settledIn(StateCancelled), nil
}

func stopStep(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e *envelope) (commandOutcome, error) {
	p := &e.Snapshot
	if !e.ActiveReceipt.Valid || !p.hasRoomFor(1) {
		return commandOutcome{}, ErrInvalidCommand
	}
	withdrawn, err := withdrawAttempt(ctx, tx, row, e.ActiveReceipt)
	if err != nil {
		return commandOutcome{}, err
	}
	e.ActiveReceipt = pgtype.UUID{}
	p.PendingAction = NothingPending
	p.appendMessage("assistant", withdrawn.stopStepNote())
	return settledIn(StateWaitingInput), nil
}

type attemptWithdrawal int

const (
	withdrawnAfterSending attemptWithdrawal = iota
	withdrawnBeforeSending
)

func withdrawAttempt(ctx context.Context, tx pgx.Tx, row gen.CreationSession, receipt pgtype.UUID) (attemptWithdrawal, error) {
	q := gen.New(tx)
	a, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: receipt, SessionID: row.ID, WorkspaceID: row.WorkspaceID})
	if err != nil {
		return withdrawnAfterSending, err
	}
	if a.Status != receiptQueued {
		return withdrawnAfterSending, nil
	}
	_, err = q.FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ID, SessionID: row.ID, WorkspaceID: row.WorkspaceID, Status: receiptCancelled, Result: []byte("{}"), Usage: []byte("{}")})
	return withdrawnBeforeSending, err
}

func (w attemptWithdrawal) stopStepNote() string {
	if w == withdrawnBeforeSending {
		return "你在模型呼叫發出前喊停，這一步沒有花到錢。"
	}
	return "你在這一步完成前喊停。模型呼叫已經發出，費用照計；它交回來的內容沒有採用。"
}

func (s *Service) acceptMessage(p *Snapshot, message string) (commandOutcome, error) {
	if strings.TrimSpace(message) == "" || utf8.RuneCountInString(message) > maxPersonMessageRunes || !p.hasRoomFor(1) {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.appendMessage("user", s.masked(message))
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func confirmBrief(p *Snapshot) (commandOutcome, error) {
	if p.PendingAction != PendingBriefConfirmation || strings.TrimSpace(p.Brief) == "" {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.BriefConfirmed = true
	p.PendingAction = NothingPending
	p.ModelChanged = nil
	return stepQueued(), nil
}

func confirmDiagramDescription(p *Snapshot) (commandOutcome, error) {
	if p.DiagramFingerprint == "" || p.PendingAction != PendingDiagramDescription || !validDiagramDescription(p.DiagramDescription) || p.DiagramInterpretation != nil {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.DiagramDescriptionConfirmed = true
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func answerDiagramUncertainty(p *Snapshot, id, answer string) (commandOutcome, error) {
	if p.DiagramFingerprint == "" || p.PendingAction != PendingDiagramAnswers || !validDiagramAnswer(answer) || !validDiagramInterpretation(p.DiagramInterpretation) {
		return commandOutcome{}, ErrInvalidCommand
	}
	for i := range p.DiagramInterpretation.Uncertainties {
		if p.DiagramInterpretation.Uncertainties[i].ID == id {
			p.DiagramInterpretation.Uncertainties[i].Answer = answer
			if allDiagramUncertaintiesAnswered(p.DiagramInterpretation) {
				p.PendingAction = PendingDiagramInterpretation
			}
			return settledIn(StateWaitingConfirmation), nil
		}
	}
	return commandOutcome{}, ErrInvalidCommand
}

func confirmDiagramInterpretation(p *Snapshot) (commandOutcome, error) {
	if p.DiagramFingerprint == "" || p.PendingAction != PendingDiagramInterpretation || !p.DiagramDescriptionConfirmed || !allDiagramUncertaintiesAnswered(p.DiagramInterpretation) {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.DiagramConfirmed = true
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func (s *Service) commitCommand(ctx context.Context, tx pgx.Tx, row gen.CreationSession, admitted admittedCommand, outcome commandOutcome) (View, *JobArgs, error) {
	c, e := admitted.command, admitted.envelope
	state := outcome.state
	var job *JobArgs
	if outcome.queueStep {
		if !canSpend(e.Snapshot, e.Limits) {
			return View{}, nil, ErrLimit
		}
		state = StateQueued
		queued, err := s.queueStep(ctx, tx, row, &e, outcome)
		if err != nil {
			return View{}, nil, err
		}
		job = queued
	}
	row, err := s.advance(ctx, tx, row, transition{state, c.Kind}, e)
	if err != nil {
		return View{}, nil, err
	}
	v, err := view(row)
	if err != nil {
		return View{}, nil, err
	}
	if err = record(ctx, tx, row, c, v); err != nil {
		return View{}, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return View{}, nil, err
	}
	return v, job, nil
}
