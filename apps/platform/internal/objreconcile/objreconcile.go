// Package objreconcile keeps what the database says about stored objects in
// agreement with what object storage actually holds (SEC-006, 04 丙-9).
//
// One periodic sweep, two directions:
//
//   - Retention. A download package past its expiry should not still be sitting
//     in the bucket. The sweep removes the bytes and marks the row purged; the
//     row itself stays readable, because "this expired" is a different answer to
//     02:WS-002 than "this never existed" (0028).
//   - Existence. `RunInputsStillAvailable` answers from the platform's own
//     deletion and expiry columns and never probes storage, which makes it an
//     optimistic upper bound: the column says the file is there, the object is
//     gone, and the comparison screen offers a re-run that fails when it fetches.
//     The download surface acquired the same gap the moment there was something
//     to download. This sweep finds those rows and stops them claiming it.
//
// It is a periodic job and deliberately not a probe on the read path: one HEAD
// per dataset per comparison buys a round trip for an answer already sitting in
// a column, and it would be bought on every read forever.
//
// Nothing here is destructive on a single observation. A row is marked only
// after its object has been missing in two consecutive rounds, counted in a
// table rather than in memory for the reason 0021 gives for the sandbox
// reconciler: the job is leader-elected and leadership moves between processes,
// so an in-memory count would reset exactly when the fault outlives a worker.
package objreconcile

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/pgconv"
)

const (
	// Interval is how often the sweep runs. Hourly, because neither thing it
	// answers changes by the minute: a retention boundary is days away and a lost
	// object stays lost. Two rounds before acting therefore means "still missing
	// an hour later", which no in-flight write survives.
	Interval = time.Hour
	// batch bounds one pass over each table. A sweep is not a migration; what it
	// does not reach this hour it reaches next hour.
	batch = 500
	// actAfterRounds is the threshold. Two and not one, because the consequence
	// of acting is telling a user their content is gone, and a store answering
	// 404 during a write or a transient fault must not cost somebody their file.
	actAfterRounds = 2

	kindDataset  = "dataset"
	kindArtifact = "artifact"
)

// ObjectStore is the slice of storage this needs: ask whether an object is
// there, and remove one. Exists must never answer false for a store it could
// not reach — see objstore.Client.Exists.
type ObjectStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	Remove(ctx context.Context, key string) error
}

// MarkFunc applies one owner context's correction to one of its own rows.
//
// The two writes this sweep needs are injected rather than imported: `artifacts`
// belongs to packaging and `datasets` to testlab, while this package is a
// generic scanner with no domain rules (ADR-032 §1). Importing either context
// here would point a generic subdomain at a core one — the layering upside down
// — so the composition root wires the owners' functions in instead and this
// package keeps knowing only that something needs correcting (ADR-033 clearance
// path 4).
//
// It takes a *gen.Queries and not a pool because the caller decides the
// transaction: purgeExpired writes on the pool, markLost writes inside the
// transaction that also carries the audit event.
type MarkFunc func(ctx context.Context, q *gen.Queries, id pgtype.UUID) error

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore
	// The owner writes, both required. Sweep refuses to run without them rather
	// than skipping the half it cannot apply: a sweep that silently corrects
	// nothing looks exactly like a sweep that found nothing to correct, and the
	// difference would only surface as users being served bytes that are not
	// there.
	RecordArtifactPurged MarkFunc
	RecordDatasetLost    MarkFunc
}

// Args carries nothing: the work is whatever the database currently claims, and
// a cursor in the payload would only let a stale job resume a scan that has
// moved on (same reasoning as run.SuperviseArgs).
type Args struct{}

func (Args) Kind() string { return "object_reconcile" }

type Worker struct {
	river.WorkerDefaults[Args]
	Svc *Service
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[Args]) error {
	return w.Svc.Sweep(ctx)
}

// Sweep is one pass. Exported so cmd/worker and the integration tests can run it
// directly rather than waiting for a timer.
func (s *Service) Sweep(ctx context.Context) error {
	// Before the store check, not after: a deployment with no store legitimately
	// has nothing to do, but a service that was wired without its owner writes is
	// a composition bug and must not be able to hide behind that.
	if s.RecordArtifactPurged == nil || s.RecordDatasetLost == nil {
		return errors.New("objreconcile: owner write functions not injected; refusing to sweep")
	}
	if s.Store == nil {
		return nil // no store wired: nothing to reconcile against
	}
	if err := s.purgeExpired(ctx); err != nil {
		return err
	}
	if err := s.checkArtifacts(ctx); err != nil {
		return err
	}
	if err := s.checkDatasets(ctx); err != nil {
		return err
	}
	return s.publishGauge(ctx)
}

// purgeExpired is SEC-006's retention half for download packages: the bytes go,
// the row stays and says it expired.
//
// Idempotent in both halves — Remove on an absent key succeeds and the mark is
// guarded by its own predicate — so a sweep interrupted anywhere is safe to run
// again (iron rule 9).
func (s *Service) purgeExpired(ctx context.Context) error {
	q := gen.New(s.Pool)
	rows, err := q.ListArtifactsPastRetention(ctx, batch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.Store.Remove(ctx, row.ObjectKey); err != nil {
			// Leave the row unmarked: next hour tries the same key again. Marking
			// it here would be recording a deletion that did not happen.
			slog.Warn("expired download object not removed; will retry",
				"artifact_id", pgconv.UUIDString(row.ID), "error", err)
			continue
		}
		if err := s.RecordArtifactPurged(ctx, q, row.ID); err != nil {
			return err
		}
		slog.Info("download artifact purged at retention", "artifact_id", pgconv.UUIDString(row.ID))
	}
	return nil
}

