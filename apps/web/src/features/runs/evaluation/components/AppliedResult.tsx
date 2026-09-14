import { Link } from "@tanstack/react-router";
import type { VersionFromSuggestions } from "../../evaluation.service";
import { BLOCKED_REASON_LABEL } from "../evaluation.model";

export function AppliedResult({
  result,
  testCaseId,
}: {
  result: VersionFromSuggestions;
  testCaseId?: string;
}) {
  return (
    <div role="status">
      <p>
        已建立新版本 <strong>#{result.version_number}</strong>
        {result.duplicate ? "（內容與既有版本相同，沿用該版本）" : ""}
      </p>
      <p className="note">
        內容雜湊 <code>{result.content_hash}</code>；套用了 {result.applied_suggestion_ids.length}{" "}
        項建議。
      </p>
      {result.rejected_suggestions.length > 0 && (
        <>
          <p>以下建議沒有被套用：</p>
          <ul>
            {result.rejected_suggestions.map((r) => (
              <li key={r.suggestion_id}>
                {r.message}（{BLOCKED_REASON_LABEL[r.blocked_reason]}）
              </li>
            ))}
          </ul>
        </>
      )}
      {testCaseId ? (
        <p>
          <Link
            to="/lab/run"
            search={{
              skill: result.skill_id,
              version: result.version_id,
              test_case: testCaseId,
            }}
          >
            以新版本重跑這個 Test Case
          </Link>
          ：連過去的是執行前權限確認畫面，仍須在那裡確認一次才會開始 Run。
        </p>
      ) : (
        <p className="note">
          這個 Run 的 Test Case 草稿已不存在，無法從這裡以相同輸入重跑新版本；新版本本身不受影響。
        </p>
      )}
      <p>
        <Link to="/skills/$skillId" params={{ skillId: result.skill_id }}>
          前往新版本所在的 Skill
        </Link>
      </p>
    </div>
  );
}
