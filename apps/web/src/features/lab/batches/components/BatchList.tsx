import { Link } from "@tanstack/react-router";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { versionNumberWord, type VersionsRead } from "../../../skill";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { useEvaluationBatches } from "../batches.service";
import { STATUS_LABEL, type EvaluationBatch } from "../batches.model";

export function BatchList({
  skillId,
  versionNumbers,
  versionsRead,
  newest,
}: {
  skillId: string;
  versionNumbers: Map<string, number>;
  versionsRead: VersionsRead;
  newest?: { version_id: string; version_number: number };
}) {
  const batches = useEvaluationBatches(skillId);
  const busy = batches.fetchStatus !== "idle";
  const rows = batches.error ? undefined : batches.data;
  const toRerun = newest && rows && latestBatchOnAnotherVersion(rows, newest.version_id);
  const rerunNumber = toRerun && versionNumbers.get(toRerun.skill_version_id.toLowerCase());

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
      {newest && toRerun && (
        <p role="status">
          v{newest.version_number} 還沒有評估批次。{" "}
          <Link
            to="/skills/$skillId/evaluation-batches"
            params={{ skillId }}
            search={{ version: newest.version_id, rerun: toRerun.id }}
          >
            用{rerunNumber === undefined ? "" : ` v${rerunNumber} `}最近一批的題目重跑 v
            {newest.version_number}
          </Link>
        </p>
      )}
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
                      {versionNumberWord(number, versionsRead)}・每題 {batch.trials} 次
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

function latestBatchOnAnotherVersion(batches: EvaluationBatch[], newestVersionId: string) {
  const newest = newestVersionId.toLowerCase();
  if (batches.some((b) => b.skill_version_id.toLowerCase() === newest)) return undefined;
  return batches.reduce<EvaluationBatch | undefined>(
    (latest, b) => (latest === undefined || b.created_at > latest.created_at ? b : latest),
    undefined,
  );
}
