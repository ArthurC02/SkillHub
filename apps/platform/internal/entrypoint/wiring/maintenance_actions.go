package wiring

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	analytics "github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const defaultMaintenanceBatch = 100

func MaintenanceBatch() int32 {
	return envx.PositiveInt32("MAINTENANCE_BATCH", defaultMaintenanceBatch)
}

func MaintenanceDuration(key string) (time.Duration, error) {
	d, err := time.ParseDuration(os.Getenv(key))
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", key)
	}
	return d, nil
}

func AccountPurgeGrace() (time.Duration, error) {
	raw := os.Getenv("PURGE_GRACE")
	if raw == "" {
		return identity.AccountDeletionGrace, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < identity.AccountDeletionGrace {
		return 0, fmt.Errorf("PURGE_GRACE must be a Go duration of at least %s, the grace a deleting account is promised", identity.AccountDeletionGrace)
	}
	return d, nil
}

type previewFunc func(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error)

type maintenanceAction struct {
	job     string
	tier    operations.ActionTier
	preview previewFunc
}

var maintenanceActions = []maintenanceAction{
	{operations.JobPurgeAccounts, operations.TierDestructive, previewAccountPurge},
	{operations.JobPurgeRunArtifacts, operations.TierDestructive, previewRunArtifactPurge},
	{operations.JobPurgeDatasets, operations.TierDestructive, previewDatasetPurge},
	{operations.JobPurgeDeletedSkills, operations.TierDestructive, previewDeletedSkillPurge},
	{operations.JobCollectObjects, operations.TierDestructive, previewObjectCollection},
	{operations.JobCheckSources, operations.TierReversible, previewSourceCheck},
	{operations.JobPurgeAudit, operations.TierDestructive, previewAuditPurge},
	{operations.JobPurgeFeedback, operations.TierDestructive, previewFeedbackPurge},
	{operations.JobRotatePartitions, operations.TierDestructive, previewPartitionRotation},
}

func MaintenanceActions(pool *pgxpool.Pool) []operations.Action {
	actions := make([]operations.Action, len(maintenanceActions))
	for i, a := range maintenanceActions {
		preview := a.preview
		actions[i] = operations.Action{
			Name: operations.MaintenanceJobAction(a.job), Tier: a.tier,
			Preview: func(ctx context.Context) (operations.Preview, error) { return preview(ctx, pool) },
		}
	}
	return actions
}

func count(key string, n int64, atMost bool) operations.PreviewCount {
	return operations.PreviewCount{Key: key, Count: n, AtMost: atMost}
}

func batched(counts ...operations.PreviewCount) operations.Preview {
	return operations.Preview{Counts: counts, BatchLimit: MaintenanceBatch()}
}

func previewAccountPurge(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	grace, err := AccountPurgeGrace()
	if err != nil {
		return operations.Preview{}, err
	}
	accounts, sessions, err := (&identity.Service{Pool: pool}).AccountPurgeBacklog(ctx, grace)
	if err != nil {
		return operations.Preview{}, err
	}
	return batched(count("accounts_past_grace", accounts, true), count("expired_sessions", sessions, false)), nil
}

func previewRunArtifactPurge(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	outputs, intents, err := (&run.Service{Pool: pool}).ArtifactRetentionBacklog(ctx)
	if err != nil {
		return operations.Preview{}, err
	}
	return batched(count("run_outputs_past_retention", outputs, true), count("run_upload_intents_due", intents, true)), nil
}

func previewDatasetPurge(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	datasets, intents, err := (&testlab.Service{Pool: pool}).DatasetRetentionBacklog(ctx)
	if err != nil {
		return operations.Preview{}, err
	}
	return batched(count("datasets_past_retention", datasets, true), count("dataset_cleanups_due", intents, true)), nil
}

func previewDeletedSkillPurge(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	grace, err := MaintenanceDuration("SKILL_DELETION_GRACE")
	if err != nil {
		return operations.Preview{}, err
	}
	skills, err := (&registry.Service{Pool: pool}).SkillsPastDeletionGrace(ctx, grace)
	if err != nil {
		return operations.Preview{}, err
	}
	return batched(count("deleted_skills_past_grace", skills, true)), nil
}

func previewObjectCollection(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	objects, err := (&registry.Service{Pool: pool}).CollectableObjects(ctx)
	if err != nil {
		return operations.Preview{}, err
	}
	return batched(count("orphan_objects", objects, true)), nil
}

func previewSourceCheck(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	sources, err := (&ingest.Service{Pool: pool}).SourcesToCheck(ctx)
	if err != nil {
		return operations.Preview{}, err
	}
	return batched(count("sources_to_check", sources, false)), nil
}

func previewAuditPurge(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	retention, err := MaintenanceDuration("AUDIT_RETENTION")
	if err != nil {
		return operations.Preview{}, err
	}
	events, err := audit.CountExpired(ctx, pool, retention)
	if err != nil {
		return operations.Preview{}, err
	}
	return operations.Preview{Counts: []operations.PreviewCount{count("audit_events_past_retention", events, false)}}, nil
}

func previewFeedbackPurge(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	retention, err := MaintenanceDuration("FEEDBACK_RETENTION")
	if err != nil {
		return operations.Preview{}, err
	}
	reports, err := (&analytics.Service{Pool: pool}).CountExpiredFeedback(ctx, retention)
	if err != nil {
		return operations.Preview{}, err
	}
	return operations.Preview{Counts: []operations.PreviewCount{count("feedback_reports_past_retention", reports, false)}}, nil
}

type partitionFamily struct {
	name, retentionKey string
	plan               func(ctx context.Context, pool *pgxpool.Pool, now time.Time, retention time.Duration) (partition.Report, int64, error)
}

func previewPartitionRotation(ctx context.Context, pool *pgxpool.Pool) (operations.Preview, error) {
	now := time.Now().UTC()
	counts, err := planRotation(ctx, pool, now, partitionFamily{"trace", "TRACE_RETENTION", trace.PlanPartitions})
	if err != nil || os.Getenv("ANALYTICS_RETENTION") == "" {
		return operations.Preview{Counts: counts}, err
	}
	more, err := planRotation(ctx, pool, now, partitionFamily{"analytics", "ANALYTICS_RETENTION", analytics.PlanPartitions})
	return operations.Preview{Counts: append(counts, more...)}, err
}

func planRotation(ctx context.Context, pool *pgxpool.Pool, now time.Time, family partitionFamily) ([]operations.PreviewCount, error) {
	retention, err := MaintenanceDuration(family.retentionKey)
	if err != nil {
		return nil, err
	}
	report, rows, err := family.plan(ctx, pool, now, retention)
	if err != nil {
		return nil, err
	}
	return []operations.PreviewCount{
		count(family.name+"_partitions_created", int64(len(report.Created)), false),
		count(family.name+"_partitions_dropped", int64(len(report.Dropped)), false),
		count(family.name+"_events_removed", rows, false),
	}, nil
}
