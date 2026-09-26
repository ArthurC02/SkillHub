import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useTrace } from "../../trace.service";
import type { TraceAdvanced } from "../../trace.service";
import { IncompleteNotice } from "./IncompleteNotice";
import { TraceEventRow } from "./TraceEventRow";

export function AdvancedMode({ runId, active }: { runId: string; active: boolean }) {
  const { events } = useSearch({ strict: false }) as { events?: string };
  const navigate = useNavigate();
  // Cursors are receive-order offsets, not page numbers, so they can't be
  // recomputed — the whole stack (not just the current one) travels in the URL.
  const pushed = events ? events.split(",").map(Number) : [];
  const cursors = [0, ...pushed];
  const pageIndex = cursors.length - 1;
  const goTo = (next: number[]) =>
    void navigate({
      to: "/runs/$runId",
      params: { runId },
      search: (prev) => ({ ...prev, events: next.length ? next.join(",") : undefined }),
    });
  const { data, isPending, isFetching, error, refetch } = useTrace(
    runId,
    "advanced",
    active,
    cursors[pageIndex],
  );
  if (isPending) return <Loading what="原始事件（一頁最多 1,000 筆，資料量大）" />;
  if (error)
    return (
      <ReadFailure error={error} what="執行紀錄">
        <p role="alert">無法讀取執行紀錄。</p>
      </ReadFailure>
    );
  const trace = data as TraceAdvanced;

  return (
    <div>
      <IncompleteNotice complete={trace.complete} />
      <p className="note" data-role="teaching">
        分頁依平台接收順序排列；每頁內依事件時間排序。這能讓執行中的 Trace 不漏掉較晚送達的事件。
      </p>
      <nav aria-label="Trace event pages">
        <button
          type="button"
          disabled={pageIndex === 0}
          aria-describedby={pageIndex === 0 ? "trace-page-first" : undefined}
          onClick={() => goTo(pushed.slice(0, -1))}
        >
          上一頁
        </button>
        <span>第 {pageIndex + 1} 頁</span>
        <button
          type="button"
          disabled={!trace.has_more}
          aria-describedby={!trace.has_more ? "trace-page-last" : undefined}
          onClick={() => goTo([...pushed, trace.next_after])}
        >
          下一頁
        </button>{" "}
        {pageIndex === 0 && (
          <span className="note" id="trace-page-first">
            這是第一頁。
          </span>
        )}
        {!trace.has_more && (
          <span className="note" id="trace-page-last">
            沒有更多事件。
          </span>
        )}{" "}
        <button type="button" disabled={isFetching} onClick={() => void refetch()}>
          {isFetching ? "重新整理中…" : "重新整理 Trace"}
        </button>
      </nav>

      <h3>事件串流</h3>
      <div className="table-scroll" tabIndex={0}>
        <table className="compare-table">
          <thead>
            <tr>
              <th scope="col">Attempt</th>
              <th scope="col">來源</th>
              <th scope="col">收到</th>
              <th scope="col">缺號</th>
              <th scope="col">遲到</th>
            </tr>
          </thead>
          <tbody>
            {trace.streams.map((stream) => (
              <tr key={`${stream.attempt}-${stream.emitted_by}`}>
                <td>{stream.attempt}</td>
                <td>{stream.emitted_by}</td>
                <td>{stream.received}</td>
                <td>
                  {stream.missing_count === 0
                    ? "無"
                    : `${stream.missing_seq?.join("、") || "未列出"}${
                        stream.missing_count > (stream.missing_seq?.length || 0)
                          ? `（共 ${stream.missing_count} 個）`
                          : ""
                      }`}
                </td>
                <td>{stream.late_events}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <h3>事件（已遮罩，依序重建）</h3>
      <p className="note" data-role="teaching">
        payload 照原樣顯示。<code>usage</code> 的 <code>cost_usd</code> 是<strong>下界</strong>
        ，不是帳單金額——權威數字是模型閘道對這個 Run 的 per-key 實付。
      </p>
      {trace.events.length === 0 ? (
        <p>尚無事件。</p>
      ) : (
        <ol className="trace-events">
          {trace.events.map((event) => (
            <TraceEventRow key={event.event_id} event={event} />
          ))}
        </ol>
      )}
    </div>
  );
}
