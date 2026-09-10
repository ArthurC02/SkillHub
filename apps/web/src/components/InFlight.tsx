import { TERMINAL_RUN_STATUSES } from "../api/trace";
import { Timestamp } from "./Timestamp";
import type { TraceSummary } from "../api/trace";
import { runStatusLabel } from "../pages/RunEvaluation";
import { Tip } from "./Tip";

export function InFlight({ summary }: { summary: TraceSummary }) {
  if (TERMINAL_RUN_STATUSES.has(summary.status)) return null;

  const moved = summary.tool_calls.total + summary.skills_total + summary.errors_total;

  return (
    <div className="notice" role="status">
      <p>
        <strong>進行中：{runStatusLabel(summary.status)}</strong>
        ——這個 Run 會自己跑到結束，不需要你回來按任何東西。
      </p>
      <p className="note">
        可以關掉這一頁（平台在跑，不是你的瀏覽器）；回到這個網址就會看到當下的進度。
        <Tip anchor="為什麼可以關掉這一頁">
          這個 Run 是資料庫裡佇列的一項工作，由平台的 Worker 執行；沙箱把過程中的事件直接送回平台的
          API。瀏覽器不在這兩條路徑上，分頁開著或關掉都不影響 Run 本身或它的紀錄。
        </Tip>
      </p>
      <p className="note">
        目前已記錄 {moved} 件事（工具呼叫、Skill 載入與錯誤合計）
        {summary.last_event_at ? (
          <>
            ｜最後一件於 <Timestamp at={summary.last_event_at} relative />
          </>
        ) : (
          "｜還沒有任何事件送達（環境仍在準備，不是紀錄遺失）"
        )}
      </p>
    </div>
  );
}
