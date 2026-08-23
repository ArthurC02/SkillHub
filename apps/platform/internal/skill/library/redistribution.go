package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// RedistributionBefore is the state a redistribution write replaced. The
// workspace comes back with it because the audit event has to be scoped to the
// workspace whose content changed, and the caller must not have to read the
// skill again to find out which one that is.
type RedistributionBefore struct {
	WorkspaceID    pgtype.UUID
	Redistribution string
}

// SetRedistribution writes the 0027/0036 redistribution verdict onto a skill.
//
// Same division as SetAccessRestriction next door, and for the same reason: the
// column is registry's because `skills` is registry's table, but which values an
// operator may assert, what each one means to a reader, who may call the route
// and what the audit event records all stay in internal/catalog. Writing one
// column is not a policy anyone can disagree with this package about.
//
// It takes the caller's transaction and never begins one, so the lock, the
// column and catalog's audit event are a single commit. This gate decides
// whether the platform will hand somebody a copy of a skill; a change to it that
// committed without its explanation, or an explanation that committed without
// the change, would be worse than either failing (iron rule 9).
//
// The lock is taken here rather than by the caller — the lesson DDD-031 left on
// SetAccessRestriction, applied at the start rather than after the fact: a
// writer that cannot see whether its own row was locked cannot promise that the
// before-state it returns is true.
//
// Values are not validated here beyond the column's CHECK. `self_supplied` in
// particular is refused by the caller, not by this function, because the reason
// it is refused is a statement about what operators may assert (0036: it is a
// fact about who supplied the bytes, not a verdict) and that is catalog's call.
//
// Returns ErrNotFound for a skill that does not exist or is soft-deleted, which
// is deliberately the same answer for both — see ErrNotFound.
func SetRedistribution(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, value string) (RedistributionBefore, error) {
	q := gen.New(tx)
	before, err := q.LockSkillForOperatorWrite(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RedistributionBefore{}, ErrNotFound
	}
	if err != nil {
		return RedistributionBefore{}, err
	}
	if err := q.SetSkillRedistribution(ctx, gen.SetSkillRedistributionParams{
		ID: skillID, Redistribution: value,
	}); err != nil {
		return RedistributionBefore{}, err
	}
	return RedistributionBefore{
		WorkspaceID: before.WorkspaceID, Redistribution: before.Redistribution,
	}, nil
}
