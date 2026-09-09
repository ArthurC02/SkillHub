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
	// BudgetUSD is the new ceiling for kind "raise_budget" (05 R-46 (raise)).
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
	// ADR-068 gate ①, checked before anything else: a session below the
	// started threshold must not touch the database at all.
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
	// ── 2026-09-09：這一段必須在 Begin 之前，而那不是排版偏好 ──────────────────
	//
	// 它原本在交易裡，而 `CatalogCheck` 自己要用連線池：`CreationKnowledgeIDs` 是
	// 一次 pgvector 檢索，`ResolveReference` 每命中一筆再讀一次版本。**交易握著一條
	// 連線，裡面的查詢再去要第二條**——連線數大的部署看不出來，而淨測試模式
	// （ADR-060 決策 6）把 `pgxpool` 釘在 `MaxConns=1`，於是它是一個必然的自我死鎖：
	// POST /creation-sessions 永遠不回應，River 的選舉與取件同時餓死在同一條連線上，
	// 只有在瀏覽器放棄、request context 被取消時才鬆開。2026-09-09 實測復現。
	//
	// 這裡沒有任何東西需要那個交易：整段只讀公開目錄、只寫記憶體裡的 `e`。
	//
	// 冪等沒有變壞：上面那次 `GetCreationSession` 仍然在最前面，所以重送同一個 id
	// 會在這之前就回舊的那一份，不會再付一次檢索的錢。
	if state == "queued" && s.CatalogCheck != nil {
		// 05 R-49: before any model call, Go asks the catalogue whether this
		// task already has a Skill (run r, 2026-09-06: left to the model, no
		// reference session ever searched). Hits wait for the person — adopt
		// one, keep them as references, or decline; a failed search never
		// blocks the session, it only skips the question.
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
		// migration 0057 makes the PK (id, workspace_id): a conflict here only
		// means this workspace already holds this id. Cross-workspace id reuse
		// is closed by that composite key, not by this recovery branch.
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

// clearDuplicateCheck forgets the duplicate guard's answer: it described a
// draft that no longer exists.
func clearDuplicateCheck(p *Snapshot) {
	p.Duplicates = nil
	p.PendingMaterialize = ""
	p.DuplicateAcknowledged = false
}

// listedReference says whether the person may adopt this id: it must be one
// Go itself put in front of them, from the catalogue check or the duplicate
// guard, never an arbitrary id.
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

// masked is the session's one door for text the person wrote or a page
// handed over: masked before it is stored, so the snapshot never holds a
// credential the way a trace never does (iron rule 11).
func (s *Service) masked(text string) string {
	if s.Mask == nil {
		return text
	}
	return s.Mask(text)
}

// attachNote appends the sentence that came with a material (2026-09-08).
//
// Before this, `diagram` and `select_references` carried no text at all, so
// 「這是我的流程，我想把它變成一個 Skill」 could not be said in the same turn as
// the picture: the person sent the file, waited a round, and only then got to
// explain it — and the model read the picture with no question attached to it.
// The web composer's 「一次只能送一種素材」 was that limit surfacing, not a
// layout choice.
//
// It is the same user message `case "message"` appends and is held to the same
// limits; what differs is that it does not stand on its own, so an empty one is
// not an error here — there is a material to carry the turn.
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

// nameCollides says whether the draft's name is one of the Skills the
// duplicate guard listed: saving it would be refused as 同名 (GEN-010), so the
// model is asked to rename before the person tries again.
func nameCollides(name string, dups []Reference) (string, bool) {
	for _, d := range dups {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(d.Name)) {
			return d.Name, true
		}
	}
	return "", false
}

