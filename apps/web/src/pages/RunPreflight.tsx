import { Loading } from "../components/Loading";
import { formatAt, Timestamp } from "../components/Timestamp";
import { LoginRequired, ReadFailure, unauthenticated } from "../components/LoginRequired";
import { useMe } from "../api/me";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { useEffect, useState, type ReactNode } from "react";
import { ApiError } from "../api/client";
import { confirmPreflight, getPreflight, startRun, type PreflightSummary } from "../api/lab";
import { useSkillVersions } from "../api/skills";
import { useOwnSkills, useTestCase } from "../api/testcases";

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
    <p>
      <label htmlFor={id}>Skill Version</label>{" "}
      <select id={id} value={value} onChange={(e) => onPick(e.target.value)}>
        {value === "" && <option value="">請選擇版本…</option>}
        {unknown && <option value={value}>{value}（不在下面的清單裡）</option>}
        {list.map((v, i) => (
          <option key={v.version_id} value={v.version_id}>
            v{v.version_number}
            {i === 0 ? "（最新）" : ""}・{formatAt(v.created_at)}
          </option>
        ))}
      </select>{" "}
      {versions.isPending && <Loading what="版本清單" className="note" />}
      <ReadFailure error={versions.error} what="版本清單">
        <span className="note" role="alert">
          無法讀取版本清單：{versions.error?.message}
        </span>
      </ReadFailure>
      {!versions.isPending && !versions.error && list.length === 0 && (
        <span className="note">
          這個工作區沒有這個 Skill 的任何版本可選——不代表這個 Skill 沒有版本，Fork
          之後才會有屬於你的版本。
        </span>
      )}
    </p>
  );
}

