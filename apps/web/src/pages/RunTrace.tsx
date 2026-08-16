import { useState } from "react";
import { Link, useParams } from "@tanstack/react-router";
import { useTrace } from "../api/trace";
import type { TraceAdvanced, TraceEvent, TraceSummary } from "../api/trace";
import { EvaluationPanel, RUN_STATUS_LABEL } from "./RunEvaluation";

/**
 * TRACE-006 and TRACE-007: the two modes of the run trace.
 *
 * Three rules shape everything below and none of them is cosmetic:
 *
 * 1. **Incompleteness is stated, never smoothed over.** A hole in a producer's
 *    sequence means an event was lost, and ADR-009 forbids presenting the
 *    remainder as the whole story. The banner is the first thing rendered.
 * 2. **Payloads render as inert text.** Everything under `emitted_by: sandbox`
 *    is untrusted output that has crossed the trust boundary (ADR-001), so it
 *    goes through JSON.stringify into a <pre>: no HTML, no ANSI, no
 *    interpretation. React escapes by default and nothing here opts out.
 * 3. **An unreported number is not zero.** A null cost means the gateway did not
 *    report one; showing 0 would tell the user their run was free
 *    (contracts/events/README.md §5).
 */

export function RunTrace() {
  const { runId } = useParams({ from: "/runs/$runId" });
  const [mode, setMode] = useState<"general" | "advanced">("general");
  // Same query key as the general mode below, so this is one request, not two.
  // The runs table is the only authority on run state (iron rule 5).
  const general = useTrace(runId, "general");

  return (
    <section className="run-trace">
      <h1>Run 詳情</h1>

      {/* EVAL-001 / design §4.3: the first thing on the page is the task
          judgement, not the run's terminal state. */}
      <EvaluationPanel runId={runId} runStatus={general.data?.status} />

      <h2>執行紀錄</h2>
      <p className="note">
        <Link to="/runs/$runId/compare" params={{ runId }} search={{ against: "" }}>
          與另一個 Run 比較
        </Link>
      </p>
      <div role="group" aria-label="檢視模式">
        <button type="button" aria-pressed={mode === "general"} onClick={() => setMode("general")}>
          一般模式
        </button>
        <button
          type="button"
          aria-pressed={mode === "advanced"}
          onClick={() => setMode("advanced")}
        >
          進階模式
        </button>
      </div>
      {mode === "general" ? <GeneralMode runId={runId} /> : <AdvancedMode runId={runId} />}
    </section>
  );
}

/** Shown whenever a producer's sequence has a hole (TRACE-001, TRACE-008). */
function IncompleteNotice({ complete }: { complete: boolean }) {
  if (complete) return null;
  return (
    <p role="status" className="trace-incomplete">
      部分事件未送達，以下內容可能不完整。
    </p>
  );
}

