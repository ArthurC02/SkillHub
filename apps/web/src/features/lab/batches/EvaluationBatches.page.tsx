import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { SkillVersionPicker, SkillWorkspaceNav, lineNewest, useSkillVersions } from "../../skill";
import { useBatchReflections, useEvaluationBatchReport } from "./batches.service";
import { heldOutTestCaseIds } from "./reflection.model";
import { NewBatchForm, type BatchChoice } from "./components/NewBatchForm";
import { BatchList } from "./components/BatchList";

export function EvaluationBatches() {
  const { skillId } = useParams({ from: "/skills/$skillId/evaluation-batches" });
  const {
    version = "",
    rerun = "",
    reflection = "",
  } = useSearch({ from: "/skills/$skillId/evaluation-batches" });
  const navigate = useNavigate();
  const versions = useSkillVersions(skillId);
  const rerunSource = useRerunSource(skillId, rerun, reflection);
  const versionRows = versions.error ? [] : (versions.data?.versions ?? []);
  const versionNumbers = new Map(
    versionRows.map((v) => [v.version_id.toLowerCase(), v.version_number]),
  );
  const newest = lineNewest(versionRows);
  const { from, waiting: waitingForSource } = rerunSource;

  return (
    <section>
      <h1 id="evaluation-batches-heading" tabIndex={-1}>
        評估批次
      </h1>
      <SkillWorkspaceNav skillId={skillId} versionId={version || undefined} />
      <p>
        一次把多題測試題各跑幾次，看這個版本有多常通過；也能加跑不載入小工具的對照，看小工具帶來多少差別。
      </p>

      <section aria-labelledby="new-batch-heading">
        <h2 id="new-batch-heading" tabIndex={-1}>
          開一個評估批次
        </h2>
        {rerun !== "" && <RerunNotice source={rerunSource} versionNumbers={versionNumbers} />}
        <SkillVersionPicker
          skillId={skillId}
          value={version}
          onPick={(id) =>
            void navigate({
              to: "/skills/$skillId/evaluation-batches",
              params: { skillId },
              search: {
                version: id,
                rerun: rerun || undefined,
                reflection: reflection || undefined,
              },
            })
          }
        />
        {version !== "" && !waitingForSource && (
          <NewBatchForm
            key={version}
            skillId={skillId}
            versionId={version}
            from={from}
            onCreated={(batchId) =>
              void navigate({
                to: "/skills/$skillId/evaluation-batches/$batchId",
                params: { skillId, batchId },
                search: { against: from ? rerun : undefined },
              })
            }
          />
        )}
      </section>

      <BatchList skillId={skillId} versionNumbers={versionNumbers} newest={newest} />
    </section>
  );
}

function useHeldOut(skillId: string, batchId: string, reflectionId: string) {
  const reflections = useBatchReflections(skillId, reflectionId === "" ? "" : batchId);
  if (reflectionId === "") return { waiting: false };
  const found = reflections.data?.find(
    (r) => r.reflection_id.toLowerCase() === reflectionId.toLowerCase(),
  );
  const failure = reflections.error ? (
    <ReadFailure
      error={reflections.error}
      what="要留著檢查的題目"
      retry={() => void reflections.refetch()}
      retrying={reflections.fetchStatus !== "idle"}
    />
  ) : (
    reflections.data &&
    !found && (
      <p role="alert">找不到那一次找改進的紀錄，沒辦法只挑沒給模型看的題目；不會用整批題目代替。</p>
    )
  );
  return { reflection: found, waiting: !found, failure };
}

function useRerunSource(skillId: string, rerun: string, reflection: string) {
  const source = useEvaluationBatchReport(skillId, rerun);
  const heldOut = useHeldOut(skillId, rerun, reflection);
  const report = source.error ? undefined : source.data;
  const ids = report?.test_cases.map((row) => row.test_case_id) ?? [];
  const from: BatchChoice | undefined = report && {
    testCaseIds: heldOut.reflection ? heldOutTestCaseIds(ids, heldOut.reflection) : ids,
    trials: report.batch.trials,
    withheld: report.batch.with_skill_withheld,
  };
  const waiting = rerun !== "" && (source.isPending || heldOut.waiting);
  return { source, report, heldOut, from, waiting };
}

function RerunNotice({
  source: { source, report, heldOut, from },
  versionNumbers,
}: {
  source: ReturnType<typeof useRerunSource>;
  versionNumbers: ReadonlyMap<string, number>;
}) {
  const sourceNumber = report && versionNumbers.get(report.batch.skill_version_id.toLowerCase());
  return (
    <>
      {source.isPending && (
        <Loading what="要重跑的評估批次" paused={source.fetchStatus === "paused"} />
      )}
      <ReadFailure
        error={source.error}
        what="要重跑的評估批次"
        retry={() => void source.refetch()}
        retrying={source.fetchStatus !== "idle"}
      />
      {heldOut.failure}
      {from && (
        <p role="status">
          已照{sourceNumber === undefined ? "" : ` v${sourceNumber} `}
          那一批填好
          {heldOut.reflection ? `沒給模型看的 ${from.testCaseIds.length} 題、` : "題目、"}
          次數與對照；開出來之後會直接和那一批逐題比較。
        </p>
      )}
    </>
  );
}
