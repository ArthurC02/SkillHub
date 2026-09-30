package creation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type ActivityFact struct {
	SessionID      pgtype.UUID
	Classification string
	Status         string
	StatusLabel    string
	Summary        string
	ActivityAt     time.Time
}

const creationNeedsAttention = "needs_attention"

func (s *Service) ActivityFacts(ctx context.Context, ws identity.Workspace) ([]ActivityFact, error) {
	rows, err := gen.New(s.Pool).ListCreationActivityFacts(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	facts := make([]ActivityFact, len(rows))
	for i, row := range rows {
		var value envelope
		if err := json.Unmarshal(row.Snapshot, &value); err != nil {
			return nil, err
		}
		classification, label := classifyCreationActivity(State(row.State))
		facts[i] = ActivityFact{
			SessionID: row.ID, Classification: classification, Status: row.State,
			StatusLabel: label, Summary: creationActivitySummary(value.Snapshot.PendingAction),
			ActivityAt: row.UpdatedAt.Time,
		}
	}
	return facts, nil
}

func classifyCreationActivity(state State) (string, string) {
	switch state {
	case StateQueued:
		return "in_progress", "等待創作"
	case StateWorking:
		return "in_progress", "創作中"
	case StateWaitingInput:
		return creationNeedsAttention, "等待你的資訊"
	case StateWaitingConfirmation:
		return creationNeedsAttention, "等待你的確認"
	case StateDraftReady:
		return creationNeedsAttention, "草稿可檢視"
	case StateCandidateReady:
		return creationNeedsAttention, "候選 Skill 可儲存"
	case StateFailed:
		return creationNeedsAttention, "創作失敗"
	case StateNeedsReupload:
		return creationNeedsAttention, "需要重新上傳"
	case StateSaved:
		return "recent", "已儲存"
	case StateCancelled:
		return "recent", "已取消"
	default:
		return creationNeedsAttention, "不支援的創作狀態"
	}
}

func creationActivitySummary(action PendingAction) string {
	switch action {
	case PendingBriefConfirmation:
		return "確認 Skill 需求"
	case PendingDiagramDescription:
		return "補充圖像說明"
	case PendingDiagramAnswers:
		return "回答圖像問題"
	case PendingDiagramInterpretation:
		return "確認圖像理解"
	case PendingReferenceChoice:
		return "確認參考 Skill"
	case PendingDuplicateAcknowledgement:
		return "確認重複 Skill"
	case PendingFetchPermission:
		return "確認外部資料存取"
	default:
		return "Skill 創作進度"
	}
}
