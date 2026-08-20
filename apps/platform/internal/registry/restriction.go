package registry

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// ErrEmptyRestriction: the empty string is neither a reason code nor "no hold".
// 0023's CHECK refuses it, and catching it here turns a constraint violation in
// the middle of somebody else's transaction into a named error at the call.
var ErrEmptyRestriction = errors.New("registry: access restriction must be a non-empty reason code, or nil to lift the hold")

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
// It takes the caller's transaction and never begins one. catalog locks the row,
// writes this column and writes the audit event in a single commit, so a hold
// can never be in force without the event that explains it, or explained without
// being in force (iron rule 9). A function that opened its own transaction would
// split that guarantee into two that can fail apart.
//
// Unscoped by workspace, like the query: a licensing hold is an operator action
// that has to reach a fork in somebody else's workspace (02:SEC-011), and it
// reads nothing about that workspace to do it.
func SetAccessRestriction(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, reason *string) error {
	if reason != nil && strings.TrimSpace(*reason) == "" {
		return ErrEmptyRestriction
	}
	return gen.New(tx).SetSkillAccessRestriction(ctx, gen.SetSkillAccessRestrictionParams{
		ID: skillID, AccessRestriction: reason,
	})
}
