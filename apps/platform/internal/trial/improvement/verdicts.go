package eval

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

var notEvaluated = labelled{
	Value: "not_evaluated",
	Label: "未評估",
	Note:  "這個 Run 還沒有任務判定。執行狀態說的是工作負載跑完了沒有,不是任務有沒有做到(ADR-025)。",
}

func verdictOf(status Status, overall Overall) labelled {
	switch status {
	case StatusPending:
		return labelled{
			Value: "evaluating",
			Label: "評估中",
			Note:  "判定還在產生。這一列的判定會變,執行狀態不會。",
		}
	case StatusFailed:
		return labelled{
			Value: "evaluation_failed",
			Label: "評估失敗",
			Note:  "判定沒有產生出來。**這不代表任務失敗**——沒有人判過,不是判過不合格。",
		}
	}
	switch overall {
	case OverallMet:
		return labelled{Value: string(OverallMet), Label: "符合",
			Note: "依這個 Run 當時的驗收條件判定為符合。"}
	case OverallPartiallyMet:
		return labelled{Value: string(OverallPartiallyMet), Label: "部分符合",
			Note: "部分驗收條件通過,部分沒有;逐條結果在這個 Run 的評估頁面。"}
	case OverallNotMet:
		return labelled{Value: string(OverallNotMet), Label: "未符合",
			Note: "依這個 Run 當時的驗收條件判定為未符合。"}
	default:
		return labelled{Value: string(OverallUndetermined), Label: "無法判斷",
			Note: "判定跑完了,而證據不足以下結論——這是判定的結果,不是判定沒跑。"}
	}
}

func (s *Service) RunVerdicts(
	ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID,
) (map[string]json.RawMessage, error) {
	blank, err := json.Marshal(notEvaluated)
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(runIDs))
	for _, id := range runIDs {
		out[pgconv.UUIDString(id)] = blank
	}
	if len(runIDs) == 0 {
		return out, nil
	}

	rows, err := gen.New(s.Pool).ListCurrentEvaluations(ctx, gen.ListCurrentEvaluationsParams{
		WorkspaceID: workspaceID, RunIds: runIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		blob, err := json.Marshal(verdictOf(Status(row.Status), Overall(row.Overall)))
		if err != nil {
			return nil, err
		}
		out[pgconv.UUIDString(row.RunID)] = blob
	}
	return out, nil
}