/** Shared with the Test Case screens rather than copied there (設計 §4.1 的同一把尺). */
export function bytes(n: number): string {
  if (n >= 1024 ** 3) return `${(n / 1024 ** 3).toFixed(1)} GB`;
  if (n >= 1024 ** 2) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${n} B`;
}

/**
 * A resource ceiling, which is not the same kind of number as a file size.
 *
 * 設計 §2.2「不強制的東西就不要顯示」 and §2.1「未知不是空白」 are two different
 * failures here and they need two different words:
 *
 *   - **absent** — the server sent no ceiling at all. That is 未知, and 未知 is
 *     rendered as a word rather than as a gap.
 *   - **zero** — the server sent a ceiling of 0. That is not 未知 and it is not a
 *     limit either: no sandbox runs in 0 B of memory, so whatever the run will
 *     actually be held to, it is not this. Printing `0 B` puts a number that
 *     passed every rule in the design system (not blank, carries a word, on the
 *     type scale) in front of a user who is about to press 我確認 — which is
 *     precisely the 04 乙-2 shape: display without enforcement.
 *
 * `bytes()` cannot make this call, and must not: a dataset of 0 bytes is a real
 * empty file. The judgement is only ever about ceilings, so it lives here and is
 * applied only to them.
 */
function ceiling(n: number | undefined): string {
  return limit(n, bytes);
}

/**
 * The same judgement for the ceilings that are not measured in bytes.
 *
 * `ceiling` grew a tail that called `bytes()`, so it could only ever guard the
 * four byte-valued limits. The other seven on this page — vCPU, both wall
 * clocks, both token budgets, pids and open files — printed
 * `summary.resource_limits.x` raw, which means a server answering `0` or
 * omitting the field put a bare `0` or a blank in front of somebody about to
 * press 我確認. That is the 04 乙-2 shape the block above this describes, and it
 * applied to seven of the eleven numbers on the screen (M2 audit, 2026-08-24).
 */
function limit(n: number | undefined, format: (n: number) => string): string {
  // 設計 §2.9 的表是封閉的六個詞，而「未回報」不在上面——它是「未測量」的同義
  // 詞，也就是「檢查沒有跑，或這個伺服器版本不回報」的那一格。
  if (typeof n !== "number" || !Number.isFinite(n)) return "未測量";
  if (n <= 0) return `伺服器回報 ${n}——這不是有效的上限,請勿據此判斷這次 Run 的可用資源`;
  return format(n);
}

const count = (n: number) => `${n}`;
const seconds = (n: number) => `${n} 秒`;
const tokens = (n: number) => `${n}`;

export const SCRIPT_LABEL: Record<PreflightSummary["scripts"]["status"], string> = {
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
  // 資訊架構 IA-6: read the session, only so the two answers below come out in
  // the right order. WorkspaceAccount's precedent — the whole query object, not
  // Home's `!!useMe().data` — because 「還在問」 must not be rendered as 「沒登入」
  // on a page whose entire body is at stake.
  const me = useMe();
  // 主詞。這一頁鉅細靡遺地說出這次 Run 會碰到哪些工具、能連哪些網路、吃掉多少記憶體，
  // 卻從頭到尾沒說**你正要拿哪一份東西、去跑哪一段題目**——唯一能認出主詞的線索是
  // 網址列裡的兩個 UUID，而按鈕上寫著「我確認以上權限」。設計 §3 第 1、2 條：這一頁
  // 的頭條不是「有哪些權限」，是「你正要拿這一份去跑這一件事」，權限是它的細節。
  // 兩個 hook 都是既有的、而且使用者剛剛走過的畫面已經讀過（TestCaseDetail 讀
  // `useTestCase`，TestCaseList 的選單讀 `useOwnSkills`），React Query 同 key 去重。
  const testCaseInfo = useTestCase(testCase);
  const ownSkills = useOwnSkills();
  const [message, setMessage] = useState("");
  const [runId, setRunId] = useState("");
  useEffect(() => {
    setPicked("");
    setMessage("");
    setRunId("");
  }, [skill, linkedVersion, testCase]);

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
        // Gate B answers 422 for SIX different refusals (execution/http.go):
        // a stale summary hash, a missing confirmation, an exhausted allowance,
        // a blocking static scan, the workspace concurrency ceiling, and a
        // capability mismatch. This branch used to report all of them as
        // 「權限內容已變更」 and throw err.message away — so a user out of their
        // daily allowance was told their permissions had moved, shown an
        // identical summary, and sent round the loop again, while the server's
        // own sentence (which carries the reset time, because entitlements
        // deliberately puts it there — "come back later" without a time is
        // unactionable) went in the bin. M2 audit, 2026-08-24.
        //
        // The server's sentence is the answer; this page says only what it
        // knows, which is that nothing started. The refetch is right for all
        // six: whatever moved, the summary below should be current.
        setMessage(
          err.message
            ? `這次 Run 沒有開始：${err.message}`
            : "這次 Run 沒有開始。下方摘要已重新讀取,請確認之後再試。",
        );
        await preflight.refetch();
        return;
      }
      setMessage(err instanceof Error ? err.message : "無法開始 Run。");
    },
  });

  // Ordered before the missing-parameters branch on purpose. A logged-out
  // visitor arriving at /lab/run was told 「這個頁面需要 ?skill=&test_case= 兩個
  // ID」 — it sent someone with no session away to hunt for query parameters, an
  // instruction that would not have helped even if they had followed it
  // (資訊架構 §5 IA-6「一頁誤導」).
  if (unauthenticated(me.error)) {
    return (
      <section>
        <h1>執行前權限確認</h1>
        <LoginRequired what="試跑與執行前權限確認" />
      </section>
    );
  }

  if (!ready) {
    return (
      <section>
        <h1>執行前權限確認</h1>
        <p>
          {/* 資訊架構 §5 IA-6 把「這個頁面需要兩個 ID」明文記成「一頁誤導」。當時的
              修法只是把未登入分支排到它前面——**那句話本身沒有被改，而它今天還有
              一條活的入邊**：GeneratedNotice 的「先跑一次試跑」連到 /lab/run?skill=
              而刻意不帶 test_case。一個剛生成完一個 Skill 的人，被 app 內部的連結
              送到一句教他讀網址的話上。連結也補上 `skill`，否則他手上唯一有的那個
              id 在這一跳被丟掉。 */}
          {skill !== ""
            ? "還要先挑一個 Test Case，才知道這次要跑哪一段題目。"
            : "要先挑一個 Skill 與一個 Test Case。"}
          {" 到 "}
          <Link to="/lab/test-cases" search={{ skill: skill || undefined }}>
            Test Case 頁
          </Link>
          建立或選一個,再從那裡連過來;要跑哪一個 Skill Version 在這個頁面上選。
        </p>
      </section>
    );
  }

  // One shell for every state, so the picker never disappears while the summary
  // it selects is loading — a picker that vanishes mid-load is one the reader
  // cannot use to get out of a version that fails to load.
  const skillName = ownSkills.data?.skills.find((sk) => sk.skill_id === skill)?.name;
  const criteria = testCaseInfo.data?.acceptance_criteria.length;

  const shell = (children: ReactNode) => (
    <section>
      <h1>執行前權限確認</h1>
      <p>
        Skill：<strong>{skillName ?? (ownSkills.isPending ? "讀取中…" : "不在你的清單裡")}</strong>
        {" ・ "}
        Test Case：
        <strong>
          {testCaseInfo.data?.name ?? (testCaseInfo.isPending ? "讀取中…" : "讀不到名稱")}
        </strong>
      </p>
      {criteria === 0 && (
        // 這一句要在最後一個可以反悔的畫面上說。沒有驗收條件的 Run 跑得起來、會花掉
        // 額度、而且不會產生任何逐條判定——在此之前沒有一個畫面提過。
        <p className="note">
          這個 Test Case 沒有驗收條件，所以這次 Run 不會產生逐條判定。試跑本身照常執行。
        </p>
      )}
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
  if (preflight.isPending) return shell(<Loading what="權限摘要" />);
  if (preflight.error) {
    return shell(
      <ReadFailure error={preflight.error} what="權限摘要">
        <p role="alert">無法讀取權限摘要:{preflight.error.message}</p>
      </ReadFailure>,
    );
  }

  const { summary, summary_hash: hash, estimated_cost: cost, quota, notes } = preflight.data;

  return shell(
    <>
      <p>以下是這次 Run 可以接觸的範圍。確認後才會開始執行。</p>

      {/*
        設計 §1.1: 成本是這一頁被點名的那份證據,而 §1.2 說頭條不能排在識別碼與
        例行細節後面。這兩列原本在 <dl> 的最後,900px 的第一屏之外(手機更遠),
        所以讀者要按下「我確認」時,能不捲動就看到的只有 MCP Server=無、網路、
        Secrets、Provider 這些例行項。錢與剩餘次數先講,其餘照原順序。
      */}
      <dl>
        {/* PDM-005 §5.3. A range, and labelled an estimate in the term itself —
            it is outside summary_hash, so it must not read like something the
            user is agreeing to.

            Absent on an older server, this row used to render as NOTHING —
            設計 §2.9 names this exact spot as a FAIL: it avoided the zero-fill
            (right) and landed on a blank (wrong), and a blank in a cost row in
            front of somebody about to press 我確認 reads as free. The word is
            「未測量」:「這個伺服器版本不回報」. */}
        <dt>預估成本（估計值）</dt>
        <dd>
          {cost ? (
            <>
              {cost.currency} ${cost.low.toFixed(2)} – ${cost.high.toFixed(2)}（常見約 $
              {cost.typical.toFixed(2)}）<p>{cost.basis}</p>
            </>
          ) : (
            <>未測量——這個伺服器版本沒有回報預估成本，不代表這次 Run 不花錢。</>
          )}
        </dd>
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
              次。
              {/*
                設計 §2.2. 這裡曾經寫「滾動視窗,最舊的一筆在 X 退出計算」,而
                entitlements/quota.go 的 Usage() 把 window_resets_at 設成
                「最舊的一筆退出」與「首個視窗的上限抬高」兩者中較早的那一個 ——
                成立未滿 30 天的工作區走的是後者,一次都還沒跑的工作區則兩者皆非。
                機制的說法對多數新帳號是假的,日期不是,所以只留日期,並且只主張
                它是下界。
              */}
              額度下一次增加不會早於 <Timestamp at={quota.window_resets_at} />。
              <p className="note">
                上限：每日 {quota.limits.daily} 次、每 {quota.limits.window_days} 天{" "}
                {quota.limits.window} 次、同時進行 {quota.limits.concurrent} 個。 這些數字就是建立
                Run 時擋你的那一份計數，不是另外顯示的估計。
              </p>
            </dd>
          </>
        )}

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

        {/* 設計 §2.1: 空陣列渲染成空白會被讀成「這裡沒有風險」,而這正是使用者
            按下確認的那一頁。與下面的 MCP Server 同一條規則,同一個做法。 */}
        <dt>工具</dt>
        <dd>{summary.tools.length === 0 ? "無（沒有授予任何工具）" : summary.tools.join("、")}</dd>

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
          {summary.injected_secrets.length === 0 ? (
            "無（不會注入任何 Secret）"
          ) : (
            <>
              {summary.injected_secrets.join("、")}
              <p>只顯示名稱;實際值為每個 Run 專屬的短效憑證,不會顯示於任何畫面。</p>
            </>
          )}
        </dd>

        <dt>Provider</dt>
        <dd>
          {summary.provider.name}
          {summary.provider.isolation_level && `（隔離:${summary.provider.isolation_level}）`}
        </dd>

        <dt>資源上限</dt>
        <dd>
          vCPU {limit(summary.resource_limits.vcpu, count)}、記憶體{" "}
          {ceiling(summary.resource_limits.memory_bytes)}、 磁碟{" "}
          {ceiling(summary.resource_limits.disk_bytes)}、 時間上限{" "}
          {limit(summary.resource_limits.wall_clock_hard_seconds, seconds)}、 Token{" "}
          {limit(summary.resource_limits.token_budget?.max_input_tokens, tokens)} 進 /{" "}
          {limit(summary.resource_limits.token_budget?.max_output_tokens, tokens)} 出
          {/*
            02:RUN-003「Token 上限必須連同輪數換算表一起呈現，不得只寫『300K』」
            (PDM-005 §5.2a-2), and 02:TEST-005 記著這條義務在三個落點一處都沒有
            實作過。它不是排版偏好：`300000` 對讀者不可讀，而同一個 300K 在工具
            密集的 Run 只夠約 5 輪、純對話夠約 15 輪——**差三倍**，而那正是他按下
            「我確認」時唯一需要判斷的東西（設計 §2.2：會擋住人的限制要在他撞上
            之前看得見）。伺服器自己的說法走 `notes[]`；這一句**無條件**印，
            因為 fail-open 到什麼都不說正是這一格原本的狀態。
          */}
          <p className="note" data-role="teaching">
            Token 上限能跑幾輪，取決於每一輪的工具呼叫次數——每次工具結果回填都要重送整個前綴，
            所以同樣的 300K input，工具密集的 Run 大約只夠 5 輪，純對話大約夠 15 輪。
          </p>
        </dd>

        {/*
          Every field inside summary_hash has to be readable here, or the user
          is re-confirming things they have never seen (設計 §1 原則 3). These
          are the rest of them — collapsed because they are not what a reader
          came to check, present because they are part of what is being agreed
          to. Narrowing the hash instead would be the unsafe direction.
        */}
        <dt>進階限制與 Provider 細節</dt>
        <dd>
          <details>
            <summary>展開其餘一併確認的欄位</summary>
            <ul>
              <li>行程數上限：{limit(summary.resource_limits.max_pids, count)}</li>
              <li>開檔數上限：{limit(summary.resource_limits.max_open_files, count)}</li>
              <li>
                產出檔案總量上限：{ceiling(summary.resource_limits.artifact_total_bytes)}、單檔{" "}
                {ceiling(summary.resource_limits.artifact_file_bytes)}
              </li>
              <li>
                軟性時間上限：{limit(summary.resource_limits.wall_clock_soft_seconds, seconds)}
                （先要求收尾; 硬性上限{" "}
                {limit(summary.resource_limits.wall_clock_hard_seconds, seconds)}才是強制中止）
              </li>
              <li>Provider 是否 rootless：{summary.provider.rootless ? "是" : "否"}</li>
              {/* 設計 §2.9 的「未測量」:沒有值不等於沒有 runtime。 */}
              <li>Runtime：{summary.provider.runtime ?? "未測量"}</li>
              <li>Runtime 版本：{summary.provider.runtime_version ?? "未測量"}</li>
            </ul>
          </details>
        </dd>
      </dl>

      {/*
        設計 §4.3: 這幾句是「平台對這一頁講的話」——notice 的定義本身。原本掛在
        一個沒有任何 CSS 規則的 .preflight-notes 上,渲染成瀏覽器預設的項目符號
        清單,看起來像資料的一部分而不是平台的自述。改吃既有的 .notice。
      */}
      {notes.map((n) => (
        <p key={n} className="notice">
          {n}
        </p>
      ))}

      {message && <p role="alert">{message}</p>}

      {runId ? (
        <p>
          {/* 設計 §2.6「平面答案優先，識別符折疊」。這一頁此刻唯一的出路，字面
              以前是一串 36 字元的十六進位——長得像一個資料欄位而不像一個出口。
              id 留著（它是這次 Run 的永久識別，鐵律 10），但排在答案後面。

              設計 §4.6.3（ADR-064）：它是出路，不是 `.action`。填色留給「開始這次
              Run」那一顆——這一頁的工作是「看完權限、決定要不要跑」，而這條連結是
              那件事**做完之後**的交接。文字與目的地一字未改。 */}
          已開始 Run。{" "}
          <Link to="/runs/$runId" params={{ runId }}>
            查看這次 Run 的結果
          </Link>
          <span className="note">
            Run ID：<code>{runId}</code>
          </span>
        </p>
      ) : (
        // 設計 §4.6.3（ADR-064）：授權確認**可以**是主要動作——§2.2 引用的 VS Code
        // 實測正是「被動、不顯眼的授權提示會失敗」。這一頁的工作是「看完權限、決定
        // 要不要跑」，完成它的就是這一顆。
        <button
          type="button"
          className="action"
          disabled={confirmAndRun.isPending}
          onClick={() => confirmAndRun.mutate(hash)}
        >
          {/* 設計 §2.4：停用要說原因，而這裡的原因（正在送出）以前一個字都沒有——
              按鈕只是變灰。這顆按鈕會依序發兩個請求（confirmPreflight 然後
              startRun），是整個發動側唯二真的產生副作用的按鈕之一。同一個 app 裡
              另外十一顆按鈕全部不是這樣：建立中…、儲存中…、Fork 中…。 */}
          {confirmAndRun.isPending ? "開始中…" : "我確認以上權限,開始 Run"}
        </button>
      )}
      <p>不同意就不要按下按鈕:未確認的 Run 不會被建立。</p>
    </>,
  );
}