function GeneralMode({ runId }: { runId: string }) {
  const { data, isPending, error } = useTrace(runId, "general");
  if (isPending) return <p>載入中…</p>;
  if (error) return <p role="alert">無法讀取執行紀錄。</p>;
  const trace = data as TraceSummary;

  return (
    <div>
      <IncompleteNotice complete={trace.complete} />
      {/* Status comes from the runs table, not from replayed events (iron rule 5),
          and is worded as execution: `succeeded` says the workload finished, not
          that the task was done (ADR-025). The task verdict is above. */}
      <p>
        執行狀態：<strong>{RUN_STATUS_LABEL[trace.status] ?? trace.status}</strong>（
        <code>{trace.status}</code>）{trace.status_reason ? `（${trace.status_reason}）` : null}
      </p>

      <h2>進度</h2>
      <ol>
        {trace.steps.map((step, i) => (
          <li key={`${i}-${step}`}>{step}</li>
        ))}
      </ol>

      <h2>使用的 Skill</h2>
      {trace.skills.length === 0 ? (
        <p>沒有記錄到 Skill 啟用事件。</p>
      ) : (
        <ul>
          {trace.skills.map((skill, i) => (
            <li key={`${skill.name}-${i}`}>
              {skill.name}（{skill.decision === "activated" ? "已啟用" : "未啟用"}）
              {skill.reason ? `：${skill.reason}` : null}
            </li>
          ))}
        </ul>
      )}

      <h2>工作內容</h2>
      <ul>
        <li>讀取套件資源 {trace.resources_read} 次</li>
        <li>
          工具呼叫 {trace.tool_calls.total} 次（成功 {trace.tool_calls.succeeded}、失敗{" "}
          {trace.tool_calls.failed}），共 {trace.tool_calls.total_duration_ms} 毫秒
          {trace.tool_calls.slowest_tool
            ? `，最久的是 ${trace.tool_calls.slowest_tool}（${trace.tool_calls.slowest_duration_ms} 毫秒）`
            : null}
        </li>
      </ul>

      {trace.errors.length > 0 ? (
        <>
          <h2>錯誤</h2>
          <ul>
            {trace.errors.map((err, i) => (
              <li key={`${err.code}-${i}`}>
                [{err.category}/{err.code}] {err.message}
              </li>
            ))}
          </ul>
        </>
      ) : null}

      <h2>最終輸出</h2>
      {/* Model output over user data: inert text, never rendered as markup. */}
      {trace.final_output ? <pre>{trace.final_output}</pre> : <p>尚無最終輸出。</p>}

      <h2>用量</h2>
      {trace.usage ? (
        <ul>
          <li>模型：{trace.usage.model || "未知"}</li>
          <li>
            Token：輸入 {trace.usage.input_tokens}、輸出 {trace.usage.output_tokens}
          </li>
          <li>
            成本：
            {/* null is "the gateway did not report it", which is not zero. */}
            {trace.usage.cost_usd === null
              ? "未回報"
              : `US$${trace.usage.cost_usd.toFixed(4)}${
                  trace.usage.cost_source === "estimated" ? "（估算值）" : ""
                }`}
          </li>
          {/* 丙-3: the sum over usage events is structurally a floor — a
              response still in flight when the stream ends is not counted. The
              settling figure is the gateway's per-key spend (ADR-017). */}
          <li className="note">
            這是<strong>下界</strong>，不是總額：合計自 Trace 的用量事件， 權威來源是模型閘道對這個
            Run 的 per-key 實付。
          </li>
        </ul>
      ) : (
        <p>沒有記錄到用量事件。</p>
      )}
    </div>
  );
}

function AdvancedMode({ runId }: { runId: string }) {
  const { data, isPending, error } = useTrace(runId, "advanced");
  if (isPending) return <p>載入中…</p>;
  if (error) return <p role="alert">無法讀取執行紀錄。</p>;
  const trace = data as TraceAdvanced;

  return (
    <div>
      <IncompleteNotice complete={trace.complete} />

      <h2>事件串流</h2>
      <table>
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
              {/* An empty missing list is "nothing lost", which is a different
                  fact from "we never checked" — so it says 無 rather than being
                  left blank. */}
              <td>{stream.missing_seq?.length ? stream.missing_seq.join("、") : "無"}</td>
              <td>{stream.late_events}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>事件（已遮罩，依序重建）</h2>
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

function TraceEventRow({ event }: { event: TraceEvent }) {
  return (
    <li>
      <p>
        <code>#{event.seq}</code> {event.occurred_at} · {event.emitted_by} · {event.type}
        {event.status ? ` · ${event.status}` : null}
        {event.late ? " · 遲到" : null}
        {event.masked_fields.length > 0 ? ` · 已遮罩 ${event.masked_fields.length} 個欄位` : null}
      </p>
      {/* Inert text. The payload crossed the trust boundary and is never
          interpreted as markup, ANSI or SVG (ADR-009). */}
      <pre>{JSON.stringify(event.payload, null, 2)}</pre>
    </li>
  );
}
