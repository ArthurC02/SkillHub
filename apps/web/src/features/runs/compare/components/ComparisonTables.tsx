import type { ComparisonSide, RunComparison } from "../../evaluation.service";
import { VersionDiff } from "../../components/VersionDiff";
import { CRITERION_LABEL } from "../../evaluation/evaluation.model";
import { runStatusLabel } from "../../runs.model";
import { verdictCell } from "./ComparisonLead";
import { RerunCell } from "./RerunCell";

function costNote(side: ComparisonSide): string {
  return `${side.cost.is_lower_bound ? "這是下界，不是總額。" : ""}權威來源：${
    side.cost.authoritative_source
  }`;
}

function credits(value: number | null): string {
  return value === null ? "未測量" : `${value} 點`;
}

const SIDE_LABEL = ["這一邊", "另一邊"];

export function ComparisonTables({ data }: { data: RunComparison }) {
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
