package ingest

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// PurgeWorkspace is ingest's share of an account deletion (CORE-007, PDM-006
// §6.1): the import provenance of versions that are gone. A source still
// backing a retained version stays, because that version is still readable by
// whoever forked it and a version whose origin is unknown is worse provenance
// than none (DISC-003).
//
// "Versions that are gone" is why this runs *after* the registry step — see the
// ordering note on identity's purgeSteps. Called before it, every source of the
// account still backs a live version and none of them would be removed.
//
// Runs on the caller's transaction and never opens one of its own
// (ADR-034): the account purge is one transaction, all of it or none of it.
func (*Service) PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	_, err := q.PurgeUnreferencedSkillSources(ctx, workspaceID)
	return err
}
