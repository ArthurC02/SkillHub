import { Link } from "@tanstack/react-router";
import type { ComparisonSide } from "../../evaluation.service";
import { runStatusLabel } from "../../runs.model";
import { verdictCell } from "./ComparisonLead.model";
import { RerunCell } from "./RerunCell";
import { SIDE_LABEL, costNote, credits } from "./ComparisonTables.model";

function RunStatusHeader({ sides }: { sides: ComparisonSide[] }) {
  return (
    <thead>
      <tr>
        <th scope="col">項目</th>
        {sides.map((side, index) => (
          <th key={side.run_id} scope="col">
            <Link to="/runs/$runId" params={{ runId: side.run_id }}>
              {SIDE_LABEL[index]} Run
            </Link>
          </th>
        ))}
      </tr>
    </thead>
  );
}

export function RunStatusTable({ sides }: { sides: ComparisonSide[] }) {
  const sharedCostNote = costNote(sides[0]) === costNote(sides[1]) ? costNote(sides[0]) : null;

  return (
    <div
      className="table-scroll"
      role="region"
      aria-label="Run 狀態比較表，可左右捲動"
      tabIndex={0}
    >
      <table className="compare-table">
        <caption>Run 任務判定與執行狀態對比</caption>
        <RunStatusHeader sides={sides} />
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
                <Link
                  to="/skills/$skillId/versions/$versionId"
                  params={{ skillId: s.skill_id, versionId: s.skill_version_id }}
                >
                  查看這個版本
                </Link>
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
                  "未產生"
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
                  "沒有錯誤紀錄"
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
