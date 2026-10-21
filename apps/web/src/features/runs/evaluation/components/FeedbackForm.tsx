import { SetEvaluationFeedbackBodyCommentMaxLength } from "@skillhub/api-client-ts/limits";
import { textLengthReason } from "../../../../shared/textLength.model";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useState } from "react";
import { ApiError, isWriteResultUnknown } from "../../../../core/api/client";
import { useEvaluationFeedback } from "../../evaluation.service";
import type { Evaluation } from "../../evaluation.service";

export function FeedbackForm({
  runId,
  evaluation,
  disabled,
}: {
  runId: string;
  evaluation: Evaluation;
  disabled: boolean;
}) {
  const [comment, setComment] = useState(evaluation.feedback?.comment ?? "");
  const sizeReason = textLengthReason(
    comment,
    "補充說明",
    SetEvaluationFeedbackBodyCommentMaxLength,
  );
  const submit = useEvaluationFeedback(runId);
  const error = submit.error;

  if (disabled) {
    return <p className="note">回饋只能對目前的判定填寫。</p>;
  }

  return (
    <div className="evaluation-feedback">
      <h3>這份評估有幫助嗎</h3>
      {evaluation.feedback && (
        <p className="note">
          你先前的回答：{evaluation.feedback.helpful ? "有幫助" : "沒幫助"}（
          <Timestamp at={evaluation.feedback.submitted_at} />
          ）。可以改。
        </p>
      )}
      <label htmlFor="feedback-comment">補充說明（選填）</label>
      <textarea
        id="feedback-comment"
        rows={3}
        value={comment}
        aria-invalid={sizeReason !== null}
        aria-describedby="feedback-comment-limit"
        onChange={(e) => {
          setComment(e.target.value);
          submit.reset();
        }}
        readOnly={submit.isPending}
      />
      <p id="feedback-comment-limit" className="note" role={sizeReason ? "status" : undefined}>
        {sizeReason ?? `最多 ${SetEvaluationFeedbackBodyCommentMaxLength} 個字。`}
      </p>
      <p>
        <button
          type="button"
          disabled={submit.isPending || sizeReason !== null}
          aria-describedby={sizeReason ? "feedback-comment-limit" : undefined}
          onClick={() => submit.mutate({ helpful: true, comment })}
        >
          有幫助
        </button>{" "}
        <button
          type="button"
          disabled={submit.isPending || sizeReason !== null}
          aria-describedby={sizeReason ? "feedback-comment-limit" : undefined}
          onClick={() => submit.mutate({ helpful: false, comment })}
        >
          沒幫助
        </button>
      </p>
      {submit.isSuccess && <p role="status">已送出回饋。</p>}
      <ReadFailure error={error} what="回饋">
        <p role="alert">
          {error instanceof ApiError && error.status === 404
            ? "這次試跑目前沒有可以附回饋的判定。"
            : isWriteResultUnknown(error)
              ? "尚未確認回饋是否已儲存。補充說明已保留，可以再按一次。"
              : "回饋沒有送出，可以再按一次。"}
        </p>
      </ReadFailure>
    </div>
  );
}
