import { unauthenticated } from "../../../shared/ui/LoginRequired";
import { bytes } from "../../../shared/format";
import { ApiError } from "../../../core/api/client";
import type { PreflightResponse, PreflightSummary } from "../lab.service";

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

export const BLOCKED_SENTENCE: Record<NonNullable<PreflightResponse["blocked"]>, string> = {
  access_restricted:
    "這個 Skill 的來源授權還在審查中,審查期間不能試跑。授權審查完成後這一頁就會讓你開始。",
  capability_mismatch:
    "這個部署現在沒有能跑這次試跑的環境——沒有接上模型閘道,或沒有一個執行沙箱符合這次試跑的要求。請聯絡管理者。",
  scan_blocked:
    "這個版本的套件在靜態掃描被擋下,不能試跑。請看這一頁的 Script 掃描結果,修正後發一個新版本。",
  scan_unavailable:
    "這個版本的套件讀不到,掃描沒有完成。沒有掃過的套件不會被當成乾淨的,所以不能試跑;請重新上傳這個版本。",
  content_not_curated:
    "這個部署只跑目錄裡的 Skill。這一版不在公開目錄、也不是被策展的那一版,所以按了也不會開始——" +
    "要跑自己的 Skill,請用有真正沙箱的部署。",
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
