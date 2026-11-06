import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { EvaluationBatch } from "../batches.model";
import { STATUS_LABEL, haltSentence } from "../batches.model";
import { ResumeBatch } from "./ResumeBatch";

export function BatchState({
  batch,
  versionNumber,
  skillId,
  batchId,
}: {
  batch: EvaluationBatch;
  versionNumber: number | undefined;
  skillId: string;
  batchId: string;
}) {
  const halted = haltSentence(batch);
  return (
    <>
      <p className="badge-row">
        <span className="badge">{STATUS_LABEL[batch.status]}</span>
        <span className="badge">
          {versionNumber === undefined ? "版本編號未知" : `v${versionNumber}`}
        </span>
        <span className="badge">每題 {batch.trials} 次</span>
      </p>
      <p className="note">
        建立於 <Timestamp at={batch.created_at} />
      </p>
      {halted && (
        <p className="notice" data-role="reason">
          {halted}
        </p>
      )}
      {batch.status === "paused" && <ResumeBatch skillId={skillId} batchId={batchId} />}
    </>
  );
}
