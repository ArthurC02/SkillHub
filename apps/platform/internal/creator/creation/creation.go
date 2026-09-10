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

	if s.CreditCanStart != nil {
		ok, err := s.CreditCanStart(ctx, ws.ID)
		if err != nil {
			return View{}, err
		}
		if !ok {
			return View{}, ErrCreditThreshold
		}
	}
	if !s.Limits.Valid() {
		return View{}, ErrUnavailable
	}
	if !id.Valid || !finite(budget) || utf8.RuneCountInString(message) > 4000 {
		return View{}, ErrInvalidCommand
	}
	if budget < s.Limits.MaxCallCostUSD || budget > s.Limits.MaxCostUSD {
		return View{}, ErrBudgetOutOfBand
	}
	key := digest(struct {
		Message string
		Budget  float64
	}{message, budget})
	if r, err := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: id, WorkspaceID: ws.ID}); err == nil {
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
	zero := 0.0
	e := envelope{Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, References: []Reference{}, BudgetUSD: budget, SpentUSD: &zero}, Limits: s.Limits, StartHash: key, Deadline: time.Now().Add(s.Limits.SessionTimeout)}
	state := "waiting_input"
	if strings.TrimSpace(message) != "" {
		e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "user", Content: s.masked(message)})
		state = "queued"
	}

	if state == "queued" && s.CatalogCheck != nil {

		refs, cost, err := s.CatalogCheck(ctx, ws, message)
		if err != nil {
			slog.Warn("creation: catalogue check failed, continuing without it", "error", err)
		}
		e.Snapshot.CatalogChecked = err == nil
		if cost > 0 {
			spent := *e.Snapshot.SpentUSD + cost
			e.Snapshot.SpentUSD = &spent
		}
		if len(refs) > 0 {
			if len(refs) > 3 {
				refs = refs[:3]
			}
			for i := range refs {
				refs[i].Confirmed = false
			}
			e.Snapshot.References = refs
			e.Snapshot.PendingAction = "confirm_references"
			state = "waiting_confirmation"
		}
	}
	b, _ := json.Marshal(e)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	row, err := q.CreateCreationSession(ctx, gen.CreateCreationSessionParams{ID: id, WorkspaceID: ws.ID, State: state, Snapshot: b, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.Limits.Retention), Valid: true}})
	if err != nil {

		_ = tx.Rollback(ctx)
		r, getErr := gen.New(s.Pool).GetCreationSession(ctx, gen.GetCreationSessionParams{ID: id, WorkspaceID: ws.ID})
		if getErr != nil {
			return View{}, ErrConflict
		}
		old, _ := decode(r)
		if old.StartHash != key {
			return View{}, ErrReplayMismatch
		}
		return view(r)
	}
	if err = q.AppendCreationEvent(ctx, gen.AppendCreationEventParams{SessionID: id, WorkspaceID: ws.ID, Revision: 1, EventType: "created", Snapshot: b}); err != nil {
		return View{}, err
	}
	if state == "queued" {

		_, err = s.enqueue(ctx, tx, row, &e, false)
		if err != nil {
			return View{}, err
		}
		row, err = s.advance(ctx, tx, row, state, "started", e)
	}
	if err != nil {
		return View{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return View{}, err
	}
	return view(row)
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
	p.PendingAction = ""
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
	if utf8.RuneCountInString(note) > 4000 || len(p.Messages) >= MaxMessages {
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
	if row.Revision != c.ExpectedRevision {
		return View{}, nil, ErrConflict
	}
	if terminal(row.State) {
		return View{}, nil, ErrInvalidCommand
	}
	e, err := decode(row)
	if err != nil {
		return View{}, nil, err
	}
	if c.Kind != "cancel" && !e.Deadline.After(time.Now()) {
		return View{}, nil, ErrDeadline
	}
	if row.State == "working" || row.State == "queued" {

		if c.Kind != "cancel" && c.Kind != "stop_step" {
			return View{}, nil, ErrConflict
		}
	}
	p := &e.Snapshot
	if p.Draft != nil {
		old := *p.Draft
		e.PreviousDraft = &old
	}
	state := "waiting_input"
	queueStep := false
	transient := false
	switch c.Kind {
	case "cancel":
		state = "cancelled"
		p.PendingAction = ""
		if e.ActiveReceipt.Valid {
			q := gen.New(tx)
			a, getErr := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: e.ActiveReceipt, SessionID: id, WorkspaceID: ws.ID})
			if getErr != nil {
				return View{}, nil, getErr
			}
			if a.Status == "queued" {
				if _, err = q.FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ID, SessionID: id, WorkspaceID: ws.ID, Status: "cancelled", Result: []byte("{}"), Usage: []byte("{}")}); err != nil {
					return View{}, nil, err
				}
			}
		}
		e.ActiveReceipt = pgtype.UUID{}
	case "stop_step":

		if !e.ActiveReceipt.Valid || len(p.Messages) >= MaxMessages {
			return View{}, nil, ErrInvalidCommand
		}
		q := gen.New(tx)
		a, getErr := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: e.ActiveReceipt, SessionID: id, WorkspaceID: ws.ID})
		if getErr != nil {
			return View{}, nil, getErr
		}

		said := "你在這一步完成前喊停。模型呼叫已經發出，費用照計；它交回來的內容沒有採用。"
		if a.Status == "queued" {
			if _, err = q.FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ID, SessionID: id, WorkspaceID: ws.ID, Status: "cancelled", Result: []byte("{}"), Usage: []byte("{}")}); err != nil {
				return View{}, nil, err
			}
			said = "你在模型呼叫發出前喊停，這一步沒有花到錢。"
		}
		e.ActiveReceipt = pgtype.UUID{}
		p.PendingAction = ""
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "assistant", Content: said})
		state = "waiting_input"
	case "message":
		if strings.TrimSpace(c.Message) == "" || utf8.RuneCountInString(c.Message) > 4000 || len(p.Messages) >= MaxMessages {
			return View{}, nil, ErrInvalidCommand
		}
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "user", Content: s.masked(c.Message)})

		p.PendingAction = ""
		queueStep = true
	case "confirm_brief":
		if p.PendingAction != "confirm_brief" || strings.TrimSpace(p.Brief) == "" {
			return View{}, nil, ErrInvalidCommand
		}
		p.BriefConfirmed = true
		p.PendingAction = ""
		p.ModelChanged = nil
		queueStep = true
	case "confirm_diagram":
		if p.PendingAction != "confirm_diagram" || !validDiagramInterpretation(p.DiagramUnderstanding) {
			return View{}, nil, ErrInvalidCommand
		}
		p.DiagramConfirmed = true
		p.PendingAction = ""
		queueStep = true
	case "select_references":
		if len(c.ReferenceSkillIDs) > 3 || s.ResolveReference == nil {
			return View{}, nil, ErrInvalidCommand
		}

		if err := s.attachNote(p, c.Message); err != nil {
			return View{}, nil, err
		}
		refs := []Reference{}
		seen := map[string]bool{}
		for _, sid := range c.ReferenceSkillIDs {
			if seen[sid] {
				return View{}, nil, ErrInvalidCommand
			}
			seen[sid] = true
			r, _, err := s.ResolveReference(ctx, ws, sid, "")
			if err != nil {
				return View{}, nil, ErrNotFound
			}
			r.Confirmed = false
			refs = append(refs, r)
		}
		p.References = refs
		invalidate(p)
		p.BriefConfirmed = false
		p.PendingAction = "confirm_references"
		state = "waiting_confirmation"
	case "adopt_reference":

		if s.Adopt == nil || len(c.ReferenceSkillIDs) != 1 || (p.PendingAction != "confirm_references" && p.PendingAction != "confirm_duplicate") || !listedReference(p, c.ReferenceSkillIDs[0]) {
			return View{}, nil, ErrInvalidCommand
		}
		candidate, err := s.Adopt(ctx, ws, c.ReferenceSkillIDs[0])
		if err != nil {
			return View{}, nil, ErrNotFound
		}
		p.Candidate = &candidate
		p.Adopted = true
		p.PendingAction = ""
		p.PendingMaterialize = ""
		e.ExistingSkillID = candidate.SkillID
		state = "saved"
	case "decline_references":
		if p.PendingAction != "confirm_references" || len(p.Messages) >= MaxMessages {
			return View{}, nil, ErrInvalidCommand
		}
		p.References = []Reference{}
		p.PendingAction = ""
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: "使用者不採用目錄裡的 Skill；請依需求撰寫。"})
		queueStep = true
	case "confirm_references":
		if p.PendingAction != "confirm_references" || s.ResolveReference == nil {
			return View{}, nil, ErrInvalidCommand
		}
		for i, r := range p.References {
			_, _, err := s.ResolveReference(ctx, ws, r.SkillID, r.VersionID)
			if err != nil {
				return View{}, nil, ErrNotFound
			}
			p.References[i].Confirmed = true
			p.References[i].Available = true
		}
		p.PendingAction = ""
		queueStep = true
	case "diagram":
		if c.Diagram == nil {
			return View{}, nil, ErrInvalidCommand
		}
		b, err := base64.StdEncoding.DecodeString(c.Diagram.Data)
		if err != nil || len(b) == 0 || len(b) > MaxDiagramBytes {
			return View{}, nil, ErrInvalidCommand
		}
		switch c.Diagram.MediaType {
		case "image/png", "image/jpeg", "image/webp":
		default:
			return View{}, nil, ErrInvalidCommand
		}

		at := len(p.Messages)

		if err := s.attachNote(p, c.Message); err != nil {
			return View{}, nil, err
		}
		h := sha256.Sum256(b)
		p.DiagramFingerprint = hex.EncodeToString(h[:])
		p.DiagramMediaType = c.Diagram.MediaType
		p.DiagramBytes = len(b)

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
		queueStep = true
		transient = true
	case "attach_run":
		if p.Candidate == nil || s.ReadRun == nil || c.RunID == "" || len(p.Messages) >= MaxMessages {
			return View{}, nil, ErrInvalidCommand
		}
		observation, err := s.ReadRun(ctx, ws, c.RunID, *p.Candidate)
		if err != nil {
			return View{}, nil, ErrNotFound
		}
		p.Candidate.RunID = c.RunID
		p.RunUnmet = runUnmet(observation)

		observation = s.masked(observation)
		p.EvaluationText = evaluationFreeText(observation)
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: observation})
		state = "candidate_ready"
		queueStep = true

		if questions := trialQuestions(observation); p.RunUnmet && questions != "" {
			p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "assistant", Content: questions})
			p.PendingAction = ""
			state = "waiting_input"
			queueStep = false
		}
	case "confirm_fetch":
		if p.PendingAction != "confirm_fetch" || p.PendingFetchURL == "" {
			return View{}, nil, ErrInvalidCommand
		}

		p.PendingAction = ""
		queueStep = true
	case "decline_fetch":
		if p.PendingAction != "confirm_fetch" || p.PendingFetchURL == "" {
			return View{}, nil, ErrInvalidCommand
		}
		rec := Fetch{URL: p.PendingFetchURL, Status: "declined"}
		p.Fetches = append(p.Fetches, rec)
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: fetchObservation(rec, "")})
		p.PendingFetchURL = ""
		p.PendingAction = ""
		queueStep = true
	case "raise_budget":
		if !finite(c.BudgetUSD) || c.BudgetUSD <= p.BudgetUSD || c.BudgetUSD > e.Limits.MaxCostUSD {
			return View{}, nil, ErrBudgetOutOfBand
		}
		p.BudgetUSD = c.BudgetUSD
		if row.State == "failed" {
			state = "waiting_input"
		} else {
			state = row.State
		}
	case "materialize", "finalize", "confirm_duplicate":

		kind := c.Kind
		if c.Kind == "confirm_duplicate" {
			if p.PendingAction != "confirm_duplicate" || p.PendingMaterialize == "" {
				return View{}, nil, ErrInvalidCommand
			}
			kind = p.PendingMaterialize
			p.DuplicateAcknowledged = true
			p.PendingAction = ""
			p.PendingMaterialize = ""

			if taken, collides := nameCollides(p.Draft.Skill.Name, p.Duplicates); collides && p.Draft != nil && p.Draft.ContentHash == c.ContentHash && len(p.Messages) < MaxMessages {
				p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: fmt.Sprintf("使用者仍要建立自己的版本，但草稿名稱「%s」與目錄裡那份相同，保存會被拒絕；請只改名稱（描述其差異），其餘內容不變，重新交出草稿。", taken)})
				queueStep = true
				break
			}
		}
		if p.Draft == nil || p.Draft.Blocked || p.Draft.ContentHash == "" || p.Draft.ContentHash != c.ContentHash || !confirmed(*p) {
			return View{}, nil, ErrInvalidCommand
		}
		if s.ResolveReference == nil && len(p.References) > 0 {
			return View{}, nil, ErrUnavailable
		}
		for _, ref := range p.References {
			if _, _, err := s.ResolveReference(ctx, ws, ref.SkillID, ref.VersionID); err != nil {
				return View{}, nil, ErrNotFound
			}
		}
		if p.Candidate == nil {
			if s.Materialize == nil {
				return View{}, nil, ErrUnavailable
			}
			if !p.DuplicateAcknowledged && s.DuplicateCheck != nil {

				dups, cost, err := s.DuplicateCheck(ctx, ws, duplicateQuery(p.Draft.Skill))
				if err != nil {
					slog.Warn("creation: duplicate check failed, materializing without it", "error", err)
				}
				if cost > 0 && p.SpentUSD != nil {
					spent := *p.SpentUSD + cost
					p.SpentUSD = &spent
				}
				if len(dups) > 0 {
					if len(dups) > 3 {
						dups = dups[:3]
					}
					for i := range dups {
						dups[i].Confirmed = false
					}
					p.Duplicates = dups
					p.PendingMaterialize = kind
					p.PendingAction = "confirm_duplicate"
					state = "waiting_confirmation"
					break
				}
				p.DuplicateAcknowledged = err == nil
			}
			_ = tx.Rollback(ctx)
			return s.materialize(ctx, ws, row, c, kind, e)
		}
		state = "candidate_ready"
		if kind == "finalize" {
			state = "saved"
		}
	default:
		return View{}, nil, ErrInvalidCommand
	}
	var job *JobArgs
	if queueStep {
		if !canSpend(*p, e.Limits) {
			return View{}, nil, ErrLimit
		}
		state = "queued"
		a, err := s.enqueue(ctx, tx, row, &e, transient)
		if err != nil {
			return View{}, nil, err
		}
		if transient {
			job = &a
		}
	}
	row, err = s.advance(ctx, tx, row, state, c.Kind, e)
	if err != nil {
		return View{}, nil, err
	}
	v, err := view(row)
	if err != nil {
		return View{}, nil, err
	}

	if err = record(ctx, tx, ws.ID, id, c, v); err != nil {
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
		if row.Revision != c.ExpectedRevision || !live(row) || terminal(row.State) {
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
		current.Snapshot.PendingAction = ""
		current.Snapshot.PendingMaterialize = ""
		current.ExistingSkillID = candidate.SkillID
		state := "candidate_ready"
		if kind == "finalize" {
			state = "saved"
		}
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
			Available bool   `json:"evaluation_available"`
			Status    string `json:"status"`
			Results   []struct {
				Text   string `json:"text"`
				Result string `json:"result"`
				Reason string `json:"reason"`
			} `json:"criterion_results"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil || !o.Evaluation.Available || o.Evaluation.Status != "completed" {
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
			Available bool   `json:"evaluation_available"`
			Status    string `json:"status"`
			Overall   string `json:"overall"`
		} `json:"evaluation"`
	}
	if json.Unmarshal([]byte(observation), &o) != nil || !o.Evaluation.Available {
		return false
	}
	return o.Evaluation.Status == "completed" && o.Evaluation.Overall != "" && o.Evaluation.Overall != "met"
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
