import type { RunComparison } from "../../evaluation.service";
import { verdictCell } from "./ComparisonLead.model";

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
