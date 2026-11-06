import { Link } from "@tanstack/react-router";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { useEvaluationBatches } from "../batches.service";
import { STATUS_LABEL } from "../batches.model";

export function BatchList({
  skillId,
  versionNumbers,
}: {
  skillId: string;
  versionNumbers: Map<string, number>;
}) {
  const batches = useEvaluationBatches(skillId);
  const busy = batches.fetchStatus !== "idle";
  const rows = batches.error ? undefined : batches.data;

  return (
    <section aria-labelledby="batch-list-heading">
      <h2 id="batch-list-heading" tabIndex={-1}>
        這個小工具的評估批次
      </h2>
      {batches.isPending && <Loading what="評估批次" paused={batches.fetchStatus === "paused"} />}
      <ReadFailure
        error={batches.error}
        what="評估批次"
        retry={() => void batches.refetch()}
        retrying={busy}
        focusTarget="batch-list-heading"
      />
      {rows &&
        (rows.length === 0 ? (
          <p role="status">還沒有評估批次。</p>
        ) : (
          <ul className="download-list">
            {rows.map((batch) => {
              const number = versionNumbers.get(batch.skill_version_id.toLowerCase());
              return (
                <li key={batch.id} className="download-item">
                  <p>
                    <Link
                      to="/skills/$skillId/evaluation-batches/$batchId"
                      params={{ skillId, batchId: batch.id }}
                      search={{ against: undefined }}
                    >
                      {number === undefined ? "版本編號未知" : `v${number}`}・每題 {batch.trials} 次
                      {batch.with_skill_withheld ? "＋不載入小工具的對照" : ""}
                    </Link>
                  </p>
                  <p className="badge-row">
                    <span className="badge">{STATUS_LABEL[batch.status]}</span>
                  </p>
                  <p className="note">
                    建立於 <Timestamp at={batch.created_at} />
                  </p>
                </li>
              );
            })}
          </ul>
        ))}
    </section>
  );
}
