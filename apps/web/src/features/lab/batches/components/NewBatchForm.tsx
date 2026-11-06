import { useId, useState } from "react";
import { CreateEvaluationBatchBodyTrialsMaximum as MAX_TRIALS } from "@skillhub/api-client-ts/limits";
import { useTestCases } from "../../testcases.service";
import { useCreateEvaluationBatch } from "../batches.service";
import { batchBlockedBy, runsToStart } from "../batches.model";
import { TestCasePicker } from "./TestCasePicker";
import { CreateOutcome } from "./CreateOutcome";

const DEFAULT_TRIALS = 3;

export type BatchChoice = { testCaseIds: string[]; trials: number; withheld: boolean };

export function NewBatchForm({
  skillId,
  versionId,
  from,
  onCreated,
}: {
  skillId: string;
  versionId: string;
  from?: BatchChoice;
  onCreated: (batchId: string) => void;
}) {
  const testCases = useTestCases(skillId);
  const create = useCreateEvaluationBatch(skillId);
  const [chosen, setChosen] = useState<string[]>(from?.testCaseIds ?? []);
  const [trials, setTrials] = useState(from?.trials ?? DEFAULT_TRIALS);
  const [withheld, setWithheld] = useState(from?.withheld ?? false);
  const reasonId = useId();

  const names = new Map(
    (testCases.data?.pages.flatMap((p) => p.test_cases) ?? []).map((row) => [
      row.test_case_id.toLowerCase(),
      row.name,
    ]),
  );
  const blockedBy = batchBlockedBy(chosen.length, trials);
  const edit =
    <T,>(set: (value: T) => void) =>
    (value: T) => {
      create.reset();
      set(value);
    };

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (blockedBy || create.isPending) return;
        create.mutate(
          {
            skill_version_id: versionId,
            test_case_ids: chosen,
            trials,
            with_skill_withheld: withheld,
          },
          { onSuccess: (batch) => onCreated(batch.id) },
        );
      }}
    >
      <TestCasePicker
        testCases={testCases}
        chosen={chosen}
        onToggle={(id, on) =>
          edit(setChosen)(on ? [...chosen, id] : chosen.filter((x) => x !== id))
        }
      />
      <p>
        <label>
          每題跑幾次{" "}
          <input
            type="number"
            min={1}
            max={MAX_TRIALS}
            value={Number.isNaN(trials) ? "" : trials}
            onChange={(e) => edit(setTrials)(e.target.valueAsNumber)}
          />
        </label>
      </p>
      <p>
        <label>
          <input
            type="checkbox"
            checked={withheld}
            onChange={(e) => edit(setWithheld)(e.target.checked)}
          />{" "}
          每次都加跑一次不載入小工具的對照
        </label>
      </p>

      <p role="status" data-role="evidence">
        {blockedBy ??
          `會開 ${runsToStart(chosen.length, trials, withheld)} 次試跑，每次都照一般試跑扣點並計入試跑次數；點數或次數不夠時批次會暫停，補足後可以繼續，已經跑過的不會重跑。`}
      </p>

      <CreateOutcome error={create.error} skillId={skillId} versionId={versionId} names={names} />

      <button
        type="submit"
        className="action"
        aria-disabled={Boolean(blockedBy) || create.isPending}
        aria-describedby={blockedBy ? reasonId : undefined}
      >
        {create.isPending ? "開始中…" : "開始評估批次"}
      </button>
      {blockedBy && (
        <span id={reasonId} hidden>
          {blockedBy}
        </span>
      )}
    </form>
  );
}
