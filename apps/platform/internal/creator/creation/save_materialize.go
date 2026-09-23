package creation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
)

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
			p.appendMessage("tool", fmt.Sprintf("使用者仍要建立自己的版本，但草稿名稱「%s」與目錄裡那份相同，保存會被拒絕；請只改名稱（描述其差異），其餘內容不變，重新交出草稿。", taken))
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
