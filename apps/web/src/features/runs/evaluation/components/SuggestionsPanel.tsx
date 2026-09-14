import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { ApiError } from "../../../../core/api/client";
import { useApplySuggestions, useRunSuggestions } from "../../evaluation.service";
import { useRun } from "../../runs.service";
import { MatchLegend } from "./MatchLegend";
import { ApplyFailureBody } from "./ApplyFailureBody";
import { SuggestionItem } from "./SuggestionItem";
import { AppliedResult } from "./AppliedResult";

export function SuggestionsPanel({ runId }: { runId: string }) {
  const suggestions = useRunSuggestions(runId);
  const run = useRun(runId).data;
  const skillId = run?.skill_id;
  const apply = useApplySuggestions(runId);
  const applied = apply.data;
  const error = apply.error;

  const notFound = suggestions.error instanceof ApiError && suggestions.error.status === 404;
  if (suggestions.isPending) return <Loading what="改善建議" />;
  if (notFound) return null;
  if (suggestions.error) {
    return <ReadFailure error={suggestions.error} what="改善建議" />;
  }

  const accepted = suggestions.data.suggestions.filter((s) => s.decision === "accepted");

  return (
    <section>
      <h3>改善建議</h3>
      {suggestions.data.suggestions.length === 0 ? (
        <p>這份評估沒有產生改善建議。</p>
      ) : (
        <>
          <p className="note">「預期影響」是模型的預測，不是量測結果。</p>
          <MatchLegend evidence={suggestions.data.suggestions.flatMap((s) => s.evidence)} />
          <ul className="suggestion-list">
            {suggestions.data.suggestions.map((s) => (
              <SuggestionItem key={s.suggestion_id} suggestion={s} runId={runId} />
            ))}
          </ul>
        </>
      )}

      {suggestions.data.suggestions.length > 0 && (
        <div>
          <p className="note">
            採納建議會建立一個<strong>新的 Skill Version</strong>
            ，不會覆寫已經跑過的版本；新版本的套件內容不同，開始 Run 前必須重新確認權限摘要。
          </p>
          <button
            type="button"
            disabled={!skillId || accepted.length === 0 || apply.isPending}
            onClick={() =>
              apply.mutate({
                skillId: skillId as string,
                evaluationId: suggestions.data.evaluation_id,
                suggestionIds: accepted.map((s) => s.suggestion_id),
              })
            }
          >
            以已接受的 {accepted.length} 項建議建立新版本
          </button>
          {!skillId && <p className="note">正在讀取這個 Run 屬於哪個 Skill…</p>}
        </div>
      )}

      <ReadFailure error={error} what="建立新版本">
        <div role="alert">
          <ApplyFailureBody error={error} />
        </div>
      </ReadFailure>
      {applied && <AppliedResult result={applied} testCaseId={run?.test_case_id} />}
    </section>
  );
}
