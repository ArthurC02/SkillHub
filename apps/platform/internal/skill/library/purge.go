package registry

import (
	"context"
	"errors"
	"time"

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

// DeletionSweep is what one pass of PurgeDeletedSkills did, and what it could
// not do yet. The two counts are not decoration: a stubborn non-zero total has
// two opposite causes, and the operator has to tell them apart from the log line
// alone. Waiting shrinks as the grace period passes and is the job draining;
// Kept never shrinks, and that is the provenance rule working correctly.
type DeletionSweep struct {
	Purged  int64
	Waiting int64
	Kept    int64
}

// PurgeDeletedSkills is WS-005's grace purge: the skills a user deleted on their
// own, past the cutoff the deployment names, with the same three provenance
// exclusions PurgeWorkspace has above.
//
// Until 2026-08-25 this did not exist while the screen confirming the deletion
// said snapshots were "retained for the 30-day grace period, then purged"; that
// sentence was struck rather than reinstated, so this job restores no claim to
// any screen -- it only makes the deployment able to keep one (04 丙-63).
//
// Opposite transaction shape from its sibling, deliberately. PurgeWorkspace
// takes the caller's tx because it is one step of identity's single account
// deletion transaction and must commit or roll back with the other five
// (ADR-034). This one has no caller transaction to join -- it is a cron sweep of
// its own backlog -- so it opens its own, which it would have to do regardless:
// `SET LOCAL skillhub.purge = 'on'` is the 0005 trigger's one exemption (0013)
// and SET LOCAL lasts exactly as long as the transaction it runs in. Same shape
// and same reason as audit.PurgeExpired.
//
// The grace period is a parameter with no default here, for the reason
// TRACE_RETENTION and the other two are deployment variables: PDM-006 §6.1's 30
// days is unratified, and a number compiled in would be this package inventing a
// deadline on which to delete somebody's content.
func (s *Service) PurgeDeletedSkills(ctx context.Context, grace time.Duration, limit int32) (DeletionSweep, error) {
	if grace <= 0 {
		// Fail closed. A zero window purges a skill deleted a second ago, and
		// "nobody configured it" must not be how a user's grace period ends.
		return DeletionSweep{}, errors.New("registry: deletion grace period must be positive")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DeletionSweep{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		return DeletionSweep{}, err
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-grace), Valid: true}
	q := gen.New(tx)
	purged, err := q.PurgeSkillsPastDeletionGrace(ctx, gen.PurgeSkillsPastDeletionGraceParams{
		Cutoff: cutoff, RowLimit: limit,
	})
	if err != nil {
		return DeletionSweep{}, err
	}
	// Counted after the delete and in the same transaction, against the same
	// cutoff: three numbers describing one state of the table rather than two.
	counts, err := q.CountSkillsAwaitingDeletionGrace(ctx, cutoff)
	if err != nil {
		return DeletionSweep{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeletionSweep{}, err
	}
	return DeletionSweep{Purged: purged, Waiting: counts.Waiting, Kept: counts.Kept}, nil
}
