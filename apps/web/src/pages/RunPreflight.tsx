import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { useState, type ReactNode } from "react";
import { ApiError } from "../api/client";
import { confirmPreflight, getPreflight, startRun, type PreflightSummary } from "../api/lab";
import { useSkillVersions } from "../api/skills";

/**
 * 03:TEST-008/009 — the pre-run permission summary and its confirmation.
 *
 * SCOPE: the test case and its files come from the Test Case screens
 * (TEST-012), which link here with `skill` and `test_case` filled in. The
 * version is picked on this page from the skill's own history; a `?version=`
 * in the URL is the default that picker opens on, so links written before it
 * existed still open on the version they name (04 丙-14).
 *
 * The one rule this screen exists to enforce: the button that starts a run is
 * only ever wired to the hash of the summary currently on the reader's screen.
 * When the server says that hash is stale (422) the summary is re-read and the
 * user confirms again — an old agreement is never resent (02:TEST-005).
 */

type LabSearch = { skill?: string; version?: string; test_case?: string };

/**
 * The version history of one skill as a `<select>` (WS-001, 04 丙-14). Lives on
 * this page because the pre-run screen is what needed it first; the packaging
 * screen imports it rather than growing a second one, because two pickers over
 * one endpoint would answer the same question in two vocabularies.
 *
 * `value` is owned by the caller, never by this component: on both screens it
 * starts as whatever the URL said, and a link into a specific version must keep
 * opening on that version. So an id that is not in the list — still loading, or
 * a version this workspace cannot see — gets an option of its own and stays
 * selected, rather than the browser silently swapping the reader onto the first
 * option in the list.
 */
export function SkillVersionPicker({
  skillId,
  value,
  onPick,
}: {
  skillId: string;
  value: string;
  onPick: (versionId: string) => void;
}) {
  const versions = useSkillVersions(skillId);
  const list = versions.data?.versions ?? [];
  const unknown = value !== "" && !list.some((v) => v.version_id === value);
  const id = `skill-version-${skillId}`;

  return (
    <p className="version-picker">
      <label htmlFor={id}>Skill Version</label>{" "}
      <select id={id} value={value} onChange={(e) => onPick(e.target.value)}>
        {value === "" && <option value="">請選擇版本…</option>}
        {unknown && <option value={value}>{value}（不在下面的清單裡）</option>}
        {list.map((v, i) => (
          <option key={v.version_id} value={v.version_id}>
            v{v.version_number}
            {i === 0 ? "（最新）" : ""}・{v.created_at.slice(0, 10)}
          </option>
        ))}
      </select>{" "}
      {versions.isPending && <span className="note">載入版本清單中…</span>}
      {versions.error && (
        <span className="note" role="alert">
          無法讀取版本清單：{versions.error.message}
        </span>
      )}
      {!versions.isPending && !versions.error && list.length === 0 && (
        <span className="note">
          這個工作區沒有這個 Skill 的任何版本可選——不代表這個 Skill 沒有版本，Fork
          之後才會有屬於你的版本。
        </span>
      )}
    </p>
  );
}

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
    version: linkedVersion = "",
    test_case: testCase = "",
  } = useSearch({ strict: false }) as LabSearch;
  // The URL's version is the default the picker opens on; a pick replaces it.
  // Deliberately not written back into the URL: this page's whole subject is
  // what the *current* selection may touch, and a history entry per pick would
  // be a back button that walks through permission summaries nobody confirmed.
  const [picked, setPicked] = useState("");
  const version = picked || linkedVersion;
  const ready = skill !== "" && testCase !== "";
  const [message, setMessage] = useState("");
  const [runId, setRunId] = useState("");

  const preflight = useQuery({
    queryKey: ["preflight", skill, version, testCase],
    queryFn: () => getPreflight(skill, version, testCase),
    enabled: ready && version !== "",
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
          這個頁面需要 <code>?skill=&amp;test_case=</code> 兩個 ID。Test Case 與 Dataset 請在{" "}
          <Link to="/lab/test-cases">Test Case 頁</Link> 建立,再從那裡連過來;要跑哪一個 Skill
          Version 在這個頁面上選。
        </p>
      </section>
    );
  }

  // One shell for every state, so the picker never disappears while the summary
  // it selects is loading — a picker that vanishes mid-load is one the reader
  // cannot use to get out of a version that fails to load.
  const shell = (children: ReactNode) => (
    <section className="page">
      <h1>執行前權限確認</h1>
      <SkillVersionPicker
        skillId={skill}
        value={version}
        onPick={(id) => {
          setPicked(id);
          // A different version is a different summary and a different run: the
          // last run's id and the last error belong to what was on screen before.
          setRunId("");
          setMessage("");
        }}
      />
      {children}
    </section>
  );

  if (version === "") return shell(<p>請先在上面選一個 Skill Version,才有權限摘要可以看。</p>);
  if (preflight.isPending) return shell(<p>載入權限摘要中…</p>);
  if (preflight.error) {
    return shell(<p role="alert">無法讀取權限摘要:{preflight.error.message}</p>);
  }

  const { summary, summary_hash: hash, estimated_cost: cost, quota, notes } = preflight.data;

  return shell(
    <>
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
        {/* PDM-010 / ADR-028: the display comes after the enforcement, never
            instead of it. These are the counters that refuse the run below, so
            the numbers are a report on a rule rather than the rule. A deployment
            with no allowance sends no `quota` block and this row is absent —
            rendering zeroes would be claiming a ceiling that does not exist
            (04 乙-2). */}
        {quota && (
          <>
            <dt>剩餘試跑額度</dt>
            <dd>
              今天還可以跑 {quota.remaining_today} 次、這個週期還可以跑 {quota.remaining_window}{" "}
              次（滾動視窗，最舊的一筆在 {quota.window_resets_at} 退出計算）。
              <p className="note">
                上限：每日 {quota.limits.daily} 次、每 {quota.limits.window_days} 天{" "}
                {quota.limits.window} 次、同時進行 {quota.limits.concurrent} 個。 這些數字就是建立
                Run 時擋你的那一份計數，不是另外顯示的估計。
              </p>
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
          <Link to="/runs/$runId" params={{ runId }}>
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
    </>,
  );
}
