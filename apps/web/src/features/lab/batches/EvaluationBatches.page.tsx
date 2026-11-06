import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { SkillVersionPicker, SkillWorkspaceNav, useSkillVersions } from "../../skill";
import { NewBatchForm } from "./components/NewBatchForm";
import { BatchList } from "./components/BatchList";

export function EvaluationBatches() {
  const { skillId } = useParams({ from: "/skills/$skillId/evaluation-batches" });
  const { version = "" } = useSearch({ from: "/skills/$skillId/evaluation-batches" });
  const navigate = useNavigate();
  const versions = useSkillVersions(skillId);
  const versionNumbers = new Map(
    (versions.error ? [] : (versions.data?.versions ?? [])).map((v) => [
      v.version_id.toLowerCase(),
      v.version_number,
    ]),
  );

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
        <SkillVersionPicker
          skillId={skillId}
          value={version}
          onPick={(id) =>
            void navigate({
              to: "/skills/$skillId/evaluation-batches",
              params: { skillId },
              search: { version: id },
            })
          }
        />
        {version !== "" && (
          <NewBatchForm
            key={version}
            skillId={skillId}
            versionId={version}
            onCreated={(batchId) =>
              void navigate({
                to: "/skills/$skillId/evaluation-batches/$batchId",
                params: { skillId, batchId },
                search: { against: undefined },
              })
            }
          />
        )}
      </section>

      <BatchList skillId={skillId} versionNumbers={versionNumbers} />
    </section>
  );
}
