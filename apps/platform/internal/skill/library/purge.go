package registry

import (
	"context"
	"errors"
	"log/slog"
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
//
// It still does not enqueue the package objects of the versions it deletes, so
// their bytes outlive the last row that knew about them. See
// [Service.CollectOrphanObjects] for why that could not be fixed from here.
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
//
// Like its sibling above it leaves the package objects of the versions it
// deletes behind; [Service.CollectOrphanObjects] says why the enqueue is not
// here yet.
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

// ObjectRemover is the slice of object storage the collector needs. Remove
// only: a package object is content-addressed, so there is nothing to read and
// nothing to look up by anything other than the key already on the worklist.
type ObjectRemover interface {
	Remove(ctx context.Context, key string) error
}

// Collection is what one pass of CollectOrphanObjects did. Three numbers for the
// reason DeletionSweep has two: an operator who sees a queue that is not
// shrinking has to tell "the keys came back" from "the store will not take the
// delete" without reading this file. Dropped rising with Collected at zero is
// the first; Depth standing still across passes is the second.
type Collection struct {
	Dropped   int64
	Collected int
	Depth     int64
}

// CollectOrphanObjects removes the package objects whose last referencing
// skill_versions row is gone (04 丙-73, 0039).
//
// It is the second half of a mechanism whose first half is not written. The
// queue is filled by EnqueueOrphanObjectCandidates, which has to run inside a
// purge transaction and before the delete -- that is the only moment the keys
// are still readable -- and neither purge can hand over the ids it is about to
// take. Both are one statement whose `purgeable` CTE selects and deletes at
// once, `:execrows`, no RETURNING, and no query anywhere lists the skills in
// either purge's scope. Recomputing the set in Go would be a second definition
// of "purgeable" sitting beside the DELETE's own, and the first thing a drift
// between the two does is leak the keys the list missed, silently and forever.
// So the enqueue belongs in those statements as a third CTE off the same
// `purgeable`, which is a db/queries change, not a change here.
//
// **No retention window and no environment variable**, unlike every other
// subcommand in cmd/maintenance, and that is not an omission. Those sweep on a
// deadline somebody has to have signed; this one has no deadline at all. It
// removes what is already unreferenced, and "unreferenced" is a fact the
// database answers -- ListCollectableObjects's NOT EXISTS -- not a policy. A
// fail-closed variable here would gate a job that deletes nothing anybody can
// still reach on a number that would mean nothing.
//
// Order per key: object first, queue row second. The reverse loses the key on a
// crash and the bytes become unfindable again, which is the whole failure this
// table exists to prevent. Object-then-row is safe to repeat (iron rule 9):
// removing an object twice succeeds, and a pass that died between the two steps
// re-lists the same key and deletes an entry whose bytes are already gone.
//
// The one mistake that must never happen is deleting an object a fork still
// reads. Nothing in this function decides that: the queue is candidates only,
// and both the drop and the list ask the database, at sweep time, whether any
// skill_versions row still names the key. A key that came back -- re-importing
// identical bytes makes a new version with the same content-addressed key --
// leaves the worklist with its object untouched.
func (s *Service) CollectOrphanObjects(ctx context.Context, store ObjectRemover, limit int32) (Collection, error) {
	if store == nil {
		return Collection{}, errors.New("registry: object collection needs an object store")
	}
	q := gen.New(s.Pool)
	dropped, err := q.DropReferencedCollectionEntries(ctx)
	if err != nil {
		return Collection{}, err
	}
	result := Collection{Dropped: dropped}

	keys, err := q.ListCollectableObjects(ctx, limit)
	if err != nil {
		return result, err
	}
	for _, key := range keys {
		if err := store.Remove(ctx, key); err != nil {
			// Leave the row: the next pass tries the same key again. Dequeueing
			// here would be recording a deletion that did not happen, and there
			// is nothing left in the database to rediscover the key from.
			slog.Warn("orphan package object not removed; will retry", "error", err)
			continue
		}
		if err := q.DeleteObjectCollectionEntry(ctx, key); err != nil {
			return result, err
		}
		result.Collected++
	}

	// Read after the pass, so the number describes what is left rather than what
	// was there. A depth that does not move across passes is the sweep having
	// stopped working.
	result.Depth, err = q.CountCollectableObjects(ctx)
	return result, err
}
