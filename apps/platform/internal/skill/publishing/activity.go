package publishing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type ActivityFact struct {
	PublicationID   pgtype.UUID
	SkillID         pgtype.UUID
	LatestVersionID pgtype.UUID
	Publisher       string
	Name            string
	Classification  string
	Status          string
	StatusLabel     string
	ActivityAt      time.Time
}

func (s *Service) ActivityFacts(ctx context.Context, ws identity.Workspace) ([]ActivityFact, error) {
	rows, err := gen.New(s.Pool).ListWorkspaceSkillPublications(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	facts := make([]ActivityFact, len(rows))
	for i, row := range rows {
		classification, label := classifyPublishingActivity(Status(row.Status), row.LatestVersionID)
		activityAt := publicationActivityTime(row.StatusChangedAt, row.LatestReleasedAt)
		facts[i] = ActivityFact{
			PublicationID: row.PublicationID, SkillID: row.SkillID,
			LatestVersionID: row.LatestVersionID, Publisher: row.PublisherName, Name: row.Name,
			Classification: classification, Status: row.Status, StatusLabel: label,
			ActivityAt: activityAt,
		}
	}
	return facts, nil
}

func publicationActivityTime(statusChangedAt, releasedAt pgtype.Timestamptz) time.Time {
	if releasedAt.Valid && releasedAt.Time.After(statusChangedAt.Time) {
		return releasedAt.Time
	}
	return statusChangedAt.Time
}

func classifyPublishingActivity(status Status, latestVersionID pgtype.UUID) (string, string) {
	switch status {
	case StatusPublished:
		if latestVersionID.Valid {
			return "recent", "已發佈"
		}
		return "needs_attention", "尚未建立 Release"
	case StatusDelisted:
		return "recent", "已下架"
	default:
		return "needs_attention", "不支援的發佈狀態"
	}
}
