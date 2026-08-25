import { Loading } from "../components/Loading";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { cancelRun, deleteRunArtifact, useRunArtifacts, type RunArtifact } from "../api/runs";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { InFlight } from "../components/InFlight";
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
    <section>
      {/*
        「Run 結果」 and not 「Run 詳情」: 詳情 is a container word — it says
        「這一頁有一些關於 X 的東西」 rather than naming what the page answers,
        which is the shape 資訊架構 §0.1 R5 exists to refuse. The page is already
        arranged as one answer: 任務判定 first, 執行紀錄 as its evidence, 產出 last.
        Only the title said otherwise.
      */}
      <h1>Run 結果</h1>

      {/* EVAL-001 / design §4.3: the first thing on the page is the task
          judgement, not the run's terminal state. */}
      <EvaluationPanel runId={runId} runStatus={general.data?.status} />

      {/* §2.12: the third axis. Above the cancel control on purpose — 「還在跑，
          而且會自己跑完」 is the fact, and cancelling is what you might do about
          it. It removes itself once the run is terminal. */}
      {general.data && <InFlight summary={general.data} />}

      <CancelRunControl runId={runId} status={general.data?.status} />

      {/* The sections inside the two modes below are h3: they are subsections of
          this heading, and screen-reader users navigate a document by its
          heading levels. 這次 Run 的產出 stays an h2 — it is a sibling of the
          record, not part of it. axe does not catch this: it flags a skipped
          level, never a level that should have been nested and was not. */}
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

const CANCELLABLE = new Set(["queued", "provisioning", "preparing", "running", "evaluating"]);

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
    onError: async (error) => {
      setConfirming(false);
      setMessage(error instanceof Error ? error.message : "取消失敗。");
      await queryClient.invalidateQueries({ queryKey: ["trace", runId] });
    },
  });

  if (!status || !CANCELLABLE.has(status)) return message ? <p role="status">{message}</p> : null;
  if (!confirming) {
    return (
      <p>
        <button type="button" onClick={() => setConfirming(true)}>
          取消這個 Run
        </button>
        {message && <span role="status"> {message}</span>}
      </p>
    );
  }
  return (
    // Design §4.3: notice is 「平台對這一頁講的話」, a standing condition. A
    // two-step confirmation is the user's own action mid-flight, and the app
    // already renders that bare with the scope sentence as .note — see
    // components/ConfirmDelete.tsx, which every other destructive control uses.
    <div>
      <p className="note">確定要取消？已開始的 Sandbox 仍要等平台完成停止與清理。</p>
      <button type="button" disabled={cancel.isPending} onClick={() => cancel.mutate()}>
        確認取消
      </button>{" "}
      <button type="button" disabled={cancel.isPending} onClick={() => setConfirming(false)}>
        返回
      </button>
      {message && <p role="alert">{message}</p>}
    </div>
  );
}

/**
 * 02:WS-002 第 3 條 and 02:SEC-006 第 1 條 — what this run produced, and the only
 * way to delete one of them short of deleting the whole account.
 *
 * Names, sizes and hashes; **no link to the bytes**. The archive is a sandbox's
 * output, the control plane does not open it (iron rule 1), and there is no
 * endpoint that serves it — offering a link would be inventing one.
 *
 * Delete states its scope before it runs and is idempotent by contract, through
 * the same ConfirmDelete the download history and the skill list use. The scope
 * sentence is what differs, and the difference is the point: an evaluation that already cited this file keeps its
 * citation, and that citation will read 「證據已不存在」 rather than silently
 * losing its evidence.
 */
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
    onError: (err) => setMessage(err instanceof Error ? err.message : "刪除失敗。"),
  });

  return (
    <>
      <h2>這次 Run 的產出</h2>
      {artifacts.isPending && <Loading what="產出清單" />}
      {artifacts.error && <p role="alert">無法讀取產出清單：{artifacts.error.message}</p>}
      {message && <p role="status">{message}</p>}

      {artifacts.data &&
        (artifacts.data.artifacts.length === 0 ? (
          <p>這次 Run 沒有留下任何檔案產出。</p>
        ) : (
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
                        的執行紀錄與評估判定都會保留。引用過這個檔案的評估不會被改寫，
                        它會顯示證據已不存在——那是當時真的看過的東西，判定不因為檔案被刪就變得不成立。
                        重複刪除不算失敗。
                      </>
                    }
                  />
                </p>
              </li>
            ))}
          </ul>
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
        {artifact.size_bytes} bytes｜建立於 {artifact.created_at}
        {artifact.expires_at
          ? `｜到期時間 ${artifact.expires_at}`
          : "｜保存期限：尚未設定（平台還沒有為 Run 產出定下保存期限，這不表示它會永久保留）"}
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
      <p className="note">平台不提供這個檔案的下載連結：它是沙箱的產出，控制平面不打開它。</p>
    </>
  );
}

/** Shown whenever a producer's sequence has a hole (TRACE-001, TRACE-008). */
function IncompleteNotice({ complete }: { complete: boolean }) {
  if (complete) return null;
  return (
    // Design §4.3 puts 降級 squarely in the notice surface. `.trace-incomplete`
    // never had a rule in index.css, so the first thing on the page painted as
    // ordinary body text, indistinguishable from the paragraph under it.
    <p role="status" className="notice">
      部分事件未送達，以下內容可能不完整。
    </p>
  );
}

