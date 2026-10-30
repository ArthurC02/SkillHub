import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { bytes } from "../../../shared/format";
import { ApiError, SessionNotReadyError } from "../../../core/api/client";
import {
  RunNotStartedError,
  isWriteResultUnknown,
  refusalReason,
  type PreflightResponse,
  type PreflightSummary,
} from "../lab.service";

export function ceiling(n: number | undefined): string {
  return limit(n, bytes);
}

export function limit(n: number | undefined, format: (n: number) => string): string {
  if (typeof n !== "number" || !Number.isFinite(n)) return "未測量";
  if (n <= 0) return `伺服器回報 ${n}——這不是有效的上限,請勿據此判斷這次試跑的可用資源`;
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
    "這個小工具的來源授權還在審查中,審查期間不能試跑。授權審查完成後這一頁就會讓你開始。",
  capability_mismatch:
    "這個部署現在沒有能跑這次試跑的環境——沒有接上模型閘道,或沒有一個執行沙箱符合這次試跑的要求。請聯絡管理者。",
  scan_blocked:
    "這個版本的套件在靜態掃描被擋下,不能試跑。請看這一頁的 Script 掃描結果,修正後發一個新版本。",
  scan_unavailable:
    "這個版本的套件讀不到,掃描沒有完成。沒有掃過的套件不會被當成乾淨的,所以不能試跑;請重新上傳這個版本。",
  content_not_curated:
    "這個部署只跑目錄裡的小工具。這一版不在公開目錄、也不是被策展的那一版,所以按了也不會開始——" +
    "要跑自己的小工具,請用有真正沙箱的部署。",
  credit_balance:
    "點數不足,不能開始這次試跑。請聯絡管理者為這個帳號加點;加點後重新整理這一頁就能開始。",
  platform_model_budget:
    "平台的模型總額度今天剩下的不夠這次試跑的上限,所以現在不能開始,也不會扣點。額度在台北時間每天上午 8 點恢復,之後重新整理這一頁就能開始。",
  workspace_concurrency:
    "這個工作區還有一次試跑沒有結束,一次只能跑一個,所以現在不能開始。等它結束或到試跑清單取消它,再重新整理這一頁。",
  quota_daily:
    "今天的試跑次數已經用完,所以現在不能開始,也不會扣點。從今天最早那一次算起 24 小時後恢復。",
  quota_window:
    "這段期間的試跑次數已經用完,所以現在不能開始,也不會扣點。上方的試跑次數寫著下一次什麼時候恢復。",
  quota_unavailable:
    "現在算不出這個工作區還剩幾次試跑,沒算清楚之前不會開始,也不會扣點。稍後重新整理這一頁再試。",
};

export const NOT_INVITED_SENTENCE =
  "這個帳號還沒有封測邀請，所以不能試跑。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";

export function startFailureSentence(err: unknown): string {
  if (!err) return "";
  if (err instanceof SessionNotReadyError) return err.message;
  if (err instanceof RunNotStartedError)
    return "權限確認未完成，尚未送出開始試跑要求。請重試權限確認。";
  if (refusalReason(err) === "dispatch_halted")
    return "平台暫停派送新的試跑，這次試跑沒有開始、也沒有用到額度。稍後再按一次就好。";
  if (isWriteResultUnknown(err))
    return "無法確認試跑是否已開始。請先查看試跑紀錄，再決定是否重新送出；再次送出可能建立另一筆試跑並使用額度。";
  if (err instanceof ApiError && err.status === 422) {
    return err.message
      ? `這次試跑沒有開始：${err.message}`
      : "這次試跑沒有開始。下方摘要已重新讀取,請確認之後再試。";
  }
  if (err instanceof ApiError && err.status === 403) {
    return "這個帳號還沒有封測邀請，所以試跑沒有開始。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
  }
  if (err instanceof ApiError && err.status === 404) {
    return "找不到這個小工具版本或測試題，可能已被刪除。";
  }
  if (unauthenticated(err)) return "";
  return "無法開始試跑，可以再按一次。";
}
