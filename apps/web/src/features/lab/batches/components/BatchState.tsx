import { Timestamp } from "../../../../shared/ui/Timestamp";
import { versionNumberWord, type VersionsRead } from "../../../skill";
import type { EvaluationBatch } from "../batches.model";
import { STATUS_LABEL, haltSentence } from "../batches.model";
import { ResumeBatch } from "./ResumeBatch";

export function BatchState({
  batch,
  versionNumber,
  versionsRead,
  skillId,
  batchId,
}: {
  batch: EvaluationBatch;
  versionNumber: number | undefined;
  versionsRead: VersionsRead;
  skillId: string;
  batchId: string;
}) {
  const halted = haltSentence(batch);
  return (
    <>
      <p className="badge-row">
        <span className="badge">{STATUS_LABEL[batch.status]}</span>
        <span className="badge">{versionNumberWord(versionNumber, versionsRead)}</span>
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