function GeneralMode({ runId }: { runId: string }) {
  const { data, isPending, error } = useTrace(runId, "general");
  if (isPending) return <Loading what="執行紀錄" />;
  if (error) return <p role="alert">無法讀取執行紀錄。</p>;
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
      {/* Status comes from the runs table, not from replayed events (iron rule 5),
          and is worded as execution: `succeeded` says the workload finished, not
          that the task was done (ADR-025). The task verdict is above. */}
      <p role="status">
        執行狀態：<strong>{RUN_STATUS_LABEL[trace.status] ?? trace.status}</strong>（
        <code>{trace.status}</code>）{trace.status_reason ? `（${trace.status_reason}）` : null}
      </p>

      <h3>進度</h3>
      <ol>
        {trace.steps.map((step, i) => (
          <li key={`${i}-${step}`}>{step}</li>
        ))}
      </ol>

      <h3>使用的 Skill</h3>
      {trace.skills.length === 0 ? (
        // 設計 §2.9: 這是真值的 0（Trace 收到了，裡面一個啟用事件都沒有），不是
        // 「未測量」。事件遺失是另一件事，由上面的 IncompleteNotice 講。
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
      {/* Model output over user data: inert text, never rendered as markup. */}
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
            {/* null is "the gateway did not report it", which is not zero. */}
            {/* 設計 §2.9 的表列詞:閘道沒有回報，不是 0。 */}
            {trace.usage.cost_usd === null
              ? "未測量"
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
        // 設計 §2.9:整個用量區塊缺席＝「檢查沒有跑，或這個伺服器版本不回報」。
        <p>用量：未測量。這一格不是 0——沒有量到不等於沒有花費。</p>
      )}
    </div>
  );
}

function AdvancedMode({ runId, active }: { runId: string; active: boolean }) {
  /*
   * 資訊架構 §0.1 R4 / system.md §1.3: which page of the event stream you are
   * looking at is 「你在看哪一份東西」, and this is the screen the design system
   * itself calls 「最需要把『你看，這裡少了三筆』寄給別人的」. As component state,
   * page 7 could not be linked and did not survive a reload.
   *
   * The whole cursor stack travels, not just the current cursor: 上一頁 needs
   * the positions behind it, and a cursor is a platform receive-order offset,
   * not a page number, so it cannot be recomputed. Page 1's stack is `[0]` and
   * carries no param, so the plain address is still the plain address.
   */
  const { events } = useSearch({ strict: false }) as { events?: string };
  const navigate = useNavigate();
  // Page 1's implicit 0 is never in the URL; `events` holds page 2 onwards.
  const pushed = events ? events.split(",").map(Number) : [];
  const cursors = [0, ...pushed];
  const pageIndex = cursors.length - 1;
  const goTo = (next: number[]) =>
    void navigate({
      to: "/runs/$runId",
      params: { runId },
      // Merged rather than replaced: the evaluation revision lives on the same
      // address.
      search: (prev) => ({ ...prev, events: next.length ? next.join(",") : undefined }),
    });
  const { data, isPending, isFetching, error, refetch } = useTrace(
    runId,
    "advanced",
    active,
    cursors[pageIndex],
  );
  // Named apart from the general tab's, because this one is genuinely the
  // heaviest response in the app: up to 1,000 masked payloads a page, with
  // `gcTime: 0` (api/trace.ts), so every return refetches. Saying which tab is
  // loading is what stops it reading as the page having hung.
  if (isPending) return <Loading what="原始事件（一頁最多 1,000 筆，資料量大）" />;
  if (error) return <p role="alert">無法讀取執行紀錄。</p>;
  const trace = data as TraceAdvanced;

  return (
    <div>
      <IncompleteNotice complete={trace.complete} />
      {/* Design §4.3: explanatory prose, so .note — not the notice surface,
          which is reserved for what the platform is currently doing to this
          page (the incomplete banner right above it is one). */}
      <p className="note">
        分頁依平台接收順序排列；每頁內依事件時間排序。這能讓執行中的 Trace 不漏掉較晚送達的事件。
      </p>
      <nav aria-label="Trace event pages">
        <button type="button" disabled={pageIndex === 0} onClick={() => goTo(pushed.slice(0, -1))}>
          上一頁
        </button>
        <span>第 {pageIndex + 1} 頁</span>
        <button
          type="button"
          disabled={!trace.has_more}
          onClick={() => goTo([...pushed, trace.next_after])}
        >
          下一頁
        </button>
        {!trace.has_more && <span className="note"> 沒有更多事件。</span>}{" "}
        <button type="button" disabled={isFetching} onClick={() => void refetch()}>
          重新整理 Trace
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
                {/* An empty missing list is "nothing lost", which is a different
                  fact from "we never checked" — so it says 無 rather than being
                  left blank. */}
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
        // `.trace-events` has no rule in index.css yet, so the UA decimal
        // markers are still on: each event is numbered twice, `1.` beside `#1`.
        // The two disagree the moment a sequence has a hole — the counter is
        // per page and gapless, `seq` is neither — and a hole is the one thing
        // this view exists to make visible (ADR-009). Kept rather than dropped
        // because the fix is one line of CSS in a file this batch does not own:
        // add `.trace-events` to the `.criterion-list, .finding-list,
        // .suggestion-list, .evidence-list` list-reset group.
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
        {(event.masked_fields?.length ?? 0) > 0
          ? ` · 已遮罩 ${event.masked_fields.length} 個欄位`
          : null}
      </p>
      {/* Inert text. The payload crossed the trust boundary and is never
          interpreted as markup, ANSI or SVG (ADR-009). */}
      <pre>{JSON.stringify(event.payload, null, 2)}</pre>
    </li>
  );
}
