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
	RunID              pgtype.UUID
	SkillVersionID     pgtype.UUID
	TestCaseSnapshotID pgtype.UUID
	Classification     string
	Status             string
	StatusLabel        string
	ActivityAt         time.Time
}

type ActivityLink struct {
	SkillID    pgtype.UUID
	SkillName  string
	TestCaseID pgtype.UUID
}

const (
	activityInProgress     = "in_progress"
	activityNeedsAttention = "needs_attention"
)

func (s *Service) ActivityFacts(ctx context.Context, workspaceID pgtype.UUID) ([]ActivityFact, error) {
	rows, err := s.queries().ListRunActivityFacts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	facts := make([]ActivityFact, len(rows))
	for i, row := range rows {
		classification, label := classifyRunActivity(row.Status)
		facts[i] = ActivityFact{
			RunID: row.ID, SkillVersionID: row.SkillVersionID, TestCaseSnapshotID: row.TestCaseSnapshotID,
			Classification: classification, Status: string(row.Status), StatusLabel: label,
			ActivityAt: row.ActivityUpdatedAt.Time,
		}
	}
	return facts, nil
}

func (s *Service) ActivityLinks(
	ctx context.Context, workspaceID pgtype.UUID, facts []ActivityFact,
) (map[pgtype.UUID]ActivityLink, error) {
	if err := s.requireRunLinks(); err != nil {
		return nil, err
	}
	versionIDs := make([]pgtype.UUID, len(facts))
	snapshotIDs := make([]pgtype.UUID, len(facts))
	for i, fact := range facts {
		versionIDs[i] = fact.SkillVersionID
		snapshotIDs[i] = fact.TestCaseSnapshotID
	}
	versions, testCases, err := s.runLinks(ctx, workspaceID, versionIDs, snapshotIDs)
	if err != nil {
		return nil, err
	}
	links := make(map[pgtype.UUID]ActivityLink, len(facts))
	for _, fact := range facts {
		version, versionFound := versions[fact.SkillVersionID]
		testCaseID, caseFound := testCases[fact.TestCaseSnapshotID]
		if !versionFound || !caseFound {
			return nil, fmt.Errorf("%w: run %s", errRunLinkMissing, pgconv.UUIDString(fact.RunID))
		}
		links[fact.RunID] = ActivityLink{SkillID: version.SkillID, SkillName: version.SkillName, TestCaseID: testCaseID}
	}
	return links, nil
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
