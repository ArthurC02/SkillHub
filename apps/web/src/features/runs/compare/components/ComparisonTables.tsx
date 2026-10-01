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
      <p className="note table-scroll-hint">左右捲動查看全部欄位。</p>
      <RunStatusTable sides={sides} />

      <h2>逐條驗收條件</h2>
      <CriterionMatrixTable criterionMatrix={data.criterion_matrix} sides={sides} />

      <h2>小工具版本差異</h2>
      {data.version_diff_url ? (
        <VersionDiff url={data.version_diff_url} />
      ) : (
        <p>兩次試跑使用同一個版本，或分屬不同小工具，沒有版本差異可看。</p>
      )}
    </>
  );
}
