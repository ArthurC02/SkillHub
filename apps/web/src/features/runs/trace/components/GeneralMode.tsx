import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useTrace } from "../../trace.service";
import type { TraceSummary } from "../../trace.service";
import { runStatusLabel } from "../../runs.model";
import { IncompleteNotice } from "./IncompleteNotice";
import { FailureClass } from "./FailureClass";
import { RunCleanupStatus } from "./RunCleanupStatus";

export function GeneralMode({ runId }: { runId: string }) {
  const { data, isPending, error } = useTrace(runId, "general");
  if (isPending) return <Loading what="執行紀錄" />;
  if (error)
    return (
      <ReadFailure error={error} what="執行紀錄">
        <p role="alert">無法讀取執行紀錄。</p>
      </ReadFailure>
    );
  const trace = data as TraceSummary;

  return (
    <div>
      <IncompleteNotice complete={trace.complete} />
      {trace.summary_truncated ? (
        <p className="notice">
          重複事件僅顯示前 100 筆（Skill {trace.skills.length}/{trace.skills_total}；錯誤{" "}
          {trace.errors.length}/{trace.errors_total}）。完整事件仍可在進階模式分頁查看。
        </p>
      ) : null}
      <p role="status">
        執行狀態：<strong>{runStatusLabel(trace.status)}</strong>（<code>{trace.status}</code>）
        {trace.status_reason ? `（${trace.status_reason}）` : null}
      </p>
      <FailureClass runId={runId} status={trace.status} />
      <RunCleanupStatus runId={runId} />

      <h3>進度</h3>
      <ol data-role="evidence">
        {trace.steps.map((step, i) => (
          <li key={`${i}-${step.status}`}>
            <strong>{runStatusLabel(step.status)}</strong>
            {step.reason ? `：${step.reason}` : null}
          </li>
        ))}
      </ol>

      <h3>使用的 Skill</h3>
      {trace.skills.length === 0 ? (
        <p>Skill 啟用事件 0 筆。</p>
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

      <h3>工作內容</h3>
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
          <h3>錯誤</h3>
          <ul>
            {trace.errors.map((err, i) => (
              <li key={`${err.code}-${i}`}>
                [{err.category}/{err.code}] {err.message}
              </li>
            ))}
          </ul>
        </>
      ) : null}

      <h3>最終輸出</h3>
      {trace.final_output ? <pre>{trace.final_output}</pre> : <p>尚無最終輸出。</p>}

      <h3>用量</h3>
      {trace.usage ? (
        <ul>
          <li>模型：{trace.usage.model || "未知"}</li>
          <li>
            Token：輸入 {trace.usage.input_tokens}、輸出 {trace.usage.output_tokens}
          </li>
          <li>
            成本：
            {trace.usage.cost_credits === null
              ? "未測量"
              : `${trace.usage.cost_credits} 點${
                  trace.usage.cost_source === "estimated" ? "（估算值）" : ""
                }`}
          </li>
          <li className="note">
            這是<strong>下界</strong>，不是總額：合計自 Trace 的用量事件， 權威來源是模型閘道對這個
            Run 的 per-key 實付。
          </li>
        </ul>
      ) : (
        <p>用量：未測量。這一格不是 0——沒有量到不等於沒有花費。</p>
      )}
    </div>
  );
}
