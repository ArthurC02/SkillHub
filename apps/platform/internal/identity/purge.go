package identity

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// WorkspacePurge is one context's share of an account deletion: clear what you
// own for this workspace, on the transaction I am already in. Taking the
// caller's *gen.Queries rather than a pool is the entire contract — CORE-007
// promises one transaction that either clears every context or none of them,
// and a step that opened its own would turn that promise into six that can each
// fail alone.
//
// They are injected rather than imported because every context imports this one
// for its workspace scope (iron rule 3), so `identity -> anyone` is a
// compile-time cycle. ADR-034 reads that cycle as the signal it is: these are
// peers, and deciding what "my rows are being deleted" means is each peer's own
// domain knowledge, not identity's.
type WorkspacePurge func(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error

type purgeStep struct {
	context string
	purge   WorkspacePurge
}

// purgeSteps is the ordered list of cross-context purge steps and the only
// place that order is written down. Both the refusal below and the transaction
// itself read it, so adding a seventh context is one line in one place and a
// missing injection cannot become a missing step.
func (s *Service) purgeSteps() []purgeStep {
	return []purgeStep{
		{"analytics", s.PurgeAnalytics},
		{"testlab", s.PurgeTestData},
		{"run", s.PurgeRunArtifacts},
		// registry before ingest, and this order is load-bearing. ingest's step
		// removes the import sources that no skill_versions row still points at;
		// registry's step is what deletes those version rows. Run the other way
		// round and every source of the account still backs a live version, so
		// none is removed — and nothing says so. No error, no row count out of
		// place, just the import provenance of a deleted account left behind.
		// Guarded rather than only documented: the account purge integration test
		// seeds a version with a source and asserts the source is gone, which
		// swapping these two lines makes fail.
		{"registry", s.PurgeSkills},
		{"ingest", s.PurgeImportSources},
	}
}

// requirePurgeSteps refuses the entire batch when any context's step is
// missing, and is called before the worklist is even read.
//
// This is a compliance property, not tidiness. Every other fail-closed check in
// this codebase guards against a wrong answer; this one guards against a purge
// that commits, reports success, and leaves another context's rows in place —
// a user told their data is gone while it is still there, with nothing anywhere
// going red. Failing the batch is the recoverable outcome: the deletion request
// stays on the worklist and the next run, on a correctly wired deployment,
// finishes it.
func (s *Service) requirePurgeSteps() error {
	var missing []string
	for _, step := range s.purgeSteps() {
		if step.purge == nil {
			missing = append(missing, step.context)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("identity: account purge steps not injected for %s; refusing to purge",
			strings.Join(missing, ", "))
	}
	return nil
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
	if err := s.requirePurgeSteps(); err != nil {
		return 0, err
	}
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

	// Five other contexts own rows in this workspace, and each decides for itself
	// what an account deletion means for them — analytics de-identifies, the
	// registry keeps whatever a third party forked (ADR-034). They run here,
	// inside this transaction and after the SET LOCAL above, because that is what
	// makes the whole account deletion atomic and what lets the registry's step
	// past the immutability trigger at all.
	for _, ws := range workspaces {
		for _, step := range s.purgeSteps() {
			if err := step.purge(ctx, q, ws.ID); err != nil {
				return fmt.Errorf("purge %s: %w", step.context, err)
			}
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
