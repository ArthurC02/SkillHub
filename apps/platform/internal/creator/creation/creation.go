package creation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"time"
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
	if !s.Limits.Valid() {
		return View{}, ErrUnavailable
	}
	if !id.Valid || !finite(budget) || budget < s.Limits.MaxCallCostUSD || budget > s.Limits.MaxCostUSD || utf8.RuneCountInString(message) > 4000 {
		return View{}, ErrInvalidCommand
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
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	zero := 0.0
	e := envelope{Snapshot: Snapshot{Messages: []llmclient.CreationMessage{}, References: []Reference{}, BudgetUSD: budget, SpentUSD: &zero}, Limits: s.Limits, StartHash: key, Deadline: time.Now().Add(s.Limits.SessionTimeout)}
	state := "waiting_input"
	if strings.TrimSpace(message) != "" {
		e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "user", Content: message})
		state = "queued"
	}
	b, _ := json.Marshal(e)
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
		// Insert the attempt first, then publish its ID in the same initial snapshot.
		_, err = s.enqueue(ctx, tx, row, &e, false)
		if err != nil {
			return View{}, err
		}
		row, err = advance(ctx, tx, row, state, "started", e)
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
	if p.DiagramFingerprint != "" && (!p.DiagramConfirmed || p.DiagramUnderstanding == "") {
		return false
	}
	for _, r := range p.References {
		if !r.Confirmed || !r.Available {
			return false
		}
	}
	return true
}
func invalidate(p *Snapshot) { p.Draft = nil; p.Candidate = nil; p.PendingAction = "" }
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
		return View{}, nil, ErrLimit
	}
	if row.State == "working" || row.State == "queued" {
		if c.Kind != "cancel" {
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
	case "message":
		if strings.TrimSpace(c.Message) == "" || utf8.RuneCountInString(c.Message) > 4000 || len(p.Messages) >= 98 {
			return View{}, nil, ErrInvalidCommand
		}
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "user", Content: c.Message})
		p.BriefConfirmed = false
		invalidate(p)
		queueStep = true
	case "confirm_brief":
		if p.PendingAction != "confirm_brief" || strings.TrimSpace(p.Brief) == "" {
			return View{}, nil, ErrInvalidCommand
		}
		p.BriefConfirmed = true
		p.PendingAction = ""
		queueStep = true
	case "confirm_diagram":
		if p.PendingAction != "confirm_diagram" || p.DiagramUnderstanding == "" {
			return View{}, nil, ErrInvalidCommand
		}
		p.DiagramConfirmed = true
		p.PendingAction = ""
		queueStep = true
	case "select_references":
		if len(c.ReferenceSkillIDs) > 3 || s.ResolveReference == nil {
			return View{}, nil, ErrInvalidCommand
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
		}
		p.PendingAction = ""
		queueStep = true
	case "diagram":
		if c.Diagram == nil {
			return View{}, nil, ErrInvalidCommand
		}
		b, err := base64.StdEncoding.DecodeString(c.Diagram.Data)
		if err != nil || len(b) == 0 || len(b) > 5<<20 {
			return View{}, nil, ErrInvalidCommand
		}
		switch c.Diagram.MediaType {
		case "image/png", "image/jpeg", "image/webp":
		default:
			return View{}, nil, ErrInvalidCommand
		}
		h := sha256.Sum256(b)
		p.DiagramFingerprint = hex.EncodeToString(h[:])
		p.DiagramMediaType = c.Diagram.MediaType
		p.DiagramBytes = len(b)
		p.DiagramUnderstanding = ""
		p.DiagramConfirmed = false
		p.BriefConfirmed = false
		invalidate(p)
		queueStep = true
		transient = true
	case "attach_run":
		if p.Candidate == nil || s.ReadRun == nil || c.RunID == "" {
			return View{}, nil, ErrInvalidCommand
		}
		observation, err := s.ReadRun(ctx, ws, c.RunID, *p.Candidate)
		if err != nil {
			return View{}, nil, ErrNotFound
		}
		p.Candidate.RunID = c.RunID
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: observation})
		state = "candidate_ready"
		queueStep = true
	case "materialize", "finalize":
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
			_ = tx.Rollback(ctx)
			return s.materialize(ctx, ws, row, c, e)
		}
		state = "candidate_ready"
		if c.Kind == "finalize" {
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
	row, err = advance(ctx, tx, row, state, c.Kind, e)
	if err != nil {
		return View{}, nil, err
	}
	v, err := view(row)
	if err != nil {
		return View{}, nil, err
	}
	// A diagram command leaves a fingerprint in its receipt hash, never raw bytes.
	if err = record(ctx, tx, ws.ID, id, c, v); err != nil {
		return View{}, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return View{}, nil, err
	}
	return v, job, nil
}
func (s *Service) materialize(ctx context.Context, ws identity.Workspace, old gen.CreationSession, c Command, e envelope) (View, *JobArgs, error) {
	p := e.Snapshot
	inputs, _ := json.Marshal(map[string]any{"diagram": map[string]any{"sha256": p.DiagramFingerprint, "media_type": p.DiagramMediaType, "bytes": p.DiagramBytes}, "references": p.References})
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
		current.Snapshot.Candidate = &candidate
		current.ExistingSkillID = candidate.SkillID
		state := "candidate_ready"
		if c.Kind == "finalize" {
			state = "saved"
		}
		row, err = advance(ctx, tx, row, state, c.Kind, current)
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
