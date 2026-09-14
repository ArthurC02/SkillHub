import { unauthenticated } from "../../../shared/ui/LoginRequired";
import { bytes } from "../../../shared/format";
import { ApiError } from "../../../core/api/client";
import type { PreflightSummary } from "../lab.service";

export function ceiling(n: number | undefined): string {
  return limit(n, bytes);
}

export function limit(n: number | undefined, format: (n: number) => string): string {
  if (typeof n !== "number" || !Number.isFinite(n)) return "未測量";
  if (n <= 0) return `伺服器回報 ${n}——這不是有效的上限,請勿據此判斷這次 Run 的可用資源`;
  return format(n);
}

export const count = (n: number) => `${n}`;

export const seconds = (n: number) => `${n} 秒`;

export const tokens = (n: number) => `${n}`;

export const SCRIPT_LABEL: Record<PreflightSummary["scripts"]["status"], string> = {
  none: "無(靜態掃描未發現 Script 或內嵌程式碼)",
  present: "有",
  unavailable: "未知(套件無法讀取,未完成掃描)",
};

export function startFailureSentence(err: unknown): string {
  if (!err) return "";
  if (err instanceof ApiError && err.status === 422) {
    return err.message
      ? `這次 Run 沒有開始：${err.message}`
      : "這次 Run 沒有開始。下方摘要已重新讀取,請確認之後再試。";
  }
  if (err instanceof ApiError && err.status === 403) {
    return "這個帳號還沒有封測邀請，所以 Run 沒有開始。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
  }
  if (err instanceof ApiError && err.status === 503) {
    return "執行環境暫時無法使用，這次 Run 沒有開始。可以直接再按一次，不需要重新確認權限。";
  }
  if (err instanceof ApiError && err.status === 404) {
    return "找不到這個 Skill 版本或 Test Case，可能已被刪除。";
  }
  if (unauthenticated(err)) return "";
  return "無法開始 Run，可以再按一次。";
}
