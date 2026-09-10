import { Loading } from "../components/Loading";
import { Timestamp } from "../components/Timestamp";
import { LoginRequired, ReadFailure, unauthenticated } from "../components/LoginRequired";
import { useMe } from "../api/me";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useRunComparison, useVersionDiff } from "../api/evaluation";
import type { ComparisonSide, RunComparison } from "../api/evaluation";
import { useRun, useRuns } from "../api/runs";
import { RunVerdict } from "../components/RunVerdict";
import { Reveal } from "../components/Reveal";
import { CRITERION_LABEL, OVERALL_LABEL, runStatusLabel } from "./RunEvaluation";

function verdictCell(side: ComparisonSide) {
  if (!side.evaluation) return "未評估（不是通過）";
  if (side.evaluation.status === "failed") return "評估未完成";
  if (side.evaluation.status === "pending") return "評估進行中";
  return OVERALL_LABEL[side.evaluation.overall] ?? side.evaluation.overall;
}

function costNote(side: ComparisonSide): string {
  return `${side.cost.is_lower_bound ? "這是下界，不是總額。" : ""}權威來源：${
    side.cost.authoritative_source
  }`;
}

function credits(value: number | null): string {
  return value === null ? "未測量" : `${value} 點`;
}

export function RunCompare() {
  const { runId } = useParams({ from: "/runs/$runId/compare" });
  const { against = "" } = useSearch({ strict: false }) as { against?: string };
  const [draft, setDraft] = useState(against);
  useEffect(() => setDraft(against), [against]);
  const navigate = useNavigate();
  const comparison = useRunComparison(runId, against);
  const me = useMe();
  const loggedOut = unauthenticated(me.error);

  const self = useRun(runId);
  const testCaseId = self.data?.test_case_id;
  const siblings = useRuns(testCaseId, Boolean(testCaseId));
  const candidates = testCaseId
    ? (siblings.data?.pages.flatMap((p) => p.runs) ?? []).filter((r) => r.run_id !== runId)
    : [];

  const pick = (id: string) =>
    void navigate({ to: "/runs/$runId/compare", params: { runId }, search: { against: id } });

  const pickForm = (
    <>
      {self.isPending ? (
        <Loading what="目前這次 Run" />
      ) : self.error ? (
        <ReadFailure error={self.error} what="目前這次 Run" />
      ) : !testCaseId ? (
        <p>這次 Run 的 Test Case 已無法解析，因此無法列出同一個 Test Case 的其他 Run。</p>
      ) : siblings.isPending ? (
        <Loading what="可比較的 Run" />
      ) : siblings.error ? (
        <ReadFailure error={siblings.error} what="可比較的 Run" />
      ) : candidates.length > 0 ? (
        <ul className="download-list">
          {candidates.map((r) => (
            <li key={r.run_id} className="download-item">
              <p className="badge-row">
                <RunVerdict verdict={r.evaluation} />
              </p>
              <p className="badge-row">
                <span className="badge">執行狀態：{runStatusLabel(r.status)}</span>
              </p>
              {r.status_reason && <p className="note">{r.status_reason}</p>}
              <p>
                <button type="button" onClick={() => pick(r.run_id)}>
                  與這一次比較（建立於 <Timestamp at={r.created_at} />）
                </button>
              </p>
            </li>
          ))}
        </ul>
      ) : (
        <p>這個 Test Case 目前只有這一次 Run，沒有同一個 Test Case 的其他 Run 可選。</p>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          pick(draft);
        }}
      >
        <label htmlFor="against">要比較的另一個 Run ID</label>{" "}
        <input
          id="against"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          size={40}
          placeholder="另一個 Run 的平台 run_id"
        />{" "}
        <button type="submit">比較</button>
        <p className="note">
          {(candidates.length > 0 ? "從上面選一個同一個 Test Case 的 Run，或" : "") +
            "輸入另一個 Run 的 ID 後開始比較。別的 Test Case 或別的 Skill 的 Run 也可以。"}
        </p>
      </form>
    </>
  );

  return (
    <section>
      <h1>Run 比較</h1>

      {comparison.data && <ComparisonLead data={comparison.data} />}

      <details>
        <summary>進階資訊（Run 識別碼）</summary>
        <ul>
          <li>
            這一邊：<code>{runId}</code>
          </li>
          <li>另一邊：{against === "" ? "尚未選擇" : <code>{against}</code>}</li>
        </ul>
      </details>

      {loggedOut ? (
        <LoginRequired what="Run 比較" />
      ) : comparison.data ? (
        <details>
          <summary>換一個要比較的 Run</summary>
          {pickForm}
        </details>
      ) : (
        pickForm
      )}

      {comparison.isPending && against !== "" && <Loading what="比較" />}
      <ReadFailure error={comparison.error} what="比較結果">
        <p role="alert">無法比較：{comparison.error?.message}</p>
      </ReadFailure>
      {comparison.data && <ComparisonTables data={comparison.data} />}

      <p className="note">
        <Link to="/runs/$runId" params={{ runId }}>
          回到這個 Run 的詳情
        </Link>
      </p>
    </section>
  );
}

