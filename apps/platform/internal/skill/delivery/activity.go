package packaging

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type ActivityFact struct {
	ArtifactID     pgtype.UUID
	SkillVersionID pgtype.UUID
	Classification string
	Status         string
	StatusLabel    string
	FileName       string
	ActivityAt     time.Time
}

const packagingNeedsAttention = "needs_attention"

func (s *Service) ActivityFacts(ctx context.Context, ws identity.Workspace) ([]ActivityFact, error) {
	rows, err := gen.New(s.Pool).ListDownloadArtifacts(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	return packagingActivityFacts(rows), nil
}

func packagingActivityFacts(rows []gen.ListDownloadArtifactsRow) []ActivityFact {
	facts := make([]ActivityFact, 0, len(rows))
	for _, row := range rows {
		classification, label := classifyPackagingActivity(ScanStatus(row.ScanStatus))
		facts = append(facts, ActivityFact{
			ArtifactID: row.ArtifactID, SkillVersionID: row.SkillVersionID,
			Classification: classification, Status: row.ScanStatus, StatusLabel: label,
			FileName: row.FileName, ActivityAt: row.CreatedAt.Time,
		})
	}
	return facts
}

func classifyPackagingActivity(status ScanStatus) (string, string) {
	switch status {
	case ScanAvailable:
		return "recent", "套件可下載"
	case ScanQuarantined:
		return packagingNeedsAttention, "套件仍在隔離區"
	case ScanRejected:
		return packagingNeedsAttention, "套件未通過掃描"
	default:
		return packagingNeedsAttention, "不支援的套件狀態"
	}
}
