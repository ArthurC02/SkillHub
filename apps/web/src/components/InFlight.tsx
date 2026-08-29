import { TERMINAL_RUN_STATUSES } from "../api/trace";
import { Timestamp } from "./Timestamp";
import type { TraceSummary } from "../api/trace";
import { runStatusLabel } from "../pages/RunEvaluation";

/**
 * 設計 §2.12: 進行中是第三軸，不是判定的一個值。
 *
 * Everything on this page before it was written answered "what happened". This
 * answers "what is happening", and 設計 §2.12 says that needs three sentences
 * and two facts:
 *
 *  - **哪一步** — the run's own status, in the same words every other surface
 *    uses for it.
 *  - **會不會自己結束** — a run reaches a terminal state on its own; nobody has
 *    to come back and press anything.
 *  - **能不能離開** — yes, and it is not a guess: `run_execute` is a River job
 *    in Postgres consumed by cmd/worker, and trace events are posted to the API
 *    by the sandbox. The browser is in neither path, so closing this page cannot
 *    stop the run or lose its record. §2.2 wants the line that enforces a claim;
 *    that is the line.
 *  - **一個會變的量** — events so far, which moves while the poll runs.
 *  - **多久沒動了** — the last event's time. A counter alone cannot tell working
 *    from wedged, because a wedged run's counter just stops, and stopping looks
 *    exactly like finishing.
 *
 * Not rendered at all once the run is terminal: a finished run has a verdict and
 * an execution state, and a banner about waiting would be a third answer to a
 * question nobody is still asking.
 */
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
        可以關掉這一頁。執行在平台的 worker 上進行，紀錄由沙箱直接回報，兩者都不經過瀏覽器；
        回到這個網址就會看到當下的進度。要提前停止，用上面的「取消這個 Run」。
      </p>
      <p className="note">
        目前已記錄 {moved} 件事（工具呼叫、Skill 載入與錯誤合計）
        {/*
          §2.9: 沒有事件不是 0 件事的另一種寫法。一個還在配置環境的 Run 本來就
          還沒產生任何事件，那是狀態不是缺值——而印「0 秒前」會是三者裡最糟的
          一個，它把「還沒開始」講成「剛剛才動過」。
        */}
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
