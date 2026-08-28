package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// TakedownBefore is the state a platform-wide takedown replaced. The workspace
// comes back with it for the same reason RedistributionBefore carries one: the
// audit event is scoped to the workspace whose content changed, and the caller
// must not have to read the skill again to find out which one that is — it has
// no scope of its own to read it with.
type TakedownBefore struct {
	WorkspaceID pgtype.UUID
}

// SetTakedown takes a skill down regardless of whose workspace holds it.
//
// This is the second query Service.Takedown's ponytail note asked for, and the
// split it asked for: that method still carries the workspace predicate, because
// an owner withdrawing their own content and an operator answering an abuse
// report are different authorizations even though they write the same column.
// Everything downstream is shared on purpose — one `takedown_at`, so one 410
// Gone, one search exclusion and one ActionSkillTakedown, whoever set it.
// 02:533 forbids a second takedown flow for operators; this is why that holds
// structurally rather than by discipline.
//
// Same division as SetRedistribution next door: the column is registry's because
// `skills` is registry's table, while who may call the route, what evidence they
// must show and what the audit event records stay in internal/catalog.
//
// It takes the caller's transaction and never begins one, so the lock, the
// column, catalog's removal of the search document and catalog's audit event are
// a single commit. A takedown that committed without its explanation would be
// exactly the row a later abuse review cannot account for (iron rule 9).
//
// ErrAlreadyTakenDown and ErrNotFound are told apart under the lock rather than
// by a second read: the scoped path can afford a second read because it has a
// workspace to scope it to, and this one does not.
//
// It does not restore. Nothing does — clearing `takedown_at` would also have to
// put the search document back, and IndexSkill writes only name and summary,
// so a naive restore would silently drop the enrichment, the embedding and the
// scan. That gap predates this function and applies to the scoped path too;
// today the answer is `maintenance reindex`. See `04` 丙-80.
func SetTakedown(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, reason string) (TakedownBefore, error) {
	q := gen.New(tx)
	before, err := q.LockSkillForOperatorWrite(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TakedownBefore{}, ErrNotFound
	}
	if err != nil {
		return TakedownBefore{}, err
	}
	if before.TakedownAt.Valid {
		return TakedownBefore{}, ErrAlreadyTakenDown
	}
	if err := q.SetSkillTakedown(ctx, gen.SetSkillTakedownParams{
		ID: skillID, TakedownReason: &reason,
	}); err != nil {
		return TakedownBefore{}, err
	}
	return TakedownBefore{WorkspaceID: before.WorkspaceID}, nil
}
