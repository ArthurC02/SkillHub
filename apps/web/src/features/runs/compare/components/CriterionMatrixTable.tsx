import type { ComparisonSide, RunComparison } from "../../evaluation.service";
import { CRITERION_LABEL } from "../../evaluation/evaluation.model";
import { SIDE_LABEL } from "./ComparisonTables.model";

export function CriterionMatrixTable({
  criterionMatrix,
  sides,
}: {
  criterionMatrix: RunComparison["criterion_matrix"];
  sides: ComparisonSide[];
}) {
  if (criterionMatrix.length === 0) return <p>沒有可對照的驗收條件。</p>;

  return (
    <div
      className="table-scroll"
      role="region"
      aria-label="驗收條件比較表，可左右捲動"
      tabIndex={0}
    >
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
          {criterionMatrix.map((row) => (
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
  );
}
