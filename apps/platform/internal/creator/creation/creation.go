package creation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Command struct {
	ID                pgtype.UUID                `json:"command_id"`
	ExpectedRevision  int64                      `json:"expected_revision"`
	Kind              string                     `json:"kind"`
	Message           string                     `json:"message,omitempty"`
	ReferenceSkillIDs []string                   `json:"reference_skill_ids,omitempty"`
	ContentHash       string                     `json:"content_hash,omitempty"`
	Diagram           *llmclient.GenerateDiagram `json:"diagram,omitempty"`
	RunID             string                     `json:"run_id,omitempty"`

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
func (s *Service) Create(ctx context.Context, ws identity.Workspace, id pgtype.UUID, message string, budget float64) (View, error) {
	if err := s.admitStart(ctx, ws, id, message, budget); err != nil {
		return View{}, err
	}
	key := digest(struct {
		Message string
		Budget  float64
	}{message, budget})
	if r, err := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: id, WorkspaceID: ws.ID}); err == nil {
		return resumeStart(r, key)
	}
	e, state := s.openingEnvelope(ctx, ws, message, budget, key)
	return s.insertSession(ctx, ws, id, key, e, state)
}

func (s *Service) admitStart(ctx context.Context, ws identity.Workspace, id pgtype.UUID, message string, budget float64) error {
	if s.CreditCanStart != nil {
		ok, err := s.CreditCanStart(ctx, ws.ID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCreditThreshold
		}
	}
	if !s.Limits.Valid() {
		return ErrUnavailable
	}
	if !id.Valid || !finite(budget) || utf8.RuneCountInString(message) > maxPersonMessageRunes {
		return ErrInvalidCommand
	}
	if budget < s.Limits.MaxCallCostUSD || budget > s.Limits.MaxCostUSD {
		return ErrBudgetOutOfBand
	}
	return nil
}

func resumeStart(r gen.CreationSession, key string) (View, error) {
	e, err := decode(r)
	if err != nil {
		return View{}, err
	}
	if e.StartHash != key {
		return View{}, ErrReplayMismatch
	}
	if !live(r) {
		return View{}, ErrNotFound
	}
	return view(r)
}

func (s *Service) openingEnvelope(ctx context.Context, ws identity.Workspace, message string, budget float64, key string) (envelope, State) {
	zero := 0.0
	e := envelope{Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, References: []Reference{}, BudgetUSD: budget, SpentUSD: &zero}, Limits: s.Limits, StartHash: key, Deadline: time.Now().Add(s.Limits.SessionTimeout)}
	if strings.TrimSpace(message) == "" {
		return e, StateWaitingInput
	}
	e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "user", Content: s.masked(message)})
	if s.CatalogCheck == nil {
		return e, StateQueued
	}
	refs, cost, err := s.CatalogCheck(ctx, ws, message)
	if err != nil {
		slog.Warn("creation: catalogue check failed, continuing without it", "error", err)
	}
	e.Snapshot.CatalogChecked = err == nil
	addSpend(&e.Snapshot, cost)
	if len(refs) == 0 {
		return e, StateQueued
	}
	e.Snapshot.References = shortlist(refs)
	e.Snapshot.PendingAction = PendingReferenceChoice
	return e, StateWaitingConfirmation
}

func (s *Service) insertSession(ctx context.Context, ws identity.Workspace, id pgtype.UUID, key string, e envelope, state State) (View, error) {
	b, _ := json.Marshal(e)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	row, err := q.CreateCreationSession(ctx, gen.CreateCreationSessionParams{ID: id, WorkspaceID: ws.ID, State: string(state), Snapshot: b, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.Limits.Retention), Valid: true}})
	if err != nil {
		_ = tx.Rollback(ctx)
		return s.startedConcurrently(ctx, ws, id, key)
	}
	if err = q.AppendCreationEvent(ctx, gen.AppendCreationEventParams{SessionID: id, WorkspaceID: ws.ID, Revision: 1, EventType: "created", Snapshot: b}); err != nil {
		return View{}, err
	}
	if state == StateQueued {
		if _, err = s.enqueue(ctx, tx, row, &e, false); err != nil {
			return View{}, err
		}
		if row, err = s.advance(ctx, tx, row, state, "started", e); err != nil {
			return View{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return view(row)
}

func (s *Service) startedConcurrently(ctx context.Context, ws identity.Workspace, id pgtype.UUID, key string) (View, error) {
	r, err := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: id, WorkspaceID: ws.ID})
	if err != nil {
		return View{}, ErrConflict
	}
	return resumeStart(r, key)
}

