import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { useState } from "react";
import { ApiError } from "../api/client";
import { confirmPreflight, getPreflight, startRun, type PreflightSummary } from "../api/lab";

/**
 * 03:TEST-008/009 — the pre-run permission summary and its confirmation.
 *
 * SCOPE, stated honestly: the test case and its files come from the Test Case
 * screens (TEST-012), which link here with `skill` and `test_case` filled in.
 * There is still no version picker, so `version` is the one id a user has to
 * supply. This page is the gate screen and nothing else.
 *
 * The one rule this screen exists to enforce: the button that starts a run is
 * only ever wired to the hash of the summary currently on the reader's screen.
 * When the server says that hash is stale (422) the summary is re-read and the
 * user confirms again — an old agreement is never resent (02:TEST-005).
 */

type LabSearch = { skill?: string; version?: string; test_case?: string };

function bytes(n: number): string {
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`;
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

const SCRIPT_LABEL: Record<PreflightSummary["scripts"]["status"], string> = {
  none: "無(靜態掃描未發現 Script 或內嵌程式碼)",
  present: "有",
  // Never rendered as 無: a package that could not be read is not a clean one.
  unavailable: "未知(套件無法讀取,未完成掃描)",
};

export function RunPreflight() {
  const {
    skill = "",
    version = "",
    test_case: testCase = "",
  } = useSearch({ strict: false }) as LabSearch;
  const ready = skill !== "" && version !== "" && testCase !== "";
  const [message, setMessage] = useState("");
  const [runId, setRunId] = useState("");

  const preflight = useQuery({
    queryKey: ["preflight", skill, version, testCase],
    queryFn: () => getPreflight(skill, version, testCase),
    enabled: ready,
    retry: false,
    // Never served from cache: this screen's whole job is to show what is true
    // now, and a stale summary would be confirmed against a hash the server has
    // already stopped accepting.
    staleTime: 0,
    gcTime: 0,
  });

  const confirmAndRun = useMutation({
    mutationFn: async (hash: string) => {
      await confirmPreflight(skill, version, testCase, hash);
      return startRun(skill, version, testCase, hash);
    },
    onSuccess: (created) => {
      setRunId(created.run_id);
      setMessage("");
    },
    onError: async (err) => {
      if (err instanceof ApiError && err.status === 422) {
        // The permissions moved between reading and confirming. Re-read and make
        // the user agree to the new summary; nothing is retried automatically.
        setMessage("權限內容已變更,請重新確認下方摘要後再開始 Run。");
        await preflight.refetch();
        return;
      }
      setMessage(err instanceof Error ? err.message : "無法開始 Run。");
    },
  });

  if (!ready) {
    return (
      <section className="page">
        <h1>執行前權限確認</h1>
        <p>
          這個頁面需要 <code>?skill=&amp;version=&amp;test_case=</code> 三個 ID。Test Case 與
          Dataset 請在 <Link to="/lab/test-cases">Test Case 頁</Link> 建立,再從那裡連過來;Skill
          Version id 目前仍需自行填入。
        </p>
      </section>
    );
  }

  if (preflight.isPending) return <p>載入權限摘要中…</p>;
  if (preflight.error) {
    return <p role="alert">無法讀取權限摘要:{preflight.error.message}</p>;
  }

  const { summary, summary_hash: hash, estimated_cost: cost, notes } = preflight.data;

  return (
    <section className="page">
      <h1>執行前權限確認</h1>
      <p>以下是這次 Run 可以接觸的範圍。確認後才會開始執行。</p>

      <dl className="preflight">
        <dt>Dataset</dt>
        <dd>
          {summary.datasets.length === 0 ? (
            "無"
          ) : (
            <ul>
              {summary.datasets.map((d) => (
                <li key={d.dataset_id}>
                  {d.file_name}（{bytes(d.size_bytes)}）
                </li>
              ))}
            </ul>
          )}
          {summary.datasets.length > 0 && <p>合計 {bytes(summary.dataset_total_bytes)}</p>}
        </dd>

        <dt>Script</dt>
        <dd>
          {SCRIPT_LABEL[summary.scripts.status]}
          {summary.scripts.findings.length > 0 && (
            <ul>
              {summary.scripts.findings.map((f) => (
                <li key={f}>{f}</li>
              ))}
            </ul>
          )}
        </dd>

        <dt>工具</dt>
        <dd>{summary.tools.join("、")}</dd>

        {/* MVP 沒有 MCP。明確顯示為「無」,不是省略不提。 */}
        <dt>MCP Server</dt>
        <dd>{summary.mcp_servers.length === 0 ? "無" : summary.mcp_servers.join("、")}</dd>

        <dt>網路</dt>
        <dd>
          {summary.network.mode}
          {summary.network.allow.length === 0
            ? "（允許清單為空:不能連出任何位址）"
            : `（允許 ${summary.network.allow.join("、")}）`}
        </dd>

        <dt>Secrets</dt>
        <dd>
          {summary.injected_secrets.join("、")}
          <p>只顯示名稱;實際值為每個 Run 專屬的短效憑證,不會顯示於任何畫面。</p>
        </dd>

        <dt>Provider</dt>
        <dd>
          {summary.provider.name}
          {summary.provider.isolation_level && `（隔離:${summary.provider.isolation_level}）`}
        </dd>

        <dt>資源上限</dt>
        <dd>
          vCPU {summary.resource_limits.vcpu}、記憶體 {bytes(summary.resource_limits.memory_bytes)}
          、 磁碟 {bytes(summary.resource_limits.disk_bytes)}、 時間上限{" "}
          {summary.resource_limits.wall_clock_hard_seconds} 秒、 Token{" "}
          {summary.resource_limits.token_budget.max_input_tokens} 進 /{" "}
          {summary.resource_limits.token_budget.max_output_tokens} 出
        </dd>
        {/* PDM-005 §5.3. A range, and labelled an estimate in the term itself —
            it is outside summary_hash, so it must not read like something the
            user is agreeing to. Absent on an older server: rendered as nothing
            rather than as a zero. */}
        {cost && (
          <>
            <dt>預估成本（估計值）</dt>
            <dd>
              {cost.currency} ${cost.low.toFixed(2)} – ${cost.high.toFixed(2)}（常見約 $
              {cost.typical.toFixed(2)}）<p>{cost.basis}</p>
            </dd>
          </>
        )}
      </dl>

      <ul className="preflight-notes">
        {notes.map((n) => (
          <li key={n}>{n}</li>
        ))}
      </ul>

      {message && <p role="alert">{message}</p>}

      {runId ? (
        <p>
          已開始 Run:{" "}
          {/* `skill` rides along so the run page can offer the EVAL-002 apply
              action; the run read endpoint does not carry a skill id. */}
          <Link to="/runs/$runId" params={{ runId }} search={{ skill }}>
            {runId}
          </Link>
        </p>
      ) : (
        <button
          type="button"
          disabled={confirmAndRun.isPending}
          onClick={() => confirmAndRun.mutate(hash)}
        >
          我確認以上權限,開始 Run
        </button>
      )}
      <p>不同意就不要按下按鈕:未確認的 Run 不會被建立。</p>
    </section>
  );
}
