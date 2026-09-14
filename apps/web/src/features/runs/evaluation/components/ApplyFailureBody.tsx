import { ApiError } from "../../../../core/api/client";
import type { RejectedSuggestion } from "../../evaluation.service";
import { BLOCKED_REASON_LABEL } from "../evaluation.model";

export function ApplyFailureBody({ error }: { error: unknown }) {
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
  if (error.status === 500) {
    return (
      <p>套用失敗。如果版本清單裡已經出現新版本，它可以用，只是「由哪些建議產生」的紀錄沒寫成。</p>
    );
  }
  return <p>套用失敗，可以再按一次。</p>;
}
