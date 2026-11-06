import type { BatchArm } from "../batches.model";
import { percent } from "../batches.model";

export function ArmSummary({ arm, heading }: { arm: BatchArm; heading: string }) {
  return (
    <section aria-label={heading}>
      <h2>{heading}</h2>
      <dl data-role="evidence">
        <dt>通過率</dt>
        <dd>
          {arm.pass_rate === undefined ? (
            "還沒有判定完成的試跑"
          ) : (
            <>
              {percent(arm.pass_rate)}（{arm.passed} / {arm.judged} 次）
              {arm.pass_rate_95 &&
                `，95% 信賴區間 ${percent(arm.pass_rate_95.low)}～${percent(arm.pass_rate_95.high)}`}
            </>
          )}
        </dd>
        <dt>每次都通過的測試題</dt>
        <dd>
          {arm.test_cases_all_passed} 題（已有定論 {arm.test_cases_decided} /{" "}
          {arm.test_cases.length} 題）
        </dd>
      </dl>
    </section>
  );
}
