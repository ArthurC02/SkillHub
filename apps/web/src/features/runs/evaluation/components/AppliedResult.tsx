import { Link } from "@tanstack/react-router";
import type { VersionFromSuggestions } from "../../evaluation.service";
import { BLOCKED_REASON_LABEL } from "../evaluation.model";

export function AppliedResult({
  result,
  testCaseId,
  sourceConfirmed,
  runId,
}: {
  result: VersionFromSuggestions;
  testCaseId?: string;
  sourceConfirmed: boolean;
  runId: string;
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
      {!sourceConfirmed ? (
        <p className="note">
          試跑來源尚未確認，暫時無法提供相同測試題的重跑入口；已建立的新版本不受影響。
        </p>
      ) : testCaseId ? (
        <p>
          <Link
            to="/skills/$skillId/test-cases/$testCaseId/runs/new"
            params={{ skillId: result.skill_id, testCaseId }}
            search={{ version: result.version_id, against: runId }}
          >
            以新版本重跑這個測試題
          </Link>
          ：連過去的是執行前權限確認畫面，仍須在那裡確認一次才會開始試跑；開始後可以和這次試跑逐條比較，看建議有沒有真的改善。
        </p>
      ) : (
        <p className="note">
          這次試跑的測試題草稿已不存在，無法從這裡以相同輸入重跑新版本；新版本本身不受影響。
        </p>
      )}
      <p>
        <Link
          to="/skills/$skillId/versions/$versionId"
          params={{ skillId: result.skill_id, versionId: result.version_id }}
        >
          開啟剛建立的版本
        </Link>
      </p>
    </div>
  );
}
