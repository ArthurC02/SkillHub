package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// categorySourceOwner is what a caller-set category is attributed to. It is
// the only value this function ever writes: 0061's other value, "curated", is
// written exactly once, by that migration's own backfill, and never by a
// runtime write path.
const categorySourceOwner = "owner"

// SetCategory writes 05 R-19's owner-set PDM-001 shelf onto a skill, or clears
// it back to 尚未定值.
//
// This is not SetAccessRestriction or SetRedistribution next door wearing a
// different name, and the difference is who may call it. Those two are
// operator actions reaching into somebody else's workspace (02:SEC-011); this
// one is the ordinary workspace scope every other write in this file uses
// (ADR-011) — an owner saying what their own bytes are for, never anybody
// else's. There is deliberately no operator equivalent: R-19's whole point is
// that the platform stopped guessing, and an operator overriding an owner's
// answer would be the platform guessing again with extra steps.
//
// category == nil clears the shelf: both `category` and `category_source` are
// written as NULL, which is 0061's pairing CHECK read in the direction that
// clears rather than sets. A non-nil category is stamped with
// category_source = "owner" — never "curated", which this package never
// writes — so the read side (catalog.categoryLabel) can always say whose
// judgement a shelf is.
//
// The three legal shelf names are validated by the Handler, not here, for the
// reason SetAccessRestriction's doc comment gives for its own reason codes:
// which values are legal is catalog's/the contract's call, not registry's —
// this function only ever writes a pair the CHECK already allows.
//
// No transaction and no audit event: unlike Delete, Takedown and Fork, a
// category has no search-index projection to keep in step (catalog reads
// `skills.category` live, joined at query time — see search.sql's `cur`
// lateral) and nothing else commits alongside it, so a single UPDATE ...
// RETURNING is the whole write.
//
// Returns ErrNotFound for a skill outside the caller's workspace or already
// soft-deleted — deliberately the same answer for both (WS-006): reading
// somebody else's skill is allowed (WS-001 fork), saying what it is for is
// not (ADR-011).
func (s *Service) SetCategory(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, category *string) (gen.Skill, error) {
	var source *string
	if category != nil {
		v := categorySourceOwner
		source = &v
	}
	row, err := gen.New(s.Pool).SetSkillCategory(ctx, gen.SetSkillCategoryParams{
		ID: skillID, WorkspaceID: ws.ID, Category: category, CategorySource: source,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, err
	}
	// Raw gen.Skill, like Takedown next door and for the same reason: the
	// handler serves it straight through toSkillResponse, and the contract's
	// Skill schema has no category field to carry the DTO's extra ones (05 R-19
	// item 4's note lives on the read side, not on this write's response).
	return row, nil
}
