import { Loading } from "../components/Loading";
import { Timestamp } from "../components/Timestamp";
import { ReadFailure } from "../components/LoginRequired";
import { ApiError } from "../api/client";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import {
  cancelRun,
  deleteRunArtifact,
  useRun,
  useRunArtifacts,
  type RunArtifact,
} from "../api/runs";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { InFlight } from "../components/InFlight";
import { useTrace, IN_FLIGHT_RUN_STATUSES } from "../api/trace";
import type { TraceAdvanced, TraceEvent, TraceSummary } from "../api/trace";
import { EvaluationPanel, runStatusLabel } from "./RunEvaluation";
import { CLEANUP_BADGE } from "./WorkspaceRuns";

export function RunTrace() {
  const { runId } = useParams({ from: "/runs/$runId" });
  const { events: linkedEvents } = useSearch({ strict: false }) as { events?: string };
  const [mode, setMode] = useState<"general" | "advanced">(linkedEvents ? "advanced" : "general");
  const run = useRun(runId);
  const general = useTrace(runId, "general");

  return (
    <section>
      <h1>Run 結果</h1>
      <ReadFailure error={run.error} what="這個 Run" />
      {run.data && (
        <p className="note">
          <Link to="/skills/$skillId" params={{ skillId: run.data.skill_id }}>
            回到這個 Run 的 Skill
          </Link>
        </p>
      )}

      <EvaluationPanel runId={runId} runStatus={general.data?.status} />

      {general.data && <InFlight summary={general.data} />}

      <CancelRunControl runId={runId} status={general.data?.status} />

      <p>
        <Link to="/runs/$runId/compare" params={{ runId }} search={{ against: "" }}>
          與另一個 Run 比較
        </Link>
      </p>

      <h2>執行紀錄</h2>
      <p className="note" data-role="teaching">
        一般模式是摘要，進階模式是這次 Run 的原始事件（已遮罩）——同一份紀錄的兩種詳細度，
        只影響下面這一節。
      </p>
      <div role="group" aria-label="執行紀錄的詳細度">
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
      {mode === "general" ? (
        <GeneralMode runId={runId} />
      ) : (
        <AdvancedMode
          key={runId}
          runId={runId}
          active={Boolean(general.data?.status && CANCELLABLE.has(general.data.status))}
        />
      )}

      <RunArtifacts runId={runId} />
    </section>
  );
}

const CANCELLABLE = IN_FLIGHT_RUN_STATUSES;

function cancelFailureSentence(error: unknown): string {
  if (error instanceof ApiError && error.status === 409)
    return "這個 Run 已經結束，沒有東西可以取消。";
  if (error instanceof ApiError && error.status === 404) return "找不到這個 Run。";
  return "取消要求沒有送出，可以再按一次。";
}

export function CancelRunControl({ runId, status }: { runId: string; status?: string }) {
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState("");
  const cancel = useMutation({
    mutationFn: () => cancelRun(runId),
    onSuccess: async (result) => {
      setConfirming(false);
      setMessage(result.note ?? "已送出取消要求。");
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["trace", runId] }),
        queryClient.invalidateQueries({ queryKey: ["run", runId] }),
      ]);
    },
    onError: async () => {
      setConfirming(false);
      await queryClient.invalidateQueries({ queryKey: ["trace", runId] });
    },
  });
  const failure = cancel.error ? (
    <ReadFailure error={cancel.error} what="取消這個 Run">
      <p role="alert">{cancelFailureSentence(cancel.error)}</p>
    </ReadFailure>
  ) : null;

  if (!status || !CANCELLABLE.has(status))
    return (
      <>
        {failure}
        {message && <p role="status">{message}</p>}
      </>
    );
  if (!confirming) {
    return (
      <p>
        <button type="button" onClick={() => setConfirming(true)}>
          取消這個 Run
        </button>
        {message && <span role="status"> {message}</span>}
        {failure}
      </p>
    );
  }
  return (
    <div>
      <p className="note">確定要取消？已開始的 Sandbox 仍要等平台完成停止與清理。</p>
      <button type="button" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
        確認取消
      </button>{" "}
      <button type="button" disabled={cancel.isPending} onClick={() => setConfirming(false)}>
        返回
      </button>
      {failure}
    </div>
  );
}

