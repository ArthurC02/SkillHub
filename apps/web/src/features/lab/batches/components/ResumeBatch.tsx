import { ApiError, userFacingReason } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { NOT_INVITED_SENTENCE } from "../../preflight/preflight.model";
import { useResumeEvaluationBatch } from "../batches.service";

function resumeFailure(error: unknown): string {
  if (error instanceof ApiError && error.status === 403) return NOT_INVITED_SENTENCE;
  if (error instanceof ApiError && error.status === 409)
    return "這個批次已經不是暫停狀態，畫面會更新成最新的狀態。";
  return userFacingReason(error) ?? "沒有繼續成功，請再試一次。";
}

export function ResumeBatch({ skillId, batchId }: { skillId: string; batchId: string }) {
  const resume = useResumeEvaluationBatch(skillId, batchId);
  return (
    <>
      <ReadFailure error={resume.error} what="評估批次">
        <p role="alert">{resumeFailure(resume.error)}</p>
      </ReadFailure>
      <button type="button" disabled={resume.isPending} onClick={() => resume.mutate()}>
        {resume.isPending ? "繼續中…" : "繼續這個批次"}
      </button>
    </>
  );
}