func addSpend(p *Snapshot, cost float64) {
	if cost > 0 && p.SpentUSD != nil {
		spent := *p.SpentUSD + cost
		p.SpentUSD = &spent
	}
}

func shortlist(refs []Reference) []Reference {
	if len(refs) > 3 {
		refs = refs[:3]
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

func listedReference(p *Snapshot, id string) bool {
	for _, r := range p.References {
		if r.SkillID == id {
			return true
		}
	}
	for _, r := range p.Duplicates {
		if r.SkillID == id {
			return true
		}
	}
	return false
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
	p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "user", Content: s.masked(note)})
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

func duplicateQuery(skill llmclient.GeneratedSkill) string {
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
	p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "assistant", Content: stopStepNote(beforeSending)})
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
	p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "user", Content: s.masked(message)})
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

func (s *Service) selectReferences(ctx context.Context, ws identity.Workspace, p *Snapshot, c Command) (commandOutcome, error) {
	if len(c.ReferenceSkillIDs) > 3 || s.ResolveReference == nil {
		return commandOutcome{}, ErrInvalidCommand
	}
	if err := s.attachNote(p, c.Message); err != nil {
		return commandOutcome{}, err
	}
	refs := []Reference{}
	seen := map[string]bool{}
	for _, sid := range c.ReferenceSkillIDs {
		if seen[sid] {
			return commandOutcome{}, ErrInvalidCommand
		}
		seen[sid] = true
		r, _, err := s.ResolveReference(ctx, ws, sid, "")
		if err != nil {
			return commandOutcome{}, ErrNotFound
		}
		r.Confirmed = false
		refs = append(refs, r)
	}
	p.References = refs
	invalidate(p)
	p.BriefConfirmed = false
	p.PendingAction = PendingReferenceChoice
	return settledIn(StateWaitingConfirmation), nil
}

func (s *Service) adoptReference(ctx context.Context, ws identity.Workspace, e *envelope, skillIDs []string) (commandOutcome, error) {
	p := &e.Snapshot
	if s.Adopt == nil || len(skillIDs) != 1 || (p.PendingAction != PendingReferenceChoice && p.PendingAction != PendingDuplicateAcknowledgement) || !listedReference(p, skillIDs[0]) {
		return commandOutcome{}, ErrInvalidCommand
	}
	candidate, err := s.Adopt(ctx, ws, skillIDs[0])
	if err != nil {
		return commandOutcome{}, ErrNotFound
	}
	p.Candidate = &candidate
	p.Adopted = true
	p.PendingAction = NothingPending
	p.PendingMaterialize = ""
	e.ExistingSkillID = candidate.SkillID
	return settledIn(StateSaved), nil
}

func declineReferences(p *Snapshot) (commandOutcome, error) {
	if p.PendingAction != PendingReferenceChoice || !p.hasRoomFor(1) {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.References = []Reference{}
	p.PendingAction = NothingPending
	p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: "使用者不採用目錄裡的 Skill；請依需求撰寫。"})
	return stepQueued(), nil
}