function RunArtifacts({ runId }: { runId: string }) {
  const artifacts = useRunArtifacts(runId);
  const client = useQueryClient();
  const [message, setMessage] = useState("");

  const remove = useMutation({
    mutationFn: (artifactId: string) => deleteRunArtifact(runId, artifactId),
    onSuccess: async () => {
      setMessage("已刪除。檔案不再存在，引用過它的評估會顯示證據已不存在。");
      await client.invalidateQueries({ queryKey: ["run", runId, "artifacts"] });
    },
    onError: () => {},
  });

  return (
    <>
      <h2>這次 Run 的產出</h2>
      {artifacts.isPending && <Loading what="產出清單" />}
      <ReadFailure error={artifacts.error} what="產出清單" />
      <ReadFailure error={remove.error} what="刪除這個產出">
        <p role="alert">沒有刪成，可以再按一次。</p>
      </ReadFailure>
      {message && <p role="status">{message}</p>}
      {artifacts.data?.truncated && (
        <p className="notice" role="status">
          有些產出未被收集；清單只保留成功收集的檔案。
        </p>
      )}

      {artifacts.data &&
        (artifacts.data.artifacts.length === 0 ? (
          <p>
            {artifacts.data.truncated
              ? "收集結果為空，無法據此判定這次 Run 沒有產生檔案。"
              : "這次 Run 沒有留下任何檔案產出。"}
          </p>
        ) : (
          <>
            <p className="note">
              這些檔案平台都不提供下載連結：每一個都是沙箱的產出，控制平面不打開它。
              {artifacts.data.artifacts.some((a) => !a.expires_at) &&
                "「尚未定值」是平台還沒有為 Run 產出定下保存期限，這不表示它會永久保留。"}
            </p>
            <ul className="download-list">
              {artifacts.data.artifacts.map((artifact) => (
                <li key={artifact.artifact_id} className="download-item">
                  <RunArtifactFacts artifact={artifact} />
                  <p>
                    <ConfirmDelete
                      scopeId={`run-artifact-scope-${artifact.artifact_id}`}
                      pending={remove.isPending}
                      onAsk={() => setMessage("")}
                      onConfirm={() => remove.mutate(artifact.artifact_id)}
                      scope={
                        <>
                          刪除的是這個檔案本身，這個 Run
                          的執行紀錄與評估判定都會保留。沒有回收桶也沒有保留期，這一頁沒有還原的地方，
                          刪了就取不回這個檔案。引用過這個檔案的評估不會被改寫，
                          它會顯示證據已不存在——那是當時真的看過的東西，判定不因為檔案被刪就變得不成立。
                          重複刪除不算失敗。
                        </>
                      }
                    />
                  </p>
                </li>
              ))}
            </ul>
          </>
        ))}
    </>
  );
}

function RunArtifactFacts({ artifact }: { artifact: RunArtifact }) {
  return (
    <>
      <p>
        <strong>{artifact.file_name}</strong> <span className="badge">{artifact.content_type}</span>
        {artifact.purged && (
          <>
            {" "}
            <span className="badge badge-expired">檔案已不存在</span>
          </>
        )}
      </p>
      <p className="note">
        {artifact.size_bytes} bytes｜建立於 <Timestamp at={artifact.created_at} />
        {artifact.expires_at ? (
          <>
            ｜到期時間 <Timestamp at={artifact.expires_at} />
          </>
        ) : (
          "｜保存期限：尚未定值"
        )}
      </p>
      {artifact.purged && (
        <p className="note">
          內容已被清除（到期或儲存端已不存在），這一列保留，因為「曾經產生過這個檔案」仍然是事實。
        </p>
      )}
      <details>
        <summary>內容雜湊</summary>
        <code>{artifact.content_hash}</code>
      </details>
    </>
  );
}

function IncompleteNotice({ complete }: { complete: boolean }) {
  if (complete) return null;
  return (
    <p role="status" className="notice">
      部分事件未送達，以下內容可能不完整。
    </p>
  );
}

function FailureClass({ runId, status }: { runId: string; status: string }) {
  const run = useRun(runId);
  if (status !== "failed") return null;
  if (run.error) return null;
  return (
    <>
      <p>
        失敗類別：
        <strong>{run.data?.failure_class?.label ?? (run.isPending ? "讀取中…" : "未記錄")}</strong>
      </p>
      {run.data?.failure_class && <p className="note">{run.data.failure_class.note}</p>}
    </>
  );
}

function RunCleanupStatus({ runId }: { runId: string }) {
  const run = useRun(runId);
  if (!run.data?.cleanup_status) return null;
  const { cleanup_status } = run.data;
  return (
    <p className="badge-row">
      <span className={CLEANUP_BADGE[cleanup_status.value] ?? "badge badge-unverified"}>
        清理狀態：{cleanup_status.label}
      </span>{" "}
      <span className="note">{cleanup_status.note}</span>
    </p>
  );
}

function GeneralMode({ runId }: { runId: string }) {
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
      <ol>
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

function AdvancedMode({ runId, active }: { runId: string; active: boolean }) {
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
        <code>#{event.seq}</code> <Timestamp at={event.occurred_at} /> · {event.emitted_by} ·{" "}
        {event.type}
        {event.status ? ` · ${event.status}` : null}
        {event.late ? " · 遲到" : null}
        {(event.masked_fields?.length ?? 0) > 0
          ? ` · 已遮罩 ${event.masked_fields.length} 個欄位`
          : null}
      </p>
      {/* payload is untrusted sandbox output; stringify + <pre> renders it as inert text, never HTML */}
      <pre>{JSON.stringify(event.payload, null, 2)}</pre>
    </li>
  );
}
