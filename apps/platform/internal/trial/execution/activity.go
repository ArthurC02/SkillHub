package run

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type ActivityFact struct {
	RunID          pgtype.UUID
	SkillID        pgtype.UUID
	SkillName      string
	SkillVersionID pgtype.UUID
	TestCaseID     pgtype.UUID
	Classification string
	Status         string
	StatusLabel    string
	ActivityAt     time.Time
}

const (
	activityInProgress     = "in_progress"
	activityNeedsAttention = "needs_attention"
)

func (s *Service) ActivityFacts(ctx context.Context, workspaceID pgtype.UUID) ([]ActivityFact, error) {
	if err := s.requireRunLinks(); err != nil {
		return nil, err
	}
	rows, err := s.queries().ListRunActivityFacts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	versionIDs := make([]pgtype.UUID, len(rows))
	snapshotIDs := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		versionIDs[i] = row.SkillVersionID
		snapshotIDs[i] = row.TestCaseSnapshotID
	}
	versions, testCases, err := s.runLinks(ctx, workspaceID, versionIDs, snapshotIDs)
	if err != nil {
		return nil, err
	}
	facts := make([]ActivityFact, len(rows))
	for i, row := range rows {
		version, versionFound := versions[row.SkillVersionID]
		testCaseID, caseFound := testCases[row.TestCaseSnapshotID]
		if !versionFound || !caseFound {
			return nil, fmt.Errorf("%w: run %s", errRunLinkMissing, pgconv.UUIDString(row.ID))
		}
		classification, label := classifyRunActivity(row.Status)
		facts[i] = ActivityFact{
			RunID: row.ID, SkillID: version.SkillID, SkillName: version.SkillName,
			SkillVersionID: row.SkillVersionID, TestCaseID: testCaseID,
			Classification: classification, Status: string(row.Status), StatusLabel: label,
			ActivityAt: row.ActivityUpdatedAt.Time,
		}
	}
	return facts, nil
}

func classifyRunActivity(status gen.RunStatus) (string, string) {
	switch status {
	case gen.RunStatusQueued:
		return activityInProgress, "等待執行"
	case gen.RunStatusProvisioning:
		return activityInProgress, "準備執行環境"
	case gen.RunStatusPreparing:
		return activityInProgress, "準備試跑"
	case gen.RunStatusRunning:
		return activityInProgress, "試跑中"
	case gen.RunStatusEvaluating:
		return activityInProgress, "評估中"
	case gen.RunStatusSucceeded:
		return "recent", "試跑完成"
	case gen.RunStatusCancelled:
		return "recent", "已取消"
	case gen.RunStatusFailed:
		return activityNeedsAttention, "試跑失敗"
	case gen.RunStatusTimedOut:
		return activityNeedsAttention, "試跑逾時"
	default:
		return activityNeedsAttention, "不支援的試跑狀態"
	}
}
