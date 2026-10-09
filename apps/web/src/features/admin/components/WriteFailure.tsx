import { ApiError } from "../../../core/api/client";
import { ReadFailure } from "../../../shared/ui/LoginRequired";

function failureMessage(error: unknown): string {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
    if (/^[㐀-鿿]/u.test(error.message.trim())) return `操作未被接受：${error.message}`;
    if (error.status === 403) return "操作未被接受；目前沒有執行權限，請確認登入身分。";
    if (error.status === 404 || error.status === 409)
      return "操作未被接受；目前狀態可能已變更，請重新整理後再判斷。";
    if (error.status === 429) return "操作未被接受；送出太頻繁，請稍後再試。";
    return "操作未被接受；請核對輸入與目前狀態後再試。";
  }
  return "無法確認操作是否完成；請先重新整理目前狀態，再決定是否重試。";
}

export function WriteFailure({ error }: { error: unknown }) {
  return (
    <ReadFailure error={error} what="這個動作的結果">
      <p role="alert">{failureMessage(error)}</p>
    </ReadFailure>
  );
}
