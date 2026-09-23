import { Loading } from "../../../shared/ui/Loading";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { LoginRequired, ReadFailure, unauthenticated } from "../../../shared/ui/LoginRequired";
import { SkillVersionPicker } from "../../skill";
import { bytes } from "../../../shared/format";
import { useMe } from "../../../core/session/me.service";
import { Link, useSearch } from "@tanstack/react-router";
import { useState, type ReactNode } from "react";
import { useConfirmAndStartRun, usePreflight } from "../lab.service";
import { useOwnSkills } from "../../skill";
import { useTestCase } from "../testcases.service";
import {
  ceiling,
  limit,
  count,
  seconds,
  tokens,
  SCRIPT_LABEL,
  BLOCKED_SENTENCE,
  startFailureSentence,
} from "./preflight.model";

type LabSearch = { skill?: string; version?: string; test_case?: string };

const isolationLabels: Record<string, string> = {
  strong: "強（獨立核心）",
  weak: "弱（與主機共用核心）",
  none: "無（沒有隔離邊界）",
};

function isolationText(strength: string) {
  return isolationLabels[strength] ?? strength;
}

export function RunPreflight() {
  const {
    skill = "",
    version: linkedVersion = "",
    test_case: testCase = "",
  } = useSearch({ strict: false }) as LabSearch;
  // Search params change without remounting the route; the key starts a fresh form.
  return (
    <Preflight
      key={`${skill}|${linkedVersion}|${testCase}`}
      skill={skill}
      linkedVersion={linkedVersion}
      testCase={testCase}
    />
  );
}

function Preflight({
  skill,
  linkedVersion,
  testCase,
}: {
  skill: string;
  linkedVersion: string;
  testCase: string;
}) {
  const [picked, setPicked] = useState("");
  const version = picked || linkedVersion;
  const ready = skill !== "" && testCase !== "";
  const me = useMe();
  const testCaseInfo = useTestCase(testCase);
  const ownSkills = useOwnSkills();
  const preflight = usePreflight(skill, version, testCase, ready && version !== "");
  const start = useConfirmAndStartRun(skill, version, testCase);
  const runId = start.data?.run_id ?? "";
  const message = startFailureSentence(start.error);

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

  const skillName = ownSkills.data?.skills.find((sk) => sk.skill_id === skill)?.name;
  const criteria = testCaseInfo.data?.acceptance_criteria.length;

  const shell = (children: ReactNode) => (
    <section>
      <h1>執行前權限確認</h1>
      <p>
        Skill：
        <strong>
          {skillName ??
            (ownSkills.isPending ? "讀取中…" : ownSkills.error ? "讀取失敗" : "不在你的清單裡")}
        </strong>
        {" ・ "}
        Test Case：
        <strong>
          {testCaseInfo.data?.name ??
            (testCaseInfo.isPending ? "讀取中…" : testCaseInfo.error ? "讀取失敗" : "讀不到名稱")}
        </strong>
      </p>
      {ownSkills.error && <ReadFailure error={ownSkills.error} what="你的 Skill 清單" />}
      {testCaseInfo.error && <ReadFailure error={testCaseInfo.error} what="Test Case" />}
      {criteria === 0 && (
        <p className="note">
          這個 Test Case 沒有驗收條件，所以這次 Run 不會產生逐條判定。試跑本身照常執行。
        </p>
      )}
      <SkillVersionPicker
        skillId={skill}
        value={version}
        onPick={(id) => {
          setPicked(id);
          start.reset();
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

  const {
    summary,
    summary_hash: hash,
    estimated_cost: cost,
    quota,
    blocked,
    notes,
  } = preflight.data;

  return shell(
    <>
      <p>以下是這次 Run 可以接觸的範圍。確認後才會開始執行。</p>

      <dl data-role="evidence">
        <dt>預估點數（估計值）</dt>
        <dd>
          {cost ? (
            <>
              {cost.low_credits} – {cost.high_credits} 點（常見約 {cost.typical_credits} 點）
              <p>{cost.basis}</p>
            </>
          ) : (
            <>未測量——這個伺服器版本沒有回報預估點數，不代表這次 Run 不用點。</>
          )}
        </dd>
        {quota && (
          <>
            <dt>剩餘試跑額度</dt>
            <dd>
              今天還可以跑 {quota.remaining_today} 次、這個週期還可以跑 {quota.remaining_window}{" "}
              次。 額度下一次增加不會早於 <Timestamp at={quota.window_resets_at} />。
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

        <dt>工具</dt>
        <dd>{summary.tools.length === 0 ? "無（沒有授予任何工具）" : summary.tools.join("、")}</dd>

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
          {summary.provider.isolation_strength &&
            `（隔離:${isolationText(summary.provider.isolation_strength)}）`}
        </dd>

        <dt>資源上限</dt>
        <dd>
          vCPU {limit(summary.resource_limits.vcpu, count)}、記憶體{" "}
          {ceiling(summary.resource_limits.memory_bytes)}、 磁碟{" "}
          {ceiling(summary.resource_limits.disk_bytes)}、 時間上限{" "}
          {limit(summary.resource_limits.wall_clock_hard_seconds, seconds)}、 Token{" "}
          {limit(summary.resource_limits.token_budget?.max_input_tokens, tokens)} 進 /{" "}
          {limit(summary.resource_limits.token_budget?.max_output_tokens, tokens)} 出
          <p className="note" data-role="teaching">
            Token 上限能跑幾輪，取決於每一輪的工具呼叫次數——每次工具結果回填都要重送整個前綴，
            所以同樣的 300K input，工具密集的 Run 大約只夠 5 輪，純對話大約夠 15 輪。
          </p>
        </dd>

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
              <li>Runtime：{summary.provider.runtime ?? "未測量"}</li>
              <li>Runtime 版本：{summary.provider.runtime_version ?? "未測量"}</li>
            </ul>
          </details>
        </dd>
      </dl>

      {notes.map((n) => (
        <p key={n} className="notice">
          {n}
        </p>
      ))}

      {unauthenticated(start.error) && <ReadFailure error={start.error} what="Run" />}
      {message && <p role="alert">{message}</p>}

      {runId ? (
        <p>
          已開始 Run。{" "}
          <Link to="/runs/$runId" params={{ runId }}>
            查看這次 Run 的結果
          </Link>
          <span className="note">
            Run ID：<code>{runId}</code>
          </span>
        </p>
      ) : blocked ? (
        <p role="alert" className="notice">
          {BLOCKED_SENTENCE[blocked]}
        </p>
      ) : (
        <>
          <p className="note">平台目前只讓有封測邀請的帳號開始 Run。</p>
          <button
            type="button"
            className="action"
            disabled={start.isPending}
            onClick={() => start.mutate(hash)}
          >
            {start.isPending ? "開始中…" : "我確認以上權限,開始 Run"}
          </button>
        </>
      )}
      <p>不同意就不要按下按鈕:未確認的 Run 不會被建立。</p>
    </>,
  );
}
