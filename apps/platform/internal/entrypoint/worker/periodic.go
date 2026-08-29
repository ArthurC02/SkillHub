package worker

// Two periodic jobs this process runs for the platform rather than for a run.
//
// Both were "a cron somebody has to install", and both are still unticked on
// m4/release-checklist.md. What separates them from the retention sweeps that
// stay in cmd/maintenance is the line iron rule 6 actually draws: rule 6 keeps
// the WHEN of a deletion outside the code, because deleting user content on a
// deadline needs a human to have set the deadline. Neither of these deletes
// anything.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// --- partitions --------------------------------------------------------------

// PartitionCreateInterval is daily rather than monthly, and that is the cheap
// choice rather than the thorough one: a pass that finds all three months
// present costs one catalog query, so running it thirty times a month instead of
// once removes every question about which day a redeploy moved the timer to.
const PartitionCreateInterval = 24 * time.Hour

// PartitionCreateArgs carries nothing: the work is "make sure the next two
// months exist".
type PartitionCreateArgs struct{}

func (PartitionCreateArgs) Kind() string { return "partition_create" }

// PartitionCreateWorker pre-creates the monthly partitions of every partitioned
// table, and drops none.
//
// The half that drops stays in `maintenance rotate-partitions` behind its
// fail-closed retention variable. This half is here because its failure mode is
// the expensive one: miss two consecutive months and rows start landing in
// <table>_default, which no partition drop can ever reach (see
// partition.expiredMonths) — so PDM-006's retention silently stops applying to
// them, and getting them out again needs a maintenance window, the ACCESS
// EXCLUSIVE drain partition.createMonth's 23514 branch prints as four
// statements.
//
// In the worker and not the API for the ordinary reason: periodic jobs run on
// the elected leader, so several worker processes do not each issue the DDL.
type PartitionCreateWorker struct {
	river.WorkerDefaults[PartitionCreateArgs]
	Pool *pgxpool.Pool
}

// partitionedTables is every table whose owner declared it partitioned, named
// from the owning contexts' own constants rather than spelled again here: a
// table name copied into a composition root is one rename away from a job that
// maintains a table nobody writes to.
func partitionedTables() []string {
	return []string{analytics.PartitionedTable, trace.PartitionedTable}
}

func (w *PartitionCreateWorker) Work(ctx context.Context, _ *river.Job[PartitionCreateArgs]) error {
	var failures []error
	for _, table := range partitionedTables() {
		// Every table, even after one fails: a stuck month on trace_events must
		// not stop analytics_events getting next month's partition — the same
		// reason MaintainMonthly drops before it creates.
		report, err := partition.CreateUpcoming(ctx, w.Pool, table, time.Now())
		if len(report.Created) > 0 {
			slog.Info("partitions created", "table", table, "partitions", report.Created)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// --- enrichment backfill -----------------------------------------------------

// EnrichmentBackfillInterval is hourly. The worklist only grows when an import
// lands while the LLM service is unreachable, so this is a catch-up and not a
// pipeline; an hour late is on time.
const EnrichmentBackfillInterval = time.Hour

// enrichmentBackfillBatch bounds one pass. Each document costs one enrichment
// call on the deployment-wide LiteLLM key (iron rule 8), so an unbounded pass
// after a long outage would be an unbounded bill nobody asked for. Fifty an hour
// drains a day's backlog inside a day.
const enrichmentBackfillBatch = 50

// EnrichmentBackfillArgs carries nothing: the work is "whatever is pending".
type EnrichmentBackfillArgs struct{}

func (EnrichmentBackfillArgs) Kind() string { return "enrichment_backfill" }

// EnrichmentBackfillWorker re-runs index-time enrichment for the documents an
// import left pending (INGEST-009 重新索引, ADR-013 §1).
//
// The same ingest.Service call cmd/reindex makes, on a timer. What that
// command's existence assumed and never said is that somebody notices: a skill
// imported while apps/llm was restarting has no embedding, so ADR-013's vector
// leg cannot recall it, and the only symptom is that searching for it in the
// other language finds nothing. Nobody reports that.
//
// A nil Svc is a working deployment and not a misconfiguration: it is what a
// deployment with no LLM_SERVICE_URL gets, and there the honest behaviour is to
// do nothing rather than to fail hourly. It is a nil field rather than a skipped
// registration so the roster is the same in every deployment — a scheduled job
// that exists in some and not others is a roster no test can pin.
type EnrichmentBackfillWorker struct {
	river.WorkerDefaults[EnrichmentBackfillArgs]
	Svc *ingest.Service
}

func (w *EnrichmentBackfillWorker) Work(ctx context.Context, _ *river.Job[EnrichmentBackfillArgs]) error {
	if w.Svc == nil {
		return nil
	}
	done, failed, err := w.Svc.ReindexPending(ctx, enrichmentBackfillBatch)
	if err != nil {
		return fmt.Errorf("enrichment backfill: %w", err)
	}
	if done > 0 || failed > 0 {
		slog.Info("enrichment backfill pass", "enriched", done, "still_pending", failed)
	}
	return nil
}

// newBackfillService wires the ingest service the backfill needs: the import
// path's service, minus everything the import path does before enrichment.
//
// catalog's write is injected rather than imported by ingest (ADR-034), and
// ReindexPending refuses to spend an enrichment call without it — the same
// wiring cmd/reindex does, done here in the composition root that owns this
// process (ADR-032 §5).
func newBackfillService(pool *pgxpool.Pool, deps Deps) *ingest.Service {
	if deps.LLM == nil || deps.Store == nil {
		return nil
	}
	catalogSvc := &catalog.Service{Pool: pool}
	return &ingest.Service{
		Pool: pool, Store: deps.Store, LLM: deps.LLM,
		IndexSkill: func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
			return catalog.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
				SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
				EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
				Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
				EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
				EnrichmentPromptVersion: p.EnrichmentPromptVersion,
			})
		},
		PendingEnrichments: func(ctx context.Context, limit int32) ([]ingest.PendingEnrichment, error) {
			rows, err := catalogSvc.PendingEnrichments(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]ingest.PendingEnrichment, len(rows))
			for i, row := range rows {
				out[i] = ingest.PendingEnrichment{
					SkillID: row.SkillID, WorkspaceID: row.WorkspaceID,
					Name: row.Name, PackageObjectKey: row.PackageObjectKey,
				}
			}
			return out, nil
		},
	}
}
