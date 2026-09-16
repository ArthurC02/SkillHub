package creation

import (
	"context"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

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

func (s *Service) selectReferences(ctx context.Context, ws identity.Workspace, p *Snapshot, c Command) (commandOutcome, error) {
	if len(c.ReferenceSkillIDs) > MaxReferences || s.ResolveReference == nil {
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
