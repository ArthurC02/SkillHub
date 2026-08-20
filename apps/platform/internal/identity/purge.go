package identity

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/pgconv"
)

// ObjectRemover is the slice of object storage the purge needs.
type ObjectRemover interface {
	Remove(ctx context.Context, key string) error
}

// PurgeExpiredAccounts carries out the account deletions whose grace period has
// run out (CORE-007). PDM-006 §6.1 splits the work in two, and so does this:
//
//	hard delete      private uploaded content — dataset files, run and download
//	                 artifacts, and the skills nobody outside the account
//	                 references — objects included. This is the substance of the
//	                 NFR-002 promise that a user can really delete their data.
//	de-identify      what has to stay: versions someone else forked or a run
//	                 used, and the user and workspace rows they hang off. Hard
//	                 deleting those would break a third party's provenance chain
//	                 (DISC-003) and the immutability rule (iron rule 4), which
//	                 punishes the wrong person.
//
// Idempotent (iron rule 9): the final write of each account is the tombstone
// that takes it off this worklist, so re-running finds nothing left to do, and
// an account whose transaction failed is retried unchanged on the next call.
//
// Scheduling is not this function's business — deployment owns that. limit caps
// one call; grace is a parameter so a shortened retention policy applies to
// requests already in flight (and so tests do not have to wait 30 days).
func (s *Service) PurgeExpiredAccounts(ctx context.Context, store ObjectRemover, grace time.Duration, limit int32) (purged int, err error) {
	q := s.queries()
	ids, err := q.ListAccountsPastGrace(ctx, gen.ListAccountsPastGraceParams{
		Cutoff: pgconv.Timestamptz(time.Now().Add(-grace)),
		Limit:  limit,
	})
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := s.purgeAccount(ctx, store, id); err != nil {
			// One bad account must not strand the rest of the batch; the row
			// stays on the worklist and the next run retries it.
			slog.Error("account purge failed", "user_id", uuidText(id), "error", err)
			continue
		}
		purged++
	}
	return purged, nil
}

func (s *Service) purgeAccount(ctx context.Context, store ObjectRemover, userID pgtype.UUID) error {
	q := s.queries()
	workspaces, err := q.ListWorkspacesByOwner(ctx, userID)
	if err != nil {
		return err
	}

	// Objects go before the transaction on purpose. Object storage has no
	// rollback, so the two failure modes are not symmetric: an object deleted
	// ahead of a transaction that later fails leaves rows pointing at missing
	// files in an account that is being deleted anyway, and the next run
	// finishes the job. The other order can leave a user's uploaded file alive
	// with nothing left in the database that knows it exists.
	for _, ws := range workspaces {
		keys, err := q.ListWorkspaceObjectKeys(ctx, ws.ID)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := store.Remove(ctx, key); err != nil {
				return err
			}
		}
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Unlocks row deletion on the tables 0005 froze, for this transaction only
	// (0013). Retention deletion was always outside the immutability rule;
	// application code never sets this.
	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		return err
	}
	q = q.WithTx(tx)

	for _, ws := range workspaces {
		// ADR-029 決策 5: funnel events and beta feedback are de-identified, not
		// deleted. Aggregates have to survive a departure — the north-star metric
		// and the funnel are compared across quarters — and a beta tester's words
		// are the evidence the scope review rests on. The link to the person is
		// what goes, and it goes here rather than by cascade because the purge
		// anonymises the workspace row instead of deleting it, so 0029's
		// ON DELETE SET NULL never fires.
		if _, err := q.DetachWorkspaceAnalytics(ctx, ws.ID); err != nil {
			return err
		}
		if _, err := q.DetachWorkspaceFeedback(ctx, ws.ID); err != nil {
			return err
		}
		if _, err := q.DeleteWorkspaceArtifacts(ctx, ws.ID); err != nil {
			return err
		}
		if _, err := q.DeleteWorkspaceDatasets(ctx, ws.ID); err != nil {
			return err
		}
		if _, err := q.PurgeUnreferencedSkills(ctx, ws.ID); err != nil {
			return err
		}
		if _, err := q.PurgeUnreferencedSkillSources(ctx, ws.ID); err != nil {
			return err
		}
	}
	if _, err := q.DeleteUserIdentities(ctx, userID); err != nil {
		return err
	}
	if _, err := q.DeleteUserSessions(ctx, userID); err != nil {
		return err
	}
	if _, err := q.AnonymizeWorkspacesByOwner(ctx, userID); err != nil {
		return err
	}
	if _, err := q.AnonymizeUser(ctx, userID); err != nil {
		return err
	}
	// The actor is the account itself: the trail has to record that the purge
	// happened, and after the tombstone above the id no longer identifies a
	// person (PDM-006 6.1 "僅保留去識別化 actor ID").
	if err := audit.Log(ctx, q, audit.Event{
		Actor:        userID,
		Action:       audit.ActionAccountPurge,
		ResourceType: audit.ResourceAccount,
		ResourceID:   userID,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func uuidText(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