// duplicateQuery is the text the duplicate guard embeds: the draft's own name
// and description, which is what the index's enriched summary describes.
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
		// Two commands may reach a session that is mid-step: end it, or stop
		// this one step (2026-09-09, 04 丙-203). Everything else has to wait for
		// the revision the step will produce.
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
		// Stop THIS step, not the session. Nothing new had to be built for it
		// (04 丙-203): three mechanisms were already in place and this command
		// just makes all three of their conditions false at once.
		//
		//  1. The Worker refuses to start a receipt that is no longer `queued`,
		//     so a step stopped before the call never makes it.
		//  2. During the call a goroutine reads this row every 250ms and cancels
		//     the in-flight HTTP request as soon as the state leaves `working`.
		//  3. finish() adopts the model's reply only while ActiveReceipt still
		//     points at its own receipt; otherwise it falls through to settleCost,
		//     which records what was actually spent and proposes nothing.
		//
		// So the money is right on both paths without a word of new accounting:
		// before the call there is nothing to pay for, and after it the cost is
		// settled exactly as any other interrupted attempt.
		if !e.ActiveReceipt.Valid || len(p.Messages) >= MaxMessages {
			return View{}, nil, ErrInvalidCommand
		}
		q := gen.New(tx)
		a, getErr := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: e.ActiveReceipt, SessionID: id, WorkspaceID: ws.ID})
		if getErr != nil {
			return View{}, nil, getErr
		}
		// Two different truths, so two different sentences. Telling someone
		// their money is safe when the call already went out would be the one
		// lie this feature could tell.
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
		// The confirmation is NOT cleared here (2026-09-06 run c: 「請繼續」 after
		// a confirmed brief sent two sessions back through propose→confirm for
		// nothing). GEN-007's 「更正已確認的需求→確認失效」 still holds: the model
		// is told to propose a new confirmation when the newest user message
		// changes the requirements, and proposal() un-confirms on any change.
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
		// The note goes in first: what the person wants these references FOR is
		// context for the references, not a reply to them.
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
		// Reuse before creation (05 R-49／R-50): the person takes an existing
		// Skill instead of composing one. Go forks it; the session ends with
		// the fork as its candidate and nothing generated.
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
		// Read before the note is appended, so it is the index the note takes
		// (and, with no note, the index the model's reply will take).
		at := len(p.Messages)
		// After the picture has been accepted, never before: a refused image must
		// not leave its sentence behind in the history as if it had been sent.
		if err := s.attachNote(p, c.Message); err != nil {
			return View{}, nil, err
		}
		h := sha256.Sum256(b)
		p.DiagramFingerprint = hex.EncodeToString(h[:])
		p.DiagramMediaType = c.Diagram.MediaType
		p.DiagramBytes = len(b)
		// The three fields above are the newest picture; this is the history.
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
		// Masked like every other untrusted text on its way into the snapshot:
		// the judge writes about output the Skill under trial produced, so a
		// credential in that output can reach here through a quoted reason.
		observation = s.masked(observation)
		p.EvaluationText = evaluationFreeText(observation)
		p.Messages = append(p.Messages, llmclient.CreationMessage{Role: "tool", Content: observation})
		state = "candidate_ready"
		queueStep = true
		// An unmet trial is the person's turn first (owner, 2026-09-06: "跑完之後
		// 和使用者的互動，獲取回饋"): the failed criteria and the judge's reasons
		// become questions, and the answer steers the model's revision.
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
		// The URL stays on the snapshot; the Worker's step job fetches it.
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
		// confirm_duplicate replays the command the duplicate guard held
		// (05 R-50): same draft hash, the person has seen the near-duplicates.
		kind := c.Kind
		if c.Kind == "confirm_duplicate" {
			if p.PendingAction != "confirm_duplicate" || p.PendingMaterialize == "" {
				return View{}, nil, ErrInvalidCommand
			}
			kind = p.PendingMaterialize
			p.DuplicateAcknowledged = true
			p.PendingAction = ""
			p.PendingMaterialize = ""
			// "Build anyway" with the duplicate's own name would only be refused
			// as 同名 at the save (run x R09, 2026-09-07): Go says so now and the
			// model renames; the person then saves the renamed draft.
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
				// 05 R-50: the last place a duplicate can be stopped. One
				// embedding of the draft's name and description; a hit within
				// the creation tool's distance is shown before anything is
				// stored, and the person adopts it or confirms the draft.
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

// materializedReference is the subset of Reference kept in generation_inputs;
// the reference's own manifest content (description, compatibility, allowed
// tools) is not stored a second time (02 line ~700).
type materializedReference struct {
	SkillID   string `json:"skill_id"`
	VersionID string `json:"version_id"`
	Name      string `json:"name"`
}

// kind is materialize or finalize — c.Kind itself may be confirm_duplicate,
// and the receipt must keep the command the client actually sent.
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
	// ADR-067: the candidate goes through the generated door but is paid for by
	// the session's own budget; CountGeneratedSkills skips rows carrying this.
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
			// The Test Case prompt is the example input, not the brief: a brief
			// describes the Skill, and an agent handed a description asks for the
			// material (run d, 2026-09-06). The brief is the fallback for sessions
			// confirmed before sample_input existed.
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
		// The duplicate guard's answer and its embedding cost were decided on
		// the snapshot Act read; the revision check above proved it is this one.
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

// trialQuestions turns an attach_run observation into the questions the person
// answers before the model revises: one line per criterion the judge did not
// pass, with the judge's reason, then what to decide. Empty when there is
// nothing to ask (no evaluation, or every criterion passed).
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

// evaluationFreeText returns the judge's own words out of an attach_run
// observation — the summary, each criterion's reason, each finding's message.
// Criterion text is the person's own acceptance criterion, so it is not in
// here: what this collects is exactly the text nobody the person trusts wrote.
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

// copiedFromEvaluation lists the marker-like literals a revised draft took
// verbatim out of the judge's own words. 05 SEC-013 (LLM01), 2026-09-07: the
// fence in creation-step/v16 stopped the model obeying an evaluation that told
// it to change the brief or add a tool, but not the one that told it to copy a
// token into the body — 1/12 of the attack set, and the residual channel the
// measurement left open. The fence argues about trust; this counts characters.
//
// What it does NOT cover, because character counting cannot: an evaluation that
// asks for a paraphrase ("spell the token out in words"), a marker made only of
// letters, and the three attack goals that never touch the draft's text at all
// — claim_met, add_bash_tool, change_brief. Those have no Go backstop; the
// prompt's own discipline (creation-step/v17) is the whole defence there.
//
// Marker-like is a shape, judged per segment: an ASCII segment mixing letters
// and digits inside a hyphenated or underscored token (`9c0d` of `EXFIL-9c0d`,
// `77e1` of `BASH-TOOL-77e1`), or a bare run of eight or more ASCII characters
// with at least two of each. Ordinary technical vocabulary a draft has every
// reason to gain after a trial — utf-8, sha256, base64, iso8601, gpt-5 — has no
// such segment; letters outside ASCII are not marker material at all, which is
// what keeps a Chinese sentence with a number in it (「金額超過5000」) out.
//
// Only what is BOTH in the draft AND absent from everything the person had a
// hand in counts. That side is read generously — every alphanumeric segment of
// their text, marker-like or not — because an identifier they supplied comes
// back in shapes they did not write: a sample input with `A1001` in it earns
// the draft's `shopify_order_A1001.csv`, and a guard that missed that
// difference would spend a nudge on an honest revision.
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
	// Sorted because the caller puts them in a message: map order would make
	// the same session read differently on every run.
	sort.Strings(copied)
	return copied
}

