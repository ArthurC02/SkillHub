import type { ComparisonSide } from "../../evaluation.service";
import { runStatusLabel } from "../../runs.model";
import { verdictCell } from "./ComparisonLead.model";
import { RerunCell } from "./RerunCell";
import { SIDE_LABEL, costNote, credits } from "./ComparisonTables.model";

export function RunStatusTable({ sides }: { sides: ComparisonSide[] }) {
  const sharedCostNote = costNote(sides[0]) === costNote(sides[1]) ? costNote(sides[0]) : null;

  return (
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
              <td key={s.run_id}>
                {s.final_output ? (
                  <pre className="run-comparison-output">{s.final_output}</pre>
                ) : (
                  "無"
                )}
              </td>
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
  );
}