// checkArtifacts finds download packages the endpoint would still serve whose
// bytes are not there, and stops them being served.
func (s *Service) checkArtifacts(ctx context.Context) error {
	q := gen.New(s.Pool)
	rows, err := q.ListArtifactsClaimingObject(ctx, batch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		confirmed, err := s.sight(ctx, q, kindArtifact, row.ID, row.ObjectKey)
		if err != nil {
			return err
		}
		if !confirmed {
			continue
		}
		if err := s.markLost(ctx, kindArtifact, row.ID, row.WorkspaceID, row.ObjectKey,
			func(q *gen.Queries) error { return s.RecordArtifactPurged(ctx, q, row.ID) },
			audit.ResourceArtifact); err != nil {
			return err
		}
	}
	return nil
}

// checkDatasets is the 丙-9 half. Marking a dataset deleted is what corrects the
// optimistic upper bound: every read path that matters already filters on
// deleted_at, so nothing on any read path changes.
func (s *Service) checkDatasets(ctx context.Context) error {
	q := gen.New(s.Pool)
	rows, err := q.ListDatasetsClaimingObject(ctx, batch)
	if err != nil {
		return err
	}
	for _, row := range rows {
		confirmed, err := s.sight(ctx, q, kindDataset, row.ID, row.ObjectKey)
		if err != nil {
			return err
		}
		if !confirmed {
			continue
		}
		if err := s.markLost(ctx, kindDataset, row.ID, row.WorkspaceID, row.ObjectKey,
			func(q *gen.Queries) error { return s.RecordDatasetLost(ctx, q, row.ID) },
			audit.ResourceDataset); err != nil {
			return err
		}
	}
	return nil
}

// sight probes one object and reports whether it has now been missing for long
// enough to act on. An error return is a database failure and aborts the sweep;
// a storage failure is not one.
//
// A store that cannot be reached produces no sighting at all: Exists returns an
// error rather than false for anything short of a definitive 404, and that
// leaves the round count untouched. A whole sweep against a store that is down
// therefore changes nothing.
func (s *Service) sight(
	ctx context.Context, q *gen.Queries, kind string, id pgtype.UUID, key string,
) (bool, error) {
	present, err := s.Store.Exists(ctx, key)
	if err != nil {
		slog.Warn("object existence check failed; not counting a sighting",
			"kind", kind, "id", pgconv.UUIDString(id), "error", err)
		return false, nil
	}
	if present {
		// Deleting the row is what makes the count consecutive rather than
		// cumulative: a reappearance starts over at one.
		return false, q.ClearObjectSighting(ctx, gen.ClearObjectSightingParams{
			ResourceKind: kind, ResourceID: id,
		})
	}
	rounds, err := q.RecordObjectSighting(ctx, gen.RecordObjectSightingParams{
		ResourceKind: kind, ResourceID: id, ObjectKey: key,
	})
	if err != nil {
		return false, err
	}
	if rounds < actAfterRounds {
		slog.Warn("object missing behind a live row; waiting for a second round",
			"kind", kind, "id", pgconv.UUIDString(id), "object_key", key)
		return false, nil
	}
	return true, nil
}

// markLost applies the correction and records it, in one transaction so a row
// can never be marked without the trail saying so (iron rule 9).
//
// The audit event has no actor: nobody asked for this. That is exactly why it is
// audited rather than only logged — it is user content becoming unavailable
// without a user action, and slog is sampled and thrown away.
func (s *Service) markLost(
	ctx context.Context, kind string, id, workspaceID pgtype.UUID, key string,
	mark func(*gen.Queries) error, resourceType string,
) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	if err := mark(q); err != nil {
		return err
	}
	if err := audit.Log(ctx, q, audit.Event{
		Workspace: workspaceID, Action: audit.ActionObjectMissing,
		ResourceType: resourceType, ResourceID: id,
		// The key, because an operator's first question is which object; no
		// content and no name (iron rule 11).
		Metadata: map[string]any{"object_key": key, "rounds": actAfterRounds},
	}); err != nil {
		return err
	}
	// The row now agrees with storage, so there is nothing left to count.
	if err := q.ClearObjectSighting(ctx, gen.ClearObjectSightingParams{
		ResourceKind: kind, ResourceID: id,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Error("stored object missing; the row no longer claims it exists",
		"kind", kind, "id", pgconv.UUIDString(id), "object_key", key)
	return nil
}

// publishGauge reports what is still missing right now. A gauge and not a
// counter for the reason SBX-012's is one: the question is whether it is still
// true, and an increase() over a window cannot answer that.
func (s *Service) publishGauge(ctx context.Context) error {
	rows, err := gen.New(s.Pool).CountPersistentObjectSightings(ctx, actAfterRounds)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, row := range rows {
		metrics.ObjectsMissing.WithLabelValues(row.ResourceKind).Set(float64(row.Sightings))
		seen[row.ResourceKind] = true
	}
	// A kind that has stopped appearing is zero, not absent: a gauge left at its
	// last value would keep an alert firing after the problem is over.
	for _, kind := range []string{kindDataset, kindArtifact} {
		if !seen[kind] {
			metrics.ObjectsMissing.WithLabelValues(kind).Set(0)
		}
	}
	return nil
}
