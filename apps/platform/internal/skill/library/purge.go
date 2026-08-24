package registry

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// PurgeWorkspace is the registry's share of an account deletion (CORE-007,
// PDM-006 §6.1): the workspace's skills and their versions, but only the ones
// nothing outside the workspace depends on. Which skills those are is registry
// knowledge — a version somebody forked or a run used is retained with its
// owner de-identified instead, because hard deleting it would break a third
// party's provenance chain (DISC-003) and the immutability rule (iron rule 4),
// which punishes the wrong person.
//
// This is the one path allowed to hard delete frozen `skill_versions` rows, and
// it only works inside identity's transaction: the exemption is the named
// `SET LOCAL skillhub.purge = 'on'` flag that the 0013 trigger looks for, and
// SET LOCAL lasts exactly as long as that transaction. A version of this
// function that opened its own would be refused by the database — which is a
// pleasant way for ADR-034's "never open a transaction" rule to be enforced
// twice, but not one to rely on: the other four steps have no such backstop.
func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	_, err := q.PurgeUnreferencedSkills(ctx, workspaceID)
	return err
}
