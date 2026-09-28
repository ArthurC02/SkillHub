import { Link } from "@tanstack/react-router";
import type { ComparisonSide } from "../../evaluation.service";

export function RerunCell({ side }: { side: ComparisonSide }) {
  if (!side.inputs_available) {
    return <>已刪除或已過期，無法以相同輸入重跑；比較內容本身不受影響。</>;
  }
  if (!side.test_case_id) {
    return <>仍在。可用同一個 Test Case 重新試跑，仍須通過執行前權限確認。</>;
  }
  return (
    <>
      仍在。{" "}
      <Link
        to="/skills/$skillId/test-cases/$testCaseId/runs/new"
        params={{ skillId: side.skill_id, testCaseId: side.test_case_id }}
        search={{ version: side.skill_version_id }}
      >
        以相同的 Test Case 與版本重新試跑
      </Link>
      （會先經過權限確認）
    </>
  );
}
