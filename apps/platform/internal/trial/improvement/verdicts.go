package eval

// The batch verdict read a run history needs (04 丙-32).
//
// It is here rather than in run for the reason ADR-033 exists: `evaluations` is
// this context's table, and a JOIN to it from inside a run-owned query would
// pass CI precisely because the ownership checker sees which context calls which
// query and not which tables a query touches. That blind spot is what ADR-033
// was written to close, so using it would be the one move the rule forbids in
// spirit while allowing in letter.
//
// It crosses to run as bytes, like catalog's SkillRisks: run reads no field of
// it and only forwards it, so a mirror struct on that side would be a second
// place the wording can drift (02:NFR-007 第 3 條).

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// labelled mirrors the contract's Labelled: an enum value with the copy shown
// for it, so no screen keeps its own enum→中文 map (04 丙-29 裁定②).
type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// notEvaluated is what a run with no evaluation row renders as. It is a value,
// not an omission: 設計系統 §2.9 rates a blank verdict the worst available
// rendering, because an empty column beside 「執行完成」 reads as a pass.
var notEvaluated = labelled{
	Value: "not_evaluated",
	Label: "未評估",
	Note:  "這個 Run 還沒有任務判定。執行狀態說的是工作負載跑完了沒有,不是任務有沒有做到(ADR-025)。",
}

// verdictOf folds the evaluation's own status together with its verdict, which
// is the whole reason this read hands out finished copy instead of two enums:
// `overall` alone lies. An evaluation is created carrying `undetermined` and
// keeps it until the judge finishes, so a pending or failed evaluation reads as
// 「無法判斷」 — and that is a verdict the judge *reached*, not a statement that no
// judge ran. Only this context can tell those apart.
func verdictOf(status, overall string) labelled {
	switch status {
	case "pending":
		return labelled{
			Value: "evaluating",
			Label: "評估中",
			Note:  "判定還在產生。這一列的判定會變,執行狀態不會。",
		}
	case "failed":
		return labelled{
			Value: "evaluation_failed",
			Label: "評估失敗",
			Note:  "判定沒有產生出來。**這不代表任務失敗**——沒有人判過,不是判過不合格。",
		}
	}
	switch overall {
	case "met":
		return labelled{Value: "met", Label: "符合",
			Note: "依這個 Run 當時的驗收條件判定為符合。"}
	case "partially_met":
		return labelled{Value: "partially_met", Label: "部分符合",
			Note: "部分驗收條件通過,部分沒有;逐條結果在這個 Run 的評估頁面。"}
	case "not_met":
		return labelled{Value: "not_met", Label: "未符合",
			Note: "依這個 Run 當時的驗收條件判定為未符合。"}
	default:
		return labelled{Value: "undetermined", Label: "無法判斷",
			Note: "判定跑完了,而證據不足以下結論——這是判定的結果,不是判定沒跑。"}
	}
}

// RunVerdicts answers the standing verdict for a page of runs, keyed by run id
// and already serialised. Every requested id gets an entry; a run with no
// evaluation gets 未評估.
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
		blob, err := json.Marshal(verdictOf(string(row.Status), string(row.Overall)))
		if err != nil {
			return nil, err
		}
		out[pgconv.UUIDString(row.RunID)] = blob
	}
	return out, nil
}
