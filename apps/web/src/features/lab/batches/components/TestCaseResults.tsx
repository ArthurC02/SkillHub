import { Link } from "@tanstack/react-router";
import type { BatchArm, BatchTestCase } from "../batches.model";
import { OUTCOME_LABEL } from "../batches.model";

export function TestCaseResults({
  arm,
  names,
  withheld,
}: {
  arm: BatchArm;
  names: Map<string, string>;
  withheld?: BatchArm;
}) {
  const control = new Map(
    (withheld?.test_cases ?? []).map((tc) => [tc.test_case_id.toLowerCase(), tc]),
  );
  return (
    <section aria-labelledby="batch-test-cases-heading">
      <h2 id="batch-test-cases-heading">逐題結果</h2>
      <ul className="download-list">
        {arm.test_cases.map((tc) => (
          <li key={tc.test_case_id} className="download-item">
            <p>
              <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId: tc.test_case_id }}>
                {names.get(tc.test_case_id.toLowerCase()) ?? "未載入名稱的測試題"}
              </Link>
            </p>
            <p data-role="evidence">
              通過 {tc.passed} / {tc.judged} 次{tc.pass_all_k === true && "・每次都通過"}
              {tc.pass_at_k === false && "・沒有一次通過"}
              {withheld && <ControlLine control={control.get(tc.test_case_id.toLowerCase())} />}
            </p>
            {tc.prompt_varied_across && (
              <p className="note" data-role="caveat">
                批次進行中這題被改過，各次試跑看到的題目不完全一樣。
              </p>
            )}
            <TrialLinks testCase={tc} />
          </li>
        ))}
      </ul>
    </section>
  );
}

function ControlLine({ control }: { control: BatchTestCase | undefined }) {
  return control ? (
    <>
      ；對照通過 {control.passed} / {control.judged} 次
    </>
  ) : (
    <>；對照沒有這一題的紀錄</>
  );
}

function TrialLinks({ testCase }: { testCase: BatchTestCase }) {
  return (
    <ol className="badge-row">
      {testCase.trials.map((trial) => {
        const runId = trial.end_run_id ?? trial.run_id;
        const label = `第 ${trial.trial} 次：${OUTCOME_LABEL[trial.outcome]}`;
        return (
          <li key={trial.trial}>
            {runId ? (
              <Link to="/runs/$runId" params={{ runId }}>
                {label}
              </Link>
            ) : (
              <span className="badge">{label}</span>
            )}
          </li>
        );
      })}
    </ol>
  );
}