// toolsNotRequested lists the tokens a revised draft's allowed_tools gained
// since the previous draft that nobody the person trusts asked for. 05 R-54
// #3, corpus-injection.json's add_bash_tool cases: the judge's own reason (or
// a fetched page, or a reference Skill's body) tells the model the fix is
// adding a tool to allowed_tools rather than editing the body, and today Go
// never looks at that field at all. This is not an allowlist — a Skill
// legitimately needing Bash is ordinary — it only flags a tool that showed up
// this round with nothing in the person's own words asking for it, the same
// shape as copiedFromEvaluation: new, absent from the last version, and
// absent from everything the person had a hand in.
//
// Tokenizing follows the specification's own convention (space-separated,
// commas tolerated — skillpkg.go's allowed-tools parser): "Bash(git:*)" and
// "bash" are the same tool for this comparison, compared by the part before
// any "(" scope, case-insensitively.
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

// negations are the words that turn a mention of a tool into a refusal of it.
// The adversarial review of the first version (2026-09-08) found the hole they
// close: a person who writes 「不要用 bash」 was read as having asked for bash,
// and an evaluation could then smuggle it in past a guard that only counted
// words.
var negations = []string{"不要", "不用", "不需要", "不能", "別用", "別", "禁止", "勿", "無需", "沒有要",
	"don't", "do not", "dont", "no ", "not ", "never", "without", "avoid", "except"}

