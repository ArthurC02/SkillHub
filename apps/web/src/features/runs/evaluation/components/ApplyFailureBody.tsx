import { ApiError } from "../../../../core/api/client";
import { Link } from "@tanstack/react-router";
import { isApplyResultUnknown, type RejectedSuggestion } from "../../evaluation.service";
import { BLOCKED_REASON_LABEL } from "../evaluation.model";

export function ApplyFailureBody({ error, skillId }: { error: unknown; skillId?: string }) {
  if (isApplyResultUnknown(error))
    return (
      <>
        <p>無法確認是否已建立新版本。請先核對版本清單與建議套用狀態，再決定是否重試。</p>
        {skillId && (
          <p>
            <Link to="/skills/$skillId" params={{ skillId }}>
              核對小工具版本
            </Link>
          </p>
        )}
        <p className="note">
          建議與版本的關聯可能稍後才更新；尚未回報套用版本，不代表沒有建立版本。
        </p>
      </>
    );
  if (!(error instanceof ApiError)) return <p>套用失敗，可以再按一次。</p>;
  if (error.status === 422) {
    const body = error.body as { rejected_suggestions?: RejectedSuggestion[] } | undefined;
    const rejected = body?.rejected_suggestions ?? [];
    return (
      <>
        <p>沒有一項建議可以套用，所以沒有建立新版本。</p>
        {rejected.length > 0 && (
          <ul>
            {rejected.map((r) => (
              <li key={r.suggestion_id}>
                {r.message}（{BLOCKED_REASON_LABEL[r.blocked_reason]}）
              </li>
            ))}
          </ul>
        )}
      </>
    );
  }
  return <p>套用失敗，可以再按一次。</p>;
}
