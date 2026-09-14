import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useState } from "react";
import { useDecideSuggestion } from "../../evaluation.service";
import type { ImprovementSuggestion } from "../../evaluation.service";
import { SUGGESTION_CATEGORY_LABEL } from "../evaluation.model";
import { EvidenceList } from "./EvidenceList";
import { SuggestionDiffView } from "./SuggestionDiffView";

export function SuggestionItem({
  suggestion,
  runId,
}: {
  suggestion: ImprovementSuggestion;
  runId: string;
}) {
  const [showDiff, setShowDiff] = useState(false);
  const decide = useDecideSuggestion(runId);
  const error = decide.error;
  const choose = (decision: "accepted" | "rejected") =>
    decide.mutate({ suggestionId: suggestion.suggestion_id, decision });

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
          接受
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
          拒絕
        </button>{" "}
        <span className="note">
          目前：
          {suggestion.decision === "accepted"
            ? "已接受"
            : suggestion.decision === "rejected"
              ? "已拒絕"
              : "尚未決定"}
          {suggestion.applied_skill_version_id ? "（已套用於新版本）" : ""}
        </span>
        {suggestion.applied_skill_version_id && (
          <span className="note" id={`reject-note-${suggestion.suggestion_id}`}>
            已建成版本的建議不能撤回
          </span>
        )}
      </p>
      <ReadFailure error={error} what="決定">
        <p role="alert">這個決定沒有記錄，可以再按一次。</p>
      </ReadFailure>
      {showDiff && <SuggestionDiffView suggestionId={suggestion.suggestion_id} />}
    </li>
  );
}
