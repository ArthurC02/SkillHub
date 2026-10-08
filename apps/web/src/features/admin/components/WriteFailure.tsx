import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ApiError } from "../../../core/api/client";
import { serverSentenceOr } from "../../../shared/format";
import { isUncertainWriteFailure } from "../admin.service";

export function WriteFailure({ error }: { error: unknown }) {
  return (
    <ReadFailure error={error} what="這個動作的結果">
      <p role="alert">
        {isUncertainWriteFailure(error)
          ? "這個動作的結果尚未確認。請核對最新狀態；確認前不要直接重送或執行相反動作。"
          : serverSentenceOr(
              error instanceof ApiError ? error.message : undefined,
              error instanceof ApiError && error.status === 409
                ? "資料已變更，請重新整理頁面確認最新狀態後再試。"
                : "這個動作沒有完成，請稍後再試；若持續失敗，請用頁尾回報問題。",
            )}
      </p>
    </ReadFailure>
  );
}