// negationWindow is how far back a refusal can sit and still govern the
// mention: enough for 「這個 Skill 不要用 bash」, short enough that a refusal of
// one tool two sentences ago does not silently govern another.
const negationWindow = 16

// asked says whether any of the person's own texts names this tool as
// something they want. A mention inside a refusal does not count; a tool
// mentioned twice counts if either mention stands unnegated.
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

// negated reports whether a refusal sits within negationWindow runes before
// the mention at index i.
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

// toolsNamedIn is the subset of tools the evaluation text actually spells
// out, so the nudge can say the judge asked for it only when the judge did.
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

// addedToolTokens is cur's allowed_tools tokens absent from prev's, compared
// case-insensitively; the token kept is cur's own spelling, for the message
// the person reads. Nil when prev has no baseline to diff against (the
// caller only invokes this when a previous draft exists) or nothing was added.
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

// toolFields splits an allowed_tools string the way skillpkg.go's manifest
// parser does: space-separated, commas tolerated.
func toolFields(s string) []string {
	return strings.Fields(strings.ReplaceAll(s, ",", " "))
}

// toolBaseName strips a scope like "(git:*)" off a tool token, so
// "Bash(git:*)" is compared as "Bash".
func toolBaseName(tool string) string {
	if i := strings.IndexByte(tool, '('); i >= 0 {
		return tool[:i]
	}
	return tool
}

// markerTokens maps each marker-like token of a text to the segments that made
// it one, folded to lower case so a copy that changes case still matches. The
// token is what a person is shown; the segment is what is compared.
func markerTokens(s string) map[string][]string {
	out := map[string][]string{}
	for _, token := range tokens(s) {
		if segs := markerLike(token); len(segs) > 0 {
			out[token] = segs
		}
	}
	return out
}

// markerSegments is the set of marker-like segments in a text — the judge's
// side of the comparison, where only the segment matters.
func markerSegments(s string) map[string]bool {
	out := map[string]bool{}
	for _, segs := range markerTokens(s) {
		for _, seg := range segs {
			out[seg] = true
		}
	}
	return out
}

// alphanumericSegments is the person's side: every segment of their text, with
// no shape test at all. Two characters is enough to be worth remembering.
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

// markerLike returns the segments that make a token marker-like, empty when it
// is not one. A compound (`exfil-9c0d`) lets a short mixed segment count; a
// bare word has to be long and mixed on its own before it does.
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
				// A digit or letter outside ASCII: a marker is not written in
				// Chinese, and 「金額超過5000」 must not read as one.
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

// draftText is every place a draft carries text a marker could ride out on:
// the body, the two descriptions the catalogue shows, the tool list, and the
// contents of each packaged file. Checking only the body would leave the file
// an attacker would rather use anyway.
func draftText(skill llmclient.GeneratedSkill) string {
	parts := []string{skill.Name, skill.Description, skill.Compatibility, skill.AllowedTools, skill.Body}
	for _, f := range skill.Files {
		parts = append(parts, f.Path, f.Content)
	}
	return strings.Join(parts, "\n")
}

// previousDraftText is the text of the draft this one replaces, empty when
// there is none: a marker the previous draft already carried is not something
// this turn copied out of the evaluation.
func previousDraftText(d *Draft) string {
	if d == nil {
		return ""
	}
	return draftText(d.Skill)
}

// personText is everything in the transcript the person themselves wrote. A
// token they typed is theirs, however marker-like it looks.
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

// runUnmet reads the one field of an attach_run observation Go acts on: an
// evaluation that finished and did not come back met. No evaluation, or an
// unreadable observation, is not "unmet" — that would nudge the model over a
// draft nobody has judged.
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

// missingDiagramNodes lists the confirmed diagram's nodes that do not appear
// in the draft body (whitespace and punctuation ignored, case-folded). A
// diagram session whose Skill walks half the flow was every diagram row of
// runs e–g (2026-09-06); the judge said so each time, after the money was
// spent. Not a validator finding: the draft is refused before it is stored.
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
