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
	ID                pgtype.UUID `json:"command_id"`
	ExpectedRevision  int64       `json:"expected_revision"`
	Kind              string      `json:"kind"`
	Message           string      `json:"message,omitempty"`
	ReferenceSkillIDs []string    `json:"reference_skill_ids,omitempty"`
	ContentHash       string      `json:"content_hash,omitempty"`
	Diagram           *Diagram    `json:"diagram,omitempty"`
	RunID             string      `json:"run_id,omitempty"`

	BudgetUSD float64 `json:"budget_usd,omitempty"`
}

const maxPersonMessageRunes = 4000

func newID() pgtype.UUID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return pgtype.UUID{Bytes: b, Valid: true}
}
func (s *Service) enqueue(ctx context.Context, tx pgx.Tx, row gen.CreationSession, e *envelope, transient bool) (JobArgs, error) {
	if s.Insert == nil && !transient {
		return JobArgs{}, ErrUnavailable
	}
	id := newID()
	a := JobArgs{SessionID: row.ID, WorkspaceID: row.WorkspaceID, Revision: row.Revision, ReceiptID: id}
	_, err := gen.New(tx).InsertCreationReceipt(ctx, gen.InsertCreationReceiptParams{ID: id, SessionID: row.ID, WorkspaceID: row.WorkspaceID, Kind: "attempt", Status: "queued", ExpectedRevision: row.Revision, RequestHash: digest(a), Result: []byte("{}")})
	if err != nil {
		return a, err
	}
	e.ActiveReceipt = id
	if !transient {
		err = s.Insert(ctx, tx, a)
	}
	return a, err
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
func record(ctx context.Context, tx pgx.Tx, ws, id pgtype.UUID, c Command, v View) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = gen.New(tx).InsertCreationReceipt(ctx, gen.InsertCreationReceiptParams{ID: c.ID, SessionID: id, WorkspaceID: ws, Kind: "command", Status: "finished", ExpectedRevision: c.ExpectedRevision, RequestHash: digest(c), Result: b})
	return err
}
func confirmed(p Snapshot) bool {
	if !p.BriefConfirmed || strings.TrimSpace(p.Brief) == "" {
		return false
	}
	if (p.DiagramFingerprint != "" || p.DiagramUnderstanding != "") && (!p.DiagramConfirmed || !validDiagramInterpretation(p.DiagramUnderstanding)) {
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

func (s *Service) Act(ctx context.Context, ws identity.Workspace, id pgtype.UUID, c Command) (View, *JobArgs, error) {
	if !c.ID.Valid || c.ExpectedRevision < 1 {
		return View{}, nil, ErrInvalidCommand
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := gen.New(tx).LockCreationSession(ctx, gen.LockCreationSessionParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !live(row)) {
		return View{}, nil, ErrNotFound
	}
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
	outcome, err := s.apply(ctx, tx, ws, row, c, &e)
	if err != nil {
		return View{}, nil, err
	}
	if outcome.materialize != "" {
		_ = tx.Rollback(ctx)
		return s.materialize(ctx, ws, row, c, outcome.materialize, e)
	}
	return s.commitCommand(ctx, tx, row, c, e, outcome)
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
	if c.Kind != "cancel" && !e.Deadline.After(time.Now()) {
		return envelope{}, ErrDeadline
	}
	if current.AwaitsTheModel() && c.Kind != "cancel" && c.Kind != "stop_step" {
		return envelope{}, ErrConflict
	}
	if e.Snapshot.Draft != nil {
		kept := *e.Snapshot.Draft
		e.PreviousDraft = &kept
	}
	return e, nil
}

func (s *Service) apply(ctx context.Context, tx pgx.Tx, ws identity.Workspace, row gen.CreationSession, c Command, e *envelope) (commandOutcome, error) {
	p := &e.Snapshot
	switch c.Kind {
	case "cancel":
		return cancelSession(ctx, tx, row, e)
	case "stop_step":
		return stopStep(ctx, tx, row, e)
	case "message":
		return s.acceptMessage(p, c.Message)
	case "confirm_brief":
		return confirmBrief(p)
	case "confirm_diagram":
		return confirmDiagram(p)
	case "select_references":
		return s.selectReferences(ctx, ws, p, c)
	case "adopt_reference":
		return s.adoptReference(ctx, ws, e, c.ReferenceSkillIDs)
	case "decline_references":
		return declineReferences(p)
	case "confirm_references":
		return s.confirmReferences(ctx, ws, p)
	case "diagram":
		return s.attachDiagram(p, c)
	case "attach_run":
		return s.attachRun(ctx, ws, p, c.RunID)
	case "confirm_fetch":
		return confirmFetch(p)
	case "decline_fetch":
		return declineFetch(p)
	case "raise_budget":
		return raiseBudget(p, e.Limits, State(row.State), c.BudgetUSD)
	case "materialize", "finalize", "confirm_duplicate":
		return s.save(ctx, ws, p, c)
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
	beforeSending, err := withdrawAttempt(ctx, tx, row, e.ActiveReceipt)
	if err != nil {
		return commandOutcome{}, err
	}
	e.ActiveReceipt = pgtype.UUID{}
	p.PendingAction = NothingPending
	p.appendMessage("assistant", stopStepNote(beforeSending))
	return settledIn(StateWaitingInput), nil
}

func withdrawAttempt(ctx context.Context, tx pgx.Tx, row gen.CreationSession, receipt pgtype.UUID) (bool, error) {
	q := gen.New(tx)
	a, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: receipt, SessionID: row.ID, WorkspaceID: row.WorkspaceID})
	if err != nil {
		return false, err
	}
	if a.Status != "queued" {
		return false, nil
	}
	_, err = q.FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ID, SessionID: row.ID, WorkspaceID: row.WorkspaceID, Status: "cancelled", Result: []byte("{}"), Usage: []byte("{}")})
	return true, err
}

func stopStepNote(beforeSending bool) string {
	if beforeSending {
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

func confirmDiagram(p *Snapshot) (commandOutcome, error) {
	if p.PendingAction != PendingDiagramConfirmation || !validDiagramInterpretation(p.DiagramUnderstanding) {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.DiagramConfirmed = true
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func (s *Service) commitCommand(ctx context.Context, tx pgx.Tx, row gen.CreationSession, c Command, e envelope, outcome commandOutcome) (View, *JobArgs, error) {
	state := outcome.state
	var job *JobArgs
	if outcome.queueStep {
		if !canSpend(e.Snapshot, e.Limits) {
			return View{}, nil, ErrLimit
		}
		state = StateQueued
		a, err := s.enqueue(ctx, tx, row, &e, outcome.transient)
		if err != nil {
			return View{}, nil, err
		}
		if outcome.transient {
			job = &a
		}
	}
	row, err := s.advance(ctx, tx, row, state, c.Kind, e)
	if err != nil {
		return View{}, nil, err
	}
	v, err := view(row)
	if err != nil {
		return View{}, nil, err
	}
	if err = record(ctx, tx, row.WorkspaceID, row.ID, c, v); err != nil {
		return View{}, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return View{}, nil, err
	}
	return v, job, nil
}
