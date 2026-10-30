import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { focusFeedback } from "../../../../shared/ui/focusFeedback";
import { ApiError } from "../../../../core/api/client";
import { useEffect, useRef, useState } from "react";
import {
  isApplyResultUnknown,
  useApplySuggestions,
  useRunSuggestions,
  type RunSuggestions,
} from "../../evaluation.service";
import { SUGGESTION_OUTCOME_SENTENCE } from "../evaluation.model";
import { useRun } from "../../runs.service";
import { MatchLegend } from "./MatchLegend";
import { ApplyFailureBody } from "./ApplyFailureBody";
import { SuggestionItem, type SuggestionDecisionState } from "./SuggestionItem";
import { AppliedResult } from "./AppliedResult";

function focusSuggestions() {
  const heading = document.getElementById("suggestions-heading");
  heading?.focus();
  heading?.scrollIntoView({ block: "center" });
}

export function SuggestionsPanel({
  runId,
  evaluationId,
  readable,
}: {
  runId: string;
  evaluationId: string;
  readable: boolean;
}) {
  const suggestions = useRunSuggestions(runId);
  const source = useRun(runId);
  const run = source.error ? undefined : source.data;
  const skillId = run?.skill_id;
  const apply = useApplySuggestions(runId);
  const applied = apply.data;
  const error = apply.error;
  const unknown = Boolean(error) && isApplyResultUnknown(error);
  const [decisionStates, setDecisionStates] = useState(
    () => new Map<string, SuggestionDecisionState>(),
  );
  const setDecisionState = (id: string, state?: SuggestionDecisionState) => {
    setDecisionStates((previous) => {
      const next = new Map(previous);
      if (state) next.set(id.toLowerCase(), state);
      else next.delete(id.toLowerCase());
      return next;
    });
  };
  const failure = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (readable && unknown)
      focusFeedback(
        failure.current,
        document.getElementById("evaluation-heading"),
        document.getElementById("suggestions-heading"),
      );
  }, [error, unknown, readable]);
  if (!readable) return null;
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
        sourceUnavailable={Boolean(source.error)}
        apply={apply}
        unknown={unknown}
        decisionStates={decisionStates}
        onDecisionStateChange={setDecisionState}
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
      {applied && (
        <AppliedResult
          result={applied}
          testCaseId={run?.test_case_id}
          sourceConfirmed={Boolean(run)}
          runId={runId}
        />
      )}
    </section>
  );
}

function SuggestionsList({
  list: { suggestions: items, outcome },
  runId,
  decisionStates,
  onDecisionStateChange,
}: {
  list: RunSuggestions;
  runId: string;
  decisionStates: ReadonlyMap<string, SuggestionDecisionState>;
  onDecisionStateChange: (id: string, state?: SuggestionDecisionState) => void;
}) {
  if (items.length === 0) return <p>{SUGGESTION_OUTCOME_SENTENCE[outcome ?? "kept"]}</p>;
  return (
    <>
      <p className="note">「預期影響」是模型的預測，不是量測結果。</p>
      <MatchLegend evidence={items.flatMap((suggestion) => suggestion.evidence)} />
      <ul className="suggestion-list">
        {items.map((suggestion) => (
          <SuggestionItem
            key={suggestion.suggestion_id.toLowerCase()}
            suggestion={suggestion}
            runId={runId}
            decisionState={decisionStates.get(suggestion.suggestion_id.toLowerCase())}
            onDecisionStateChange={onDecisionStateChange}
          />
        ))}
      </ul>
    </>
  );
}

