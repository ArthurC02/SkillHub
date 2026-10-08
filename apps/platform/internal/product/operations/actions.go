package operations

import (
	"context"
	"strings"
)

type ActionTier string

const (
	TierReadOnly    ActionTier = "read_only"
	TierReversible  ActionTier = "reversible"
	TierDestructive ActionTier = "destructive"
)

type PreviewCount struct {
	Key    string `json:"key"`
	Count  int64  `json:"count"`
	AtMost bool   `json:"at_most"`
}

type Preview struct {
	Counts     []PreviewCount `json:"counts"`
	BatchLimit int32          `json:"batch_limit,omitempty"`
}

type Action struct {
	Name    string
	Tier    ActionTier
	Preview func(ctx context.Context) (Preview, error)
}

const maintenanceJobActionPrefix = "run-"

func MaintenanceJobAction(job string) string {
	return maintenanceJobActionPrefix + job
}

func MaintenanceJobOf(action string) (string, bool) {
	job, ok := strings.CutPrefix(action, maintenanceJobActionPrefix)
	return job, ok && job != ""
}

const (
	JobPurgeAccounts      = "purge-accounts"
	JobPurgeRunArtifacts  = "purge-run-artifacts"
	JobPurgeDatasets      = "purge-datasets"
	JobPurgeDeletedSkills = "purge-deleted-skills"
	JobCollectObjects     = "collect-objects"
	JobCheckSources       = "check-sources"
	JobPurgeAudit         = "purge-audit"
	JobPurgeFeedback      = "purge-feedback"
	JobRotatePartitions   = "rotate-partitions"
)

var ProposableMaintenanceJobs = []string{
	JobPurgeAccounts, JobPurgeRunArtifacts, JobPurgeDatasets, JobPurgeDeletedSkills, JobCollectObjects,
	JobCheckSources, JobPurgeAudit, JobPurgeFeedback, JobRotatePartitions,
}