func (s *Service) confirmReferences(ctx context.Context, ws identity.Workspace, p *Snapshot) (commandOutcome, error) {
	if p.PendingAction != PendingReferenceChoice || s.ResolveReference == nil {
		return commandOutcome{}, ErrInvalidCommand
	}
	for i, r := range p.References {
		if _, _, err := s.ResolveReference(ctx, ws, r.SkillID, r.VersionID); err != nil {
			return commandOutcome{}, ErrNotFound
		}
		p.References[i].Confirmed = true
		p.References[i].Available = true
	}
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func (s *Service) attachDiagram(p *Snapshot, c Command) (commandOutcome, error) {
	image, err := diagramImage(c.Diagram)
	if err != nil {
		return commandOutcome{}, err
	}
	at := len(p.Messages)
	if err := s.attachNote(p, c.Message); err != nil {
		return commandOutcome{}, err
	}
	h := sha256.Sum256(image)
	p.DiagramFingerprint = hex.EncodeToString(h[:])
	p.DiagramMediaType = c.Diagram.MediaType
	p.DiagramBytes = len(image)
	p.Attachments = append(p.Attachments, Attachment{
		MessageIndex: at,
		MediaType:    p.DiagramMediaType,
		Bytes:        p.DiagramBytes,
		SHA256:       p.DiagramFingerprint,
	})
	p.DiagramUnderstanding = ""
	p.DiagramConfirmed = false
	p.BriefConfirmed = false
	invalidate(p)
	return commandOutcome{queueStep: true, transient: true}, nil
}

func diagramImage(d *llmclient.GenerateDiagram) ([]byte, error) {
	if d == nil {
		return nil, ErrInvalidCommand
	}
	image, err := base64.StdEncoding.DecodeString(d.Data)
	if err != nil || len(image) == 0 || len(image) > MaxDiagramBytes {
		return nil, ErrInvalidCommand
	}
	switch d.MediaType {
	case "image/png", "image/jpeg", "image/webp":
		return image, nil
	}
	return nil, ErrInvalidCommand
}

func (s *Service) attachRun(ctx context.Context, ws identity.Workspace, p *Snapshot, runID string) (commandOutcome, error) {
	if p.Candidate == nil || s.ReadRun == nil || runID == "" || !p.hasRoomFor(1) {
		return commandOutcome{}, ErrInvalidCommand
	}
	observation, err := s.ReadRun(ctx, ws, runID, *p.Candidate)
	if err != nil {
		return commandOutcome{}, ErrNotFound
	}
	p.Candidate.RunID = runID
	p.RunUnmet = runUnmet(observation)
	observation = s.masked(observation)
	p.EvaluationText = evaluationFreeText(observation)
	p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: observation})
	if questions := trialQuestions(observation); p.RunUnmet && questions != "" {
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "assistant", Content: questions})
		p.PendingAction = NothingPending
		return settledIn(StateWaitingInput), nil
	}
	return stepQueued(), nil
}

func awaitsFetchConfirmation(p *Snapshot) bool {
	return p.PendingAction == PendingFetchPermission && p.PendingFetchURL != ""
}

func confirmFetch(p *Snapshot) (commandOutcome, error) {
	if !awaitsFetchConfirmation(p) {
		return commandOutcome{}, ErrInvalidCommand
	}
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func declineFetch(p *Snapshot) (commandOutcome, error) {
	if !awaitsFetchConfirmation(p) {
		return commandOutcome{}, ErrInvalidCommand
	}
	rec := Fetch{URL: p.PendingFetchURL, Status: "declined"}
	p.Fetches = append(p.Fetches, rec)
	p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: fetchObservation(rec, "")})
	p.PendingFetchURL = ""
	p.PendingAction = NothingPending
	return stepQueued(), nil
}

func raiseBudget(p *Snapshot, l Limits, current State, budget float64) (commandOutcome, error) {
	if !finite(budget) || budget <= p.BudgetUSD || budget > l.MaxCostUSD {
		return commandOutcome{}, ErrBudgetOutOfBand
	}
	p.BudgetUSD = budget
	if current == StateFailed {
		return settledIn(StateWaitingInput), nil
	}
	return settledIn(current), nil
}

func (s *Service) save(ctx context.Context, ws identity.Workspace, p *Snapshot, c Command) (commandOutcome, error) {
	kind := c.Kind
	if c.Kind == "confirm_duplicate" {
		if p.PendingAction != PendingDuplicateAcknowledgement || p.PendingMaterialize == "" {
			return commandOutcome{}, ErrInvalidCommand
		}
		kind = p.PendingMaterialize
		p.DuplicateAcknowledged = true
		p.PendingAction = NothingPending
		p.PendingMaterialize = ""
		if taken, collides := draftNameTaken(*p, c.ContentHash); collides {
			p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: fmt.Sprintf("使用者仍要建立自己的版本，但草稿名稱「%s」與目錄裡那份相同，保存會被拒絕；請只改名稱（描述其差異），其餘內容不變，重新交出草稿。", taken)})
			return stepQueued(), nil
		}
	}
	if !saveable(*p, c.ContentHash) {
		return commandOutcome{}, ErrInvalidCommand
	}
	if err := s.referencesResolve(ctx, ws, p.References); err != nil {
		return commandOutcome{}, err
	}
	if p.Candidate != nil {
		return settledIn(savedState(kind)), nil
	}
	if s.Materialize == nil {
		return commandOutcome{}, ErrUnavailable
	}
	if !p.DuplicateAcknowledged && s.DuplicateCheck != nil && s.holdForDuplicates(ctx, ws, p, kind) {
		return settledIn(StateWaitingConfirmation), nil
	}
	return commandOutcome{materialize: kind}, nil
}

