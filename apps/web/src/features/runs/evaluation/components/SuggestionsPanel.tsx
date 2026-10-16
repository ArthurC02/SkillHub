import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { ApiError } from "../../../../core/api/client";
import { useEffect, useRef, useState } from "react";
import {
  isApplyResultUnknown,
  useApplySuggestions,
  useRunSuggestions,
} from "../../evaluation.service";
import { useRun } from "../../runs.service";
import { MatchLegend } from "./MatchLegend";
import { ApplyFailureBody } from "./ApplyFailureBody";
import { SuggestionItem } from "./SuggestionItem";
import { AppliedResult } from "./AppliedResult";

function focusSuggestions() {
  const heading = document.getElementById("suggestions-heading");
  heading?.focus();
  heading?.scrollIntoView({ block: "center" });
}

export function SuggestionsPanel({ runId, evaluationId }: { runId: string; evaluationId: string }) {
  const suggestions = useRunSuggestions(runId);
  const run = useRun(runId).data;
  const skillId = run?.skill_id;
  const apply = useApplySuggestions(runId);
  const applied = apply.data;
  const error = apply.error;
  const unknown = Boolean(error) && isApplyResultUnknown(error);
  const [unsettledDecisions, setUnsettledDecisions] = useState(new Set<string>());
  const setDecisionUnsettled = (id: string, unsettled: boolean) => {
    setUnsettledDecisions((previous) => {
      const next = new Set(previous);
      if (unsettled) next.add(id);
      else next.delete(id);
      return next;
    });
  };
  const failure = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (unknown) failure.current?.focus();
  }, [error, unknown]);
  if (
    suggestions.error instanceof ApiError &&
    suggestions.error.status === 404 &&
    !error &&
    !applied
  )
    return null;

  return (
    <section aria-labelledby="suggestions-heading">
      <h3 id="suggestions-heading" tabIndex={-1}>
        改善建議
      </h3>
      <SuggestionsContent
        suggestions={suggestions}
        runId={runId}
        evaluationId={evaluationId}
        skillId={skillId}
        apply={apply}
        unknown={unknown}
        decisionsUnsettled={unsettledDecisions.size > 0}
        onDecisionUnsettled={setDecisionUnsettled}
      />
      <ReadFailure error={error} what="建立新版本">
        <div ref={failure} role="alert" tabIndex={-1} aria-label="建立版本結果">
          <ApplyFailureBody error={error} skillId={apply.variables?.skillId} />
        </div>
      </ReadFailure>
      {unknown && (
        <button
          type="button"
          disabled={suggestions.fetchStatus !== "idle"}
          onClick={() => {
            void suggestions.refetch();
            focusSuggestions();
          }}
        >
          重新確認建議套用狀態
        </button>
      )}
      {applied && <AppliedResult result={applied} testCaseId={run?.test_case_id} runId={runId} />}
    </section>
  );
}

function SuggestionsContent({
  suggestions,
  runId,
  evaluationId,
  skillId,
  apply,
  unknown,
  decisionsUnsettled,
  onDecisionUnsettled,
}: {
  suggestions: ReturnType<typeof useRunSuggestions>;
  runId: string;
  evaluationId: string;
  skillId: string | undefined;
  apply: ReturnType<typeof useApplySuggestions>;
  unknown: boolean;
  decisionsUnsettled: boolean;
  onDecisionUnsettled: (id: string, unsettled: boolean) => void;
}) {
  const notFound = suggestions.error instanceof ApiError && suggestions.error.status === 404;
  if (suggestions.isPending) return <Loading what="改善建議" />;
  if (notFound) return null;
  if (suggestions.error) {
    return (
      <ReadFailure
        error={suggestions.error}
        what="改善建議"
        retry={() => {
          void suggestions.refetch();
          focusSuggestions();
        }}
        retrying={suggestions.isFetching}
      />
    );
  }

  if (suggestions.data.evaluation_id !== evaluationId) {
    return (
      <section>
        <p className="notice notice-warning" role="status">
          取得的改善建議屬於另一份評估，不能作為這份判定的改善依據。可重新讀取，或回到目前的判定。
        </p>
        <button
          type="button"
          disabled={suggestions.isFetching}
          onClick={() => void suggestions.refetch()}
        >
          {suggestions.isFetching ? "重新讀取中…" : "重新讀取改善建議"}
        </button>
      </section>
    );
  }

  const accepted = suggestions.data.suggestions.filter((s) => s.decision === "accepted");

  return (
    <>
      {suggestions.data.suggestions.length === 0 ? (
        <p>這份評估沒有產生改善建議。</p>
      ) : (
        <>
          <p className="note">「預期影響」是模型的預測，不是量測結果。</p>
          <MatchLegend evidence={suggestions.data.suggestions.flatMap((s) => s.evidence)} />
          <ul className="suggestion-list">
            {suggestions.data.suggestions.map((s) => (
              <SuggestionItem
                key={s.suggestion_id}
                suggestion={s}
                runId={runId}
                onDecisionUnsettled={onDecisionUnsettled}
              />
            ))}
          </ul>
        </>
      )}

      {suggestions.data.suggestions.length > 0 && (
        <div>
          <p className="note">
            採納建議會建立一個<strong>新的小工具 Version</strong>
            ，不會覆寫已經跑過的版本；新版本的套件內容不同，開始試跑前必須重新確認權限摘要。
          </p>
          <button
            type="button"
            disabled={
              !skillId ||
              accepted.length === 0 ||
              decisionsUnsettled ||
              apply.isPending ||
              suggestions.fetchStatus !== "idle"
            }
            aria-describedby={
              decisionsUnsettled
                ? "suggestion-decisions-note"
                : suggestions.fetchStatus !== "idle"
                  ? "suggestions-refresh-note"
                  : undefined
            }
            onClick={() =>
              apply.mutate({
                skillId: skillId as string,
                evaluationId: suggestions.data.evaluation_id,
                suggestionIds: accepted.map((s) => s.suggestion_id),
              })
            }
          >
            {unknown ? "核對後仍" : ""}以已接受的 {accepted.length} 項建議建立新版本
          </button>
          {decisionsUnsettled && (
            <p id="suggestion-decisions-note" role="status" className="note">
              改善建議的決定尚未確認；請等待目前操作完成，或重新送出未確認的決定後，再建立版本。
            </p>
          )}
          {suggestions.fetchStatus !== "idle" && (
            <p id="suggestions-refresh-note" role="status" className="note">
              改善建議清單尚未確認；確認後才能建立版本。
            </p>
          )}
          {!skillId && <p className="note">正在讀取這次試跑屬於哪個小工具…</p>}
        </div>
      )}
    </>
  );
}