function ComparisonLead({ data }: { data: RunComparison }) {
  const [left, right] = data.runs;
  const leftVerdict = verdictCell(left);
  const rightVerdict = verdictCell(right);

  return (
    <p className="verdict">
      任務判定：這一邊<strong>{leftVerdict}</strong>，另一邊<strong>{rightVerdict}</strong>
      {leftVerdict === rightVerdict ? "——兩邊相同。" : "——兩邊不同。"}
    </p>
  );
}

const SIDE_LABEL = ["這一邊", "另一邊"];

function ComparisonTables({ data }: { data: RunComparison }) {
  const [left, right] = data.runs;
  const sides = [left, right];
  const sharedCostNote = costNote(left) === costNote(right) ? costNote(left) : null;

  return (
    <>
      <h2>任務判定與執行狀態</h2>
      <div className="table-scroll" tabIndex={0}>
        <table className="compare-table">
          <caption>Run 任務判定與執行狀態對比</caption>
          <thead>
            <tr>
              <th scope="col">項目</th>
              {sides.map((s, i) => (
                <th key={s.run_id} scope="col">
                  {SIDE_LABEL[i]}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              <th scope="row">任務判定</th>
              {sides.map((s) => (
                <td key={s.run_id}>{verdictCell(s)}</td>
              ))}
            </tr>
            <tr>
              <th scope="row">執行狀態</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {runStatusLabel(s.status)}（<code>{s.status}</code>）
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Skill 版本</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  <details>
                    <summary>版本 ID</summary>
                    <code>{s.skill_version_id}</code>
                  </details>
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">最終輸出</th>
              {sides.map((s) => (
                <td key={s.run_id}>{s.final_output ? <pre>{s.final_output}</pre> : "無"}</td>
              ))}
            </tr>
            <tr>
              <th scope="row">錯誤</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {s.errors && s.errors.length > 0 ? (
                    <ul>
                      {s.errors.map((e, i) => (
                        <li key={`${e.code ?? ""}-${i}`}>
                          [{e.category ?? "?"}/{e.code ?? "?"}] {e.message}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    "無"
                  )}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">延遲</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {s.duration_ms === undefined ? "未開始執行" : `${s.duration_ms} 毫秒`}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">
                Run 用掉的點數（下界）
                {sharedCostNote && <p className="note">{sharedCostNote}</p>}
              </th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {credits(s.cost.credits)}
                  {sharedCostNote ? null : <p className="note">{costNote(s)}</p>}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">
                評估用掉的點數
                <p className="note">與上一列分開列，不相加。</p>
              </th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {s.evaluation ? credits(s.evaluation.cost.evaluation_credits) : "未評估"}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">輸入是否仍在</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  <RerunCell side={s} />
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </div>

      <h2>逐條驗收條件</h2>
      {data.criterion_matrix.length === 0 ? (
        <p>沒有可對照的驗收條件。</p>
      ) : (
        <div className="table-scroll" tabIndex={0}>
          <table className="compare-table">
            <caption>驗收條件判定矩陣對比</caption>
            <thead>
              <tr>
                <th scope="col">驗收條件</th>
                {sides.map((s, i) => (
                  <th key={s.run_id} scope="col">
                    {SIDE_LABEL[i]}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.criterion_matrix.map((row) => (
                <tr key={row.criterion_id}>
                  <th scope="row">{row.text}</th>
                  {row.results.map((r) => (
                    <td key={r.run_id}>
                      {r.result === null ? (
                        <span className="compare-unknown">未評估</span>
                      ) : (
                        CRITERION_LABEL[r.result]
                      )}
                      {r.source === "model" ? <p className="note">模型評估</p> : null}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <h2>Skill 版本差異</h2>
      {data.version_diff_url ? (
        <VersionDiff url={data.version_diff_url} />
      ) : (
        <p>兩次 Run 使用同一個版本，或分屬不同 Skill，沒有版本差異可看。</p>
      )}
    </>
  );
}

function RerunCell({ side }: { side: ComparisonSide }) {
  if (!side.inputs_available) {
    return <>已刪除或已過期，無法以相同輸入重跑；比較內容本身不受影響。</>;
  }
  if (!side.test_case_id) {
    return <>仍在。可用同一個 Test Case 重新試跑，仍須通過執行前權限確認。</>;
  }
  return (
    <>
      仍在。{" "}
      <Link
        to="/lab/run"
        search={{
          skill: side.skill_id,
          version: side.skill_version_id,
          test_case: side.test_case_id,
        }}
      >
        以相同的 Test Case 與版本重新試跑
      </Link>
      （會先經過權限確認）
    </>
  );
}

export function VersionDiff({ url }: { url: string }) {
  const diff = useVersionDiff(url);
  if (diff.isPending) return <Loading what="版本差異" />;
  if (diff.error) return <ReadFailure error={diff.error} what="版本差異" />;
  if (diff.data.files.length === 0) return <p>兩個版本的檔案內容相同。</p>;

  return (
    <ul className="file-tree">
      {diff.data.files.map((f) => (
        <li key={f.path}>
          <code>{f.path}</code> · {f.status}
          {f.diff ? (
            <pre className="diff">
              <Reveal text={f.diff} />
            </pre>
          ) : (
            <p className="note">（二進位或過大，不顯示差異）</p>
          )}
        </li>
      ))}
    </ul>
  );
}