func draftNameTaken(p Snapshot, contentHash string) (string, bool) {
	if p.Draft == nil || p.Draft.ContentHash != contentHash || !p.hasRoomFor(1) {
		return "", false
	}
	return nameCollides(p.Draft.Skill.Name, p.Duplicates)
}

func saveable(p Snapshot, contentHash string) bool {
	return p.Draft != nil && !p.Draft.Blocked && p.Draft.ContentHash != "" && p.Draft.ContentHash == contentHash && confirmed(p)
}

func savedState(kind string) State {
	if kind == "finalize" {
		return StateSaved
	}
	return StateCandidateReady
}

func (s *Service) referencesResolve(ctx context.Context, ws identity.Workspace, refs []Reference) error {
	if s.ResolveReference == nil && len(refs) > 0 {
		return ErrUnavailable
	}
	for _, ref := range refs {
		if _, _, err := s.ResolveReference(ctx, ws, ref.SkillID, ref.VersionID); err != nil {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Service) holdForDuplicates(ctx context.Context, ws identity.Workspace, p *Snapshot, kind string) bool {
	dups, cost, err := s.DuplicateCheck(ctx, ws, duplicateQuery(p.Draft.Skill))
	if err != nil {
		slog.Warn("creation: duplicate check failed, materializing without it", "error", err)
	}
	addSpend(p, cost)
	if len(dups) == 0 {
		p.DuplicateAcknowledged = err == nil
		return false
	}
	p.Duplicates = shortlist(dups)
	p.PendingMaterialize = kind
	p.PendingAction = PendingDuplicateAcknowledgement
	return true
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

type materializedReference struct {
	SkillID   string `json:"skill_id"`
	VersionID string `json:"version_id"`
	Name      string `json:"name"`
}

func (s *Service) materialize(ctx context.Context, ws identity.Workspace, old gen.CreationSession, c Command, kind string, e envelope) (View, *JobArgs, error) {
	p := e.Snapshot
	refs := make([]materializedReference, len(p.References))
	for i, r := range p.References {
		refs[i] = materializedReference{r.SkillID, r.VersionID, r.Name}
	}
	m := map[string]any{"references": refs}
	if p.DiagramFingerprint != "" {
		m["diagram"] = map[string]any{"sha256": p.DiagramFingerprint, "media_type": p.DiagramMediaType, "bytes": p.DiagramBytes}
	}

	m["interactive"] = true
	inputs, _ := json.Marshal(m)
	provenance := Provenance{p.Brief, p.Model, p.PromptVersion, e.ExistingSkillID, inputs}
	var result View
	err := s.Materialize(ctx, ws, p.Draft.Skill, provenance, func(ctx context.Context, tx pgx.Tx, candidate Candidate) error {
		row, err := gen.New(tx).LockCreationSession(ctx, gen.LockCreationSessionParams{ID: old.ID, WorkspaceID: ws.ID})
		if err != nil {
			return err
		}
		if row.Revision != c.ExpectedRevision || !live(row) || State(row.State).HasEnded() {
			return ErrConflict
		}
		current, err := decode(row)
		if err != nil {
			return err
		}
		if current.Snapshot.Draft == nil || current.Snapshot.Draft.ContentHash != c.ContentHash || !confirmed(current.Snapshot) {
			return ErrConflict
		}
		if _, found, err := replay(ctx, tx, ws.ID, row.ID, c); found || err != nil {
			return ErrConflict
		}
		if s.CreateAcceptanceTestCase != nil && len(current.Snapshot.AcceptanceCriteria) > 0 {

			prompt := current.Snapshot.SampleInput
			if strings.TrimSpace(prompt) == "" {
				prompt = current.Snapshot.Brief
			}
			id, err := s.CreateAcceptanceTestCase(ctx, tx, ws, candidate.SkillID, "創作驗收條件", prompt, current.Snapshot.AcceptanceCriteria)
			if err != nil {
				return err
			}
			candidate.TestCaseID = id
		}
		current.Snapshot.Candidate = &candidate

		current.Snapshot.SpentUSD = p.SpentUSD
		current.Snapshot.Duplicates = p.Duplicates
		current.Snapshot.DuplicateAcknowledged = p.DuplicateAcknowledged
		current.Snapshot.PendingAction = NothingPending
		current.Snapshot.PendingMaterialize = ""
		current.ExistingSkillID = candidate.SkillID
		state := savedState(kind)
		row, err = s.advance(ctx, tx, row, state, c.Kind, current)
		if err != nil {
			return err
		}
		result, err = view(row)
		if err != nil {
			return err
		}
		return record(ctx, tx, ws.ID, row.ID, c, result)
	})
	return result, nil, err
}

func trialQuestions(observation string) string {
	var o struct {
		Evaluation struct {
			Available bool             `json:"evaluation_available"`
			Status    evaluationStatus `json:"status"`
			Results   []struct {
				Text   string `json:"text"`
				Result string `json:"result"`
				Reason string `json:"reason"`
			} `json:"criterion_results"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil || !o.Evaluation.Available || o.Evaluation.Status != evaluationCompleted {
		return ""
	}
	var lines []string
	for _, r := range o.Evaluation.Results {
		if r.Result != "failed" && r.Result != "undetermined" {
			continue
		}
		label := "沒過"
		if r.Result == "undetermined" {
			label = "這份樣本驗不到"
		}
		line := fmt.Sprintf("- 「%s」：%s", truncateRunes(r.Text, 200), label)
		if r.Reason != "" {
			line += "——" + truncateRunes(r.Reason, 300)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return "這次試跑有條件沒過：\n" + strings.Join(lines, "\n") + "\n要照這些條件改草稿、還是改條件或範例輸入？也可以直接說你要它改哪裡。"
}

func evaluationFreeText(observation string) string {
	var o struct {
		Evaluation struct {
			Summary string `json:"summary"`
			Results []struct {
				Reason string `json:"reason"`
			} `json:"criterion_results"`
			Findings []struct {
				Message string `json:"message"`
			} `json:"deterministic_findings"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil {
		return ""
	}
	parts := []string{o.Evaluation.Summary}
	for _, r := range o.Evaluation.Results {
		parts = append(parts, r.Reason)
	}
	for _, f := range o.Evaluation.Findings {
		parts = append(parts, f.Message)
	}
	return strings.Join(parts, "\n")
}

func copiedFromEvaluation(evaluationText, draftText string, theirs ...string) []string {
	if strings.TrimSpace(evaluationText) == "" {
		return nil
	}
	fromJudge := markerSegments(evaluationText)
	if len(fromJudge) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, t := range theirs {
		for _, seg := range alphanumericSegments(t) {
			known[seg] = true
		}
	}
	var copied []string
	seen := map[string]bool{}
	for token, segments := range markerTokens(draftText) {
		for _, seg := range segments {
			if fromJudge[seg] && !known[seg] && !seen[token] {
				seen[token] = true
				copied = append(copied, token)
			}
		}
	}

	sort.Strings(copied)
	return copied
}

func toolsNotRequested(prevTools, curTools string, theirs ...string) []string {
	added := addedToolTokens(prevTools, curTools)
	if len(added) == 0 {
		return nil
	}
	var out []string
	for _, tool := range added {
		if !asked(toolBaseName(tool), theirs) {
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	return out
}

var negations = []string{"不要", "不用", "不需要", "不能", "別用", "別", "禁止", "勿", "無需", "沒有要",
	"don't", "do not", "dont", "no ", "not ", "never", "without", "avoid", "except"}

const negationWindow = 16

func asked(tool string, theirs []string) bool {
	needle := strings.ToLower(strings.TrimSpace(tool))
	if needle == "" {
		return false
	}
	for _, t := range theirs {
		hay := strings.ToLower(t)
		for at := 0; ; {
			i := strings.Index(hay[at:], needle)
			if i < 0 {
				break
			}
			i += at
			if !negated(hay, i) {
				return true
			}
			at = i + len(needle)
		}
	}
	return false
}

func negated(hay string, i int) bool {
	start := i
	for n := 0; start > 0 && n < negationWindow; n++ {
		_, size := utf8.DecodeLastRuneInString(hay[:start])
		start -= size
	}
	before := hay[start:i]
	for _, n := range negations {
		if strings.Contains(before, n) {
			return true
		}
	}
	return false
}

func toolsNamedIn(evaluationText string, tools []string) []string {
	if strings.TrimSpace(evaluationText) == "" {
		return nil
	}
	hay := strings.ToLower(evaluationText)
	var named []string
	for _, tool := range tools {
		if base := strings.ToLower(toolBaseName(tool)); base != "" && strings.Contains(hay, base) {
			named = append(named, tool)
		}
	}
	return named
}

func addedToolTokens(prev, cur string) []string {
	old := map[string]bool{}
	for _, t := range toolFields(prev) {
		old[strings.ToLower(t)] = true
	}
	var added []string
	seen := map[string]bool{}
	for _, t := range toolFields(cur) {
		key := strings.ToLower(t)
		if !old[key] && !seen[key] {
			seen[key] = true
			added = append(added, t)
		}
	}
	return added
}

func toolFields(s string) []string {
	return strings.Fields(strings.ReplaceAll(s, ",", " "))
}

func toolBaseName(tool string) string {
	if i := strings.IndexByte(tool, '('); i >= 0 {
		return tool[:i]
	}
	return tool
}

func markerTokens(s string) map[string][]string {
	out := map[string][]string{}
	for _, token := range tokens(s) {
		if segs := markerLike(token); len(segs) > 0 {
			out[token] = segs
		}
	}
	return out
}

func markerSegments(s string) map[string]bool {
	out := map[string]bool{}
	for _, segs := range markerTokens(s) {
		for _, seg := range segs {
			out[seg] = true
		}
	}
	return out
}

func alphanumericSegments(s string) []string {
	var out []string
	for _, token := range tokens(s) {
		for _, seg := range strings.FieldsFunc(token, isSeparator) {
			if len([]rune(seg)) >= 2 {
				out = append(out, seg)
			}
		}
	}
	return out
}

func isSeparator(r rune) bool { return r == '-' || r == '_' }

func tokens(s string) []string {
	var out []string
	for _, field := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !isSeparator(r)
	}) {
		if field = strings.Trim(field, "-_"); field != "" {
			out = append(out, field)
		}
	}
	return out
}

// markerLike flags a token's ASCII segments that mix letters and digits: a
// hyphen/underscore-joined segment counts at length 4+, a bare segment only
// at length 8+ with at least two of each character class.
func markerLike(token string) []string {
	segments := strings.FieldsFunc(token, isSeparator)
	compound := len(segments) > 1
	var found []string
	for _, seg := range segments {
		letters, digits := 0, 0
		ascii := true
		for _, r := range seg {
			switch {
			case r >= '0' && r <= '9':
				digits++
			case r >= 'a' && r <= 'z':
				letters++
			default:

				ascii = false
			}
		}
		if !ascii || letters < 1 || digits < 1 || len(seg) < 4 {
			continue
		}
		if compound || (letters >= 2 && digits >= 2 && len(seg) >= 8) {
			found = append(found, seg)
		}
	}
	return found
}

func draftText(skill llmclient.GeneratedSkill) string {
	parts := []string{skill.Name, skill.Description, skill.Compatibility, skill.AllowedTools, skill.Body}
	for _, f := range skill.Files {
		parts = append(parts, f.Path, f.Content)
	}
	return strings.Join(parts, "\n")
}

func previousDraftText(d *Draft) string {
	if d == nil {
		return ""
	}
	return draftText(d.Skill)
}

func personText(messages []llmclient.CreationMessage) string {
	var b strings.Builder
	for _, m := range messages {
		if m.Role == "user" {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func runUnmet(observation string) bool {
	var o struct {
		Evaluation struct {
			Available bool              `json:"evaluation_available"`
			Status    evaluationStatus  `json:"status"`
			Overall   evaluationOverall `json:"overall"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil || !o.Evaluation.Available {
		return false
	}
	return o.Evaluation.Status == evaluationCompleted && o.Evaluation.Overall != "" && o.Evaluation.Overall != overallMet
}

func missingDiagramNodes(understanding, body string) []string {
	var sections map[string][]string
	if json.Unmarshal([]byte(understanding), &sections) != nil {
		return nil
	}
	haystack := foldForMatch(body)
	var missing []string
	for _, node := range sections["nodes"] {
		if needle := foldForMatch(node); needle != "" && !strings.Contains(haystack, needle) {
			missing = append(missing, node)
		}
	}
	return missing
}

func foldForMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
