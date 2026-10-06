import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ApiError } from "../../../core/api/client";
import { serverSentenceOr } from "../../../shared/format";

export function WriteFailure({ error }: { error: unknown }) {
  return (
    <ReadFailure error={error} what="這個動作的結果">
      <p role="alert">
        {serverSentenceOr(
          error instanceof ApiError ? error.message : undefined,
          "這個動作沒有完成，請稍後再試；若持續失敗，請用頁尾回報問題。",
        )}
      </p>
    </ReadFailure>
  );
}
