import type { PairedDifference } from "../batches.model";
import { signedPoints, verdictSentence } from "../batches.model";

export function PairedSummary({
  heading,
  nested = false,
  difference,
  betterLabel,
  worseLabel,
}: {
  heading: string;
  nested?: boolean;
  difference: PairedDifference;
  betterLabel: string;
  worseLabel: string;
}) {
  return (
    <section aria-label={heading}>
      {nested ? <h3>{heading}</h3> : <h2>{heading}</h2>}
      <dl data-role="evidence">
        <dt>結論</dt>
        <dd>{verdictSentence(difference.verdict, betterLabel, worseLabel)}</dd>
        <dt>平均差距</dt>
        <dd>
          {difference.difference === undefined
            ? "還沒有兩邊都判定過的測試題"
            : signedPoints(difference.difference)}
          {difference.difference_95 &&
            `（95% 信賴區間 ${signedPoints(difference.difference_95.low)}～${signedPoints(difference.difference_95.high)}）`}
          {difference.difference !== undefined &&
            !difference.difference_95 &&
            "（少於兩題可以配對，算不出信賴區間）"}
        </dd>
        <dt>逐題</dt>
        <dd>
          {betterLabel} {difference.better} 題、{worseLabel} {difference.worse} 題、一樣{" "}
          {difference.tied} 題
        </dd>
        <dt>配對</dt>
        <dd>
          兩邊都判定過 {difference.paired} 題；只有一邊判定過、不算進來的 {difference.unpaired} 題
        </dd>
      </dl>
    </section>
  );
}
