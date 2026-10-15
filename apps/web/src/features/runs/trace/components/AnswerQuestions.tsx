import { ContinueRunBodyAnswersItemMaxLength } from "@skillhub/api-client-ts/limits";
import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useContinueRun } from "../../runs.service";
import type { TraceSummary } from "../../trace.service";

const characterCount = (text: string) => [...text].length;

const failureMessage = (error: unknown) => (error instanceof Error ? error.message : String(error));

export function AnswerQuestions({
  runId,
  questions,
  readConfirmed,
  checking,
  onRetryRead,
}: {
  runId: string;
  questions: NonNullable<TraceSummary["questions"]>;
  readConfirmed: boolean;
  checking: boolean;
  onRetryRead: () => void;
}) {
  const [answers, setAnswers] = useState<string[]>(() => questions.map(() => ""));
  const navigate = useNavigate();
  const continuation = useContinueRun(runId);
  const trimmed = answers.map((answer) => answer.trim());
  const blank = trimmed.some((answer) => answer === "");
  const tooLong = trimmed.some(
    (answer) => characterCount(answer) > ContinueRunBodyAnswersItemMaxLength,
  );
  const blocked = !readConfirmed || blank || tooLong;

  const setAnswer = (index: number, value: string) => {
    setAnswers((current) => current.map((answer, i) => (i === index ? value : answer)));
    continuation.reset();
  };

  return (
    <section aria-labelledby={`answer-questions-${runId}`}>
      <h3 id={`answer-questions-${runId}`} tabIndex={-1}>
        這次試跑需要你回答
      </h3>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (blocked || continuation.isPending) return;
          continuation.mutate(trimmed, {
            onSuccess: (next) => navigate({ to: "/runs/$runId", params: { runId: next.run_id } }),
          });
        }}
      >
        {questions.map((item, index) => {
          const inputId = `answer-${runId}-${index}`;
          return (
            <fieldset key={index}>
              <legend>{item.question}</legend>
              <p className="note">{item.why}</p>
              {item.options?.map((option) => (
                <label key={option}>
                  <input
                    type="radio"
                    name={inputId}
                    checked={answers[index] === option}
                    onChange={() => setAnswer(index, option)}
                    disabled={continuation.isPending}
                  />{" "}
                  {option}
                </label>
              ))}
              <div className="field">
                <label htmlFor={inputId}>你的回答</label>
                <textarea
                  id={inputId}
                  value={answers[index]}
                  onChange={(event) => setAnswer(index, event.target.value)}
                  readOnly={continuation.isPending}
                />
              </div>
            </fieldset>
          );
        })}
        <button
          type="submit"
          disabled={blocked || continuation.isPending}
          aria-describedby={blocked ? `answer-questions-${runId}-why` : undefined}
        >
          {continuation.isPending ? "送出中…" : "送出回答並繼續試跑"}
        </button>
        {blocked && (
          <p
            id={`answer-questions-${runId}-why`}
            className="note"
            role={!readConfirmed ? "status" : undefined}
          >
            {!readConfirmed
              ? "原始問題尚未確認；已保留先前的問題與你的回答。請先重新確認問題。"
              : blank
                ? "「送出回答並繼續試跑」要等每一題都回答。"
                : `每個回答最多 ${ContinueRunBodyAnswersItemMaxLength} 個字。`}
          </p>
        )}
        {!readConfirmed && (
          <button
            type="button"
            disabled={checking}
            onClick={() => {
              onRetryRead();
              const heading = document.getElementById(`answer-questions-${runId}`);
              heading?.focus();
              heading?.scrollIntoView({ block: "center" });
            }}
          >
            重新確認問題
          </button>
        )}
        {continuation.error && (
          <ReadFailure error={continuation.error} what="送出回答">
            <p role="alert">{failureMessage(continuation.error)}</p>
          </ReadFailure>
        )}
      </form>
    </section>
  );
}
