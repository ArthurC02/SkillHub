import type { ComparisonSide, RunComparison } from "../../evaluation.service";
import { OVERALL_LABEL } from "../../evaluation/evaluation.model";

export function verdictCell(side: ComparisonSide) {
  if (!side.evaluation) return "未評估（不是通過）";
  if (side.evaluation.status === "failed") return "評估未完成";
  if (side.evaluation.status === "pending") return "評估進行中";
  return OVERALL_LABEL[side.evaluation.overall] ?? side.evaluation.overall;
}

export function ComparisonLead({ data }: { data: RunComparison }) {
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
