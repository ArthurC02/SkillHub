import type { RunComparison } from "../../evaluation.service";
import { VersionDiff } from "../../components/VersionDiff";
import { CriterionMatrixTable } from "./CriterionMatrixTable";
import { RunStatusTable } from "./RunStatusTable";
import "./ComparisonTables.css";

export function ComparisonTables({ data }: { data: RunComparison }) {
  const [left, right] = data.runs;
  const sides = [left, right];

  return (
    <>
      <h2>任務判定與執行狀態</h2>
      <RunStatusTable sides={sides} />

      <h2>逐條驗收條件</h2>
      <CriterionMatrixTable criterionMatrix={data.criterion_matrix} sides={sides} />

      <h2>Skill 版本差異</h2>
      {data.version_diff_url ? (
        <VersionDiff url={data.version_diff_url} />
      ) : (
        <p>兩次 Run 使用同一個版本，或分屬不同 Skill，沒有版本差異可看。</p>
      )}
    </>
  );
}
