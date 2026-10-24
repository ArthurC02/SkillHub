import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { focusFeedback } from "../../../../shared/ui/focusFeedback";
import { useEffect, useRef, useState } from "react";
import { ApiError, isWriteResultUnknown } from "../../../../core/api/client";
import { useDecideSuggestion } from "../../evaluation.service";
import type { ImprovementSuggestion } from "../../evaluation.service";
import { SUGGESTION_CATEGORY_LABEL } from "../evaluation.model";
import { EvidenceList } from "./EvidenceList";
import { SuggestionDiffView } from "./SuggestionDiffView";

const DECISION_LABEL = { accepted: "已接受", rejected: "已拒絕", pending: "尚未決定" };

function decisionRefusalSentence(err: unknown): string {
  if (err instanceof ApiError && err.status === 404)
    return "這個建議已經不在了，請重新讀取這份評估。";
  if (err instanceof ApiError && err.status === 409)
    return "這個建議的狀態已經變了，可能已在別處做了決定或建成新版本；請重新讀取目前判定後再試。";
  return "請重新讀取目前判定後再試。";
}

type DecisionChoice = "accepted" | "rejected";
type UnconfirmedDecision = { error: unknown; decision: DecisionChoice };

export type SuggestionDecisionState = {
  pending?: DecisionChoice;
  error?: unknown;
  unconfirmed?: UnconfirmedDecision;
};

function DecisionFailure({
  error,
  unconfirmed,
}: {
  error: unknown;
  unconfirmed?: UnconfirmedDecision;
}) {
  const failure = unconfirmed?.error ?? error;
  const choice = unconfirmed
    ? `「${unconfirmed.decision === "accepted" ? "接受" : "拒絕"}」`
    : "這個決定";
  const feedback = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (failure)
      focusFeedback(
        feedback.current,
        document.getElementById("evaluation-heading"),
        document.getElementById("suggestions-heading"),
      );
  }, [failure]);
  return (
    <>
      <ReadFailure error={failure} what="決定">
        <p ref={feedback} tabIndex={-1} role="alert">
          {isWriteResultUnknown(failure)
            ? `尚未確認${choice}是否已記錄。清單仍顯示上次讀取的決定。再次選擇會重新送出，並取代目前決定。`
            : `這個決定被拒絕。${decisionRefusalSentence(failure)}`}
        </p>
      </ReadFailure>
      {unconfirmed && error !== unconfirmed.error && (
        <ReadFailure error={error} what="決定">
          <p role="alert">{`這次重新送出被拒絕，先前的決定仍未確認。${decisionRefusalSentence(error)}`}</p>
        </ReadFailure>
      )}
    </>
  );
}

export function SuggestionItem({
  suggestion,
  runId,
  decisionState,
  onDecisionStateChange,
}: {
  suggestion: ImprovementSuggestion;
  runId: string;
  decisionState?: SuggestionDecisionState;
  onDecisionStateChange: (id: string, state?: SuggestionDecisionState) => void;
}) {
  const [showDiff, setShowDiff] = useState(false);
  const unconfirmed = decisionState?.unconfirmed;
  const decide = useDecideSuggestion(runId, {
    onError: (error, decision) => {
      onDecisionStateChange(suggestion.suggestion_id, {
        error,
        unconfirmed: isWriteResultUnknown(error) ? { error, decision } : unconfirmed,
      });
    },
    onConfirmed: () => {
      onDecisionStateChange(suggestion.suggestion_id);
    },
  });
  const error = decisionState?.error;
  const pendingDecision =
    decisionState?.pending ?? (decide.isPending ? decide.variables?.decision : undefined);
  const pending = pendingDecision !== undefined;
  const choose = (decision: DecisionChoice) => {
    onDecisionStateChange(suggestion.suggestion_id, { pending: decision, unconfirmed });
    decide.mutate({ suggestionId: suggestion.suggestion_id, decision });
  };

  return (
    <li className="suggestion">
      <p>
        <span className="badge">{SUGGESTION_CATEGORY_LABEL[suggestion.category]}</span>{" "}
        <code>{suggestion.target_path}</code>
      </p>
      <p>{suggestion.problem}</p>
      <p className="note">預期影響：{suggestion.expected_impact}</p>
      <EvidenceList evidence={suggestion.evidence} />

      <p>
        <button type="button" aria-expanded={showDiff} onClick={() => setShowDiff((v) => !v)}>
          {showDiff ? "收起差異" : "查看差異"}
        </button>{" "}
        <button
          type="button"
          aria-pressed={suggestion.decision === "accepted"}
          disabled={pending}
          onClick={() => choose("accepted")}
        >
          {pendingDecision === "accepted" ? "接受中…" : "接受"}
        </button>{" "}
        <button
          type="button"
          aria-pressed={suggestion.decision === "rejected"}
          disabled={pending || Boolean(suggestion.applied_skill_version_id)}
          aria-describedby={
            suggestion.applied_skill_version_id
              ? `reject-note-${suggestion.suggestion_id}`
              : undefined
          }
          onClick={() => choose("rejected")}
        >
          {pendingDecision === "rejected" ? "拒絕中…" : "拒絕"}
        </button>{" "}
        <span className="note" role={pendingDecision ? "status" : undefined}>
          {pendingDecision
            ? `處理中：正在${pendingDecision === "accepted" ? "接受" : "拒絕"}…`
            : `${unconfirmed ? "上次讀取" : "目前"}：${DECISION_LABEL[suggestion.decision]}`}
          {suggestion.applied_skill_version_id ? "（已套用於新版本）" : ""}
        </span>
        {suggestion.applied_skill_version_id && (
          <span className="note" id={`reject-note-${suggestion.suggestion_id}`}>
            已建成版本的建議不能撤回
          </span>
        )}
      </p>
      <DecisionFailure error={error} unconfirmed={unconfirmed} />
      {showDiff && <SuggestionDiffView suggestionId={suggestion.suggestion_id} />}
    </li>
  );
}
