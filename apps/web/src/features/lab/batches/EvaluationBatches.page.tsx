import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { SkillVersionPicker, SkillWorkspaceNav, useSkillVersions } from "../../skill";
import { useEvaluationBatchReport } from "./batches.service";
import { NewBatchForm, type BatchChoice } from "./components/NewBatchForm";
import { BatchList } from "./components/BatchList";

export function EvaluationBatches() {
  const { skillId } = useParams({ from: "/skills/$skillId/evaluation-batches" });
  const { version = "", rerun = "" } = useSearch({ from: "/skills/$skillId/evaluation-batches" });
  const navigate = useNavigate();
  const versions = useSkillVersions(skillId);
  const source = useEvaluationBatchReport(skillId, rerun);
  const versionRows = versions.error ? [] : (versions.data?.versions ?? []);
  const versionNumbers = new Map(
    versionRows.map((v) => [v.version_id.toLowerCase(), v.version_number]),
  );
  const newest = versionRows.reduce<(typeof versionRows)[number] | undefined>(
    (best, v) => (best === undefined || v.version_number > best.version_number ? v : best),
    undefined,
  );
  const sourceReport = source.error ? undefined : source.data;
  const from: BatchChoice | undefined = sourceReport && {
    testCaseIds: sourceReport.test_cases.map((row) => row.test_case_id),
    trials: sourceReport.batch.trials,
    withheld: sourceReport.batch.with_skill_withheld,
  };
  const sourceNumber =
    sourceReport && versionNumbers.get(sourceReport.batch.skill_version_id.toLowerCase());
  const waitingForSource = rerun !== "" && source.isPending;

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
        {rerun !== "" && (
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
            {from && (
              <p role="status">
                已照{sourceNumber === undefined ? "" : ` v${sourceNumber} `}
                那一批填好題目、次數與對照；開出來之後會直接和那一批逐題比較。
              </p>
            )}
          </>
        )}
        <SkillVersionPicker
          skillId={skillId}
          value={version}
          onPick={(id) =>
            void navigate({
              to: "/skills/$skillId/evaluation-batches",
              params: { skillId },
              search: { version: id, rerun: rerun || undefined },
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
