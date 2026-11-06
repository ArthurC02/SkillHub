import { Link, useNavigate } from "@tanstack/react-router";
import { ApiError } from "../../../../core/api/client";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { formatAt } from "../../../../shared/ui/Timestamp.model";
import { useSkillVersions } from "../../../skill";
import { useEvaluationBatchComparison, useEvaluationBatches } from "../batches.service";
import { PairedSummary } from "./PairedSummary";
import { pairedFromComparison } from "../batches.model";

const HEADING = "batch-comparison-heading";

export function BatchComparison({
  skillId,
  batchId,
  against,
}: {
  skillId: string;
  batchId: string;
  against: string;
}) {
  const navigate = useNavigate();
  const batches = useEvaluationBatches(skillId);
  const versions = useSkillVersions(skillId);
  const comparison = useEvaluationBatchComparison(skillId, batchId, against);
  const numbers = new Map(
    (versions.error ? [] : (versions.data?.versions ?? [])).map((v) => [
      v.version_id.toLowerCase(),
      v.version_number,
    ]),
  );
  const others = (batches.error ? [] : (batches.data ?? [])).filter(
    (b) => b.id.toLowerCase() !== batchId.toLowerCase(),
  );
  const result = comparison.error ? undefined : comparison.data;
  const unusable =
    comparison.error instanceof ApiError &&
    (comparison.error.status === 404 || comparison.error.status === 400);

  return (
    <section aria-labelledby={HEADING}>
      <h2 id={HEADING} tabIndex={-1}>
        和另一個批次逐題比較
      </h2>
      {batches.isPending && (
        <Loading what="可比較的批次" paused={batches.fetchStatus === "paused"} />
      )}
      <ReadFailure
        error={batches.error}
        what="可比較的批次"
        retry={() => void batches.refetch()}
        retrying={batches.fetchStatus !== "idle"}
        focusTarget={HEADING}
      />
      {batches.data && !batches.error && others.length === 0 && (
        <p role="status">這個小工具還沒有其他評估批次可以比較。</p>
      )}
      {others.length > 0 && (
        <p>
          <label>
            比較基準{" "}
            <select
              value={against}
              onChange={(e) =>
                void navigate({
                  to: "/skills/$skillId/evaluation-batches/$batchId",
                  params: { skillId, batchId },
                  search: { against: e.target.value || undefined },
                })
              }
            >
              <option value="">請選擇批次…</option>
              {others.map((b) => {
                const number = numbers.get(b.skill_version_id.toLowerCase());
                return (
                  <option key={b.id} value={b.id}>
                    {number === undefined ? "版本編號未知" : `v${number}`}・每題 {b.trials} 次・
                    {formatAt(b.created_at)}
                  </option>
                );
              })}
            </select>
          </label>
        </p>
      )}
      {against !== "" && (
        <>
          {comparison.isPending && (
            <Loading what="比較結果" paused={comparison.fetchStatus === "paused"} />
          )}
          <ReadFailure
            error={comparison.error}
            what="比較結果"
            retry={unusable ? undefined : () => void comparison.refetch()}
            retrying={comparison.fetchStatus !== "idle"}
            focusTarget={HEADING}
          >
            {unusable && (
              <p role="alert">
                這兩個批次不能比較：比較基準不存在，或不屬於這個小工具。請換一個批次。
              </p>
            )}
          </ReadFailure>
          {result && (
            <>
              <PairedSummary
                heading="這個批次減去比較基準"
                nested
                difference={pairedFromComparison(result)}
                betterLabel="這個批次比較常通過"
                worseLabel="比較基準比較常通過"
              />
              <p>
                <Link
                  to="/skills/$skillId/evaluation-batches/$batchId"
                  params={{ skillId, batchId: against }}
                  search={{ against: undefined }}
                >
                  開啟比較基準的批次結果
                </Link>
              </p>
            </>
          )}
        </>
      )}
    </section>
  );
}
