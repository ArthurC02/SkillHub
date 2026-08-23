package registry

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// ErrEmptyRestriction: the empty string is neither a reason code nor "no hold".
// 0023's CHECK refuses it, and catching it here turns a constraint violation in
// the middle of somebody else's transaction into a named error at the call.
var ErrEmptyRestriction = errors.New("registry: access restriction must be a non-empty reason code, or nil to lift the hold")

type RestrictionBefore struct {
	WorkspaceID       pgtype.UUID
	AccessRestriction *string
}

// SetAccessRestriction writes the 0023 licensing hold onto a skill: a reason
// code holds its materials back, nil lifts the hold. One statement for both
// directions, so updated_at has one place it can be forgotten rather than two.
//
// The column is registry's — `skills` is registry's table — but the decision is
// not: which reason codes exist, what sentence each one renders, who may call
// the operator routes and what the audit event records all stay in
// internal/catalog, next to the reads they govern. Only the write came back to
// the owner (ADR-033 clearance path 4); a column write is not a policy.
//
// It takes the caller's transaction and never begins one. The lock, this column
// and catalog's audit event are a single commit, so a hold can never be in force
// without the event that explains it, or explained without being in force (iron
// rule 9). A function that opened its own transaction would split that guarantee
// into two that can fail apart.
//
// 2026-08-21 (DDD-031, ADR-035 B 組): the FOR UPDATE that makes the returned
// before-state trustworthy used to be taken by catalog, one statement before it
// called this function. That left the owner of the write unable to say whether
// its own writes were serialised — a second caller locking the wrong row, or no
// row, left no trace here. The lock now happens where the write happens, and the
// before-state comes back as this function's result instead of as something the
// caller read for itself. Nothing about the *meaning* of the row moved: catalog
// still decides what the reason codes are and still writes the audit event.
//
// Taking the lock a second time in a transaction that already holds it is a
// no-op in Postgres, so callers that legitimately locked the row earlier in the
// same transaction are unaffected.
//
// Unscoped by workspace, like the two statements it runs: a licensing hold is an
// operator action that has to reach a fork in somebody else's workspace
// (02:SEC-011), and it reads nothing about that workspace to do it.
//
// Returns ErrNotFound for a skill that does not exist or is soft-deleted, which
// is the same answer for both by design — see ErrNotFound.
func SetAccessRestriction(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, reason *string) (RestrictionBefore, error) {
	if reason != nil && strings.TrimSpace(*reason) == "" {
		return RestrictionBefore{}, ErrEmptyRestriction
	}
	q := gen.New(tx)
	before, err := q.LockSkillForOperatorWrite(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RestrictionBefore{}, ErrNotFound
	}
	if err != nil {
		return RestrictionBefore{}, err
	}
	if err := q.SetSkillAccessRestriction(ctx, gen.SetSkillAccessRestrictionParams{
		ID: skillID, AccessRestriction: reason,
	}); err != nil {
		return RestrictionBefore{}, err
	}
	return RestrictionBefore{
		WorkspaceID: before.WorkspaceID, AccessRestriction: before.AccessRestriction,
	}, nil
}
