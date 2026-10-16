import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useEffect, useRef, useState } from "react";
import { isWriteResultUnknown, serverReason } from "../../../../core/api/client";
import { useDecideSuggestion } from "../../evaluation.service";
import type { ImprovementSuggestion } from "../../evaluation.service";
import { SUGGESTION_CATEGORY_LABEL } from "../evaluation.model";
import { EvidenceList } from "./EvidenceList";
import { SuggestionDiffView } from "./SuggestionDiffView";

type UnconfirmedDecision = { error: unknown; decision: "accepted" | "rejected" };

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
    if (failure) feedback.current?.focus();
  }, [failure]);
  return (
    <>
      <ReadFailure error={failure} what="決定">
        <p ref={feedback} tabIndex={-1} role="alert">
          {isWriteResultUnknown(failure)
            ? `尚未確認${choice}是否已記錄。清單仍顯示上次讀取的決定。再次選擇會重新送出，並取代目前決定。`
            : (serverReason(failure) ?? "這個決定被拒絕。請重新讀取目前判定後再試。")}
        </p>
      </ReadFailure>
      {unconfirmed && error !== unconfirmed.error && (
        <ReadFailure error={error} what="決定">
          <p role="alert">{serverReason(error) ?? "這次重新送出被拒絕，先前的決定仍未確認。"}</p>
        </ReadFailure>
      )}
    </>
  );
}

export function SuggestionItem({
  suggestion,
  runId,
  onDecisionUnsettled,
}: {
  suggestion: ImprovementSuggestion;
  runId: string;
  onDecisionUnsettled: (id: string, unsettled: boolean) => void;
}) {
  const [showDiff, setShowDiff] = useState(false);
  const [unconfirmed, setUnconfirmed] = useState<UnconfirmedDecision>();
  const decide = useDecideSuggestion(runId);
  const error = decide.error;
  const pendingDecision = decide.isPending ? decide.variables?.decision : undefined;
  const choose = (decision: "accepted" | "rejected") => {
    onDecisionUnsettled(suggestion.suggestion_id, true);
    decide.mutate(
      { suggestionId: suggestion.suggestion_id, decision },
      {
        onError: (error) => {
          if (isWriteResultUnknown(error)) setUnconfirmed({ error, decision });
          if (!isWriteResultUnknown(error) && !unconfirmed)
            onDecisionUnsettled(suggestion.suggestion_id, false);
        },
        onSuccess: () => {
          setUnconfirmed(undefined);
          onDecisionUnsettled(suggestion.suggestion_id, false);
        },
      },
    );
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
        <button type="button" onClick={() => setShowDiff((v) => !v)}>
          {showDiff ? "收起差異" : "查看差異"}
        </button>{" "}
        <button
          type="button"
          aria-pressed={suggestion.decision === "accepted"}
          disabled={decide.isPending}
          onClick={() => choose("accepted")}
        >
          {pendingDecision === "accepted" ? "接受中…" : "接受"}
        </button>{" "}
        <button
          type="button"
          aria-pressed={suggestion.decision === "rejected"}
          disabled={decide.isPending || Boolean(suggestion.applied_skill_version_id)}
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
            : `${unconfirmed ? "上次讀取" : "目前"}：${
                suggestion.decision === "accepted"
                  ? "已接受"
                  : suggestion.decision === "rejected"
                    ? "已拒絕"
                    : "尚未決定"
              }`}
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