function SuggestionsContent({
  suggestions,
  runId,
  evaluationId,
  skillId,
  sourceUnavailable,
  apply,
  unknown,
  decisionStates,
  onDecisionStateChange,
}: {
  suggestions: ReturnType<typeof useRunSuggestions>;
  runId: string;
  evaluationId: string;
  skillId: string | undefined;
  sourceUnavailable: boolean;
  apply: ReturnType<typeof useApplySuggestions>;
  unknown: boolean;
  decisionStates: ReadonlyMap<string, SuggestionDecisionState>;
  onDecisionStateChange: (id: string, state?: SuggestionDecisionState) => void;
}) {
  const decisionsUnsettled = [...decisionStates.values()].some(
    (state) => state.pending || state.unconfirmed,
  );
  const notFound = suggestions.error instanceof ApiError && suggestions.error.status === 404;
  if (suggestions.isPending) return <Loading what="改善建議" />;
  if (notFound) return null;
  if (suggestions.error)
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
  const holds: BuildHolds = {
    applying: apply.isPending,
    decisionsUnsettled,
    refreshing: suggestions.fetchStatus !== "idle",
    noneAccepted: Boolean(skillId) && accepted.length === 0,
    sourceMissing: !skillId,
  };

  return (
    <>
      <SuggestionsList
        list={suggestions.data}
        runId={runId}
        decisionStates={decisionStates}
        onDecisionStateChange={onDecisionStateChange}
      />

      {suggestions.data.suggestions.length > 0 && (
        <div>
          <p className="note">
            採納建議會建立一個<strong>新的小工具 Version</strong>
            ，不會覆寫已經跑過的版本；新版本的套件內容不同，開始試跑前必須重新確認權限摘要。
          </p>
          <button
            type="button"
            disabled={Object.values(holds).some(Boolean)}
            aria-describedby={buildHoldNoteId(holds)}
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
          <BuildHoldNotes holds={holds} />
          <SuggestionSourceNote skillId={skillId} sourceUnavailable={sourceUnavailable} />
        </div>
      )}
    </>
  );
}

type BuildHolds = {
  applying: boolean;
  decisionsUnsettled: boolean;
  refreshing: boolean;
  noneAccepted: boolean;
  sourceMissing: boolean;
};

function buildHoldNoteId(holds: BuildHolds): string | undefined {
  if (holds.applying) return "apply-suggestions-pending-note";
  if (holds.decisionsUnsettled) return "suggestion-decisions-note";
  if (holds.refreshing) return "suggestions-refresh-note";
  if (holds.noneAccepted) return "suggestions-none-accepted-note";
  return holds.sourceMissing ? "suggestion-source-note" : undefined;
}

function BuildHoldNotes({ holds }: { holds: BuildHolds }) {
  const quiet = !holds.applying && !holds.decisionsUnsettled && !holds.refreshing;
  return (
    <>
      {holds.applying && (
        <p id="apply-suggestions-pending-note" role="status" className="note">
          正在建立新版本…
        </p>
      )}
      {holds.decisionsUnsettled && (
        <p id="suggestion-decisions-note" role="status" className="note">
          改善建議的決定尚未確認；請等待目前操作完成，或重新送出未確認的決定後，再建立版本。
        </p>
      )}
      {holds.refreshing && (
        <p id="suggestions-refresh-note" role="status" className="note">
          改善建議清單尚未確認；確認後才能建立版本。
        </p>
      )}
      {quiet && holds.noneAccepted && (
        <p id="suggestions-none-accepted-note" className="note">
          還沒有接受任何建議。先在上面的建議接受至少一項，才能用它們建立新版本。
        </p>
      )}
    </>
  );
}

function SuggestionSourceNote({
  skillId,
  sourceUnavailable,
}: {
  skillId: string | undefined;
  sourceUnavailable: boolean;
}) {
  if (skillId) return null;
  return (
    <p id="suggestion-source-note" className="note">
      {sourceUnavailable ? (
        <>
          試跑來源尚未確認，暫時不能建立新版本。請先
          <a href="#run-heading">重新確認試跑來源</a>。
        </>
      ) : (
        "正在讀取這次試跑屬於哪個小工具…"
      )}
    </p>
  );
}
