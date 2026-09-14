import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useState } from "react";
import { ApiError } from "../../../../core/api/client";
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
        maxLength={2000}
        value={comment}
        onChange={(e) => setComment(e.target.value)}
      />
      <p>
        <button
          type="button"
          disabled={submit.isPending}
          onClick={() => submit.mutate({ helpful: true, comment })}
        >
          有幫助
        </button>{" "}
        <button
          type="button"
          disabled={submit.isPending}
          onClick={() => submit.mutate({ helpful: false, comment })}
        >
          沒幫助
        </button>
      </p>
      {submit.isSuccess && <p role="status">已送出回饋。</p>}
      <ReadFailure error={error} what="回饋">
        <p role="alert">
          {error instanceof ApiError && error.status === 404
            ? "這個 Run 目前沒有可以附回饋的判定。"
            : "回饋沒有送出，可以再按一次。"}
        </p>
      </ReadFailure>
    </div>
  );
}
