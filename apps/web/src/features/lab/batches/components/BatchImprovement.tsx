import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { userFacingReason } from "../../../../core/api/client";
import { queryKeys } from "../../../../core/api/queryKeys";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import {
  ApplyFailureBody,
  MatchLegend,
  PREDICTED_IMPACT_NOTE,
  SuggestionItem,
  isApplyResultUnknown,
  type SuggestionDecisionState,
} from "../../../runs";
import type { BatchReport } from "../batches.model";
import { CandidateEvaluation } from "./CandidateEvaluation";
import { useApplyReflection, useBatchReflections, useReflectOnBatch } from "../batches.service";
import {
  CANDIDATE_FOCUS_LABEL,
  REFLECTION_OUTCOME_SENTENCE,
  heldOutTestCaseIds,
  reflectHoldReason,
  type BatchReflection,
} from "../reflection.model";

const HEADING = "batch-improvement-heading";

export function BatchImprovement({
  skillId,
  batchId,
  report,
  names,
}: {
  skillId: string;
  batchId: string;
  report: BatchReport;
  names: ReadonlyMap<string, string>;
}) {
  const reflections = useBatchReflections(skillId, batchId);
  const reflect = useReflectOnBatch(skillId, batchId);
  const [dev, setDev] = useState<ReadonlySet<string>>(new Set());
  const testCases = report.test_cases;
  const hold = reflectHoldReason(testCases, dev, report.batch.status === "launching");
  const latest = reflections.error ? undefined : reflections.data?.[0];
  const toggle = (id: string) =>
    setDev((held) => {
      const next = new Set(held);
      if (!next.delete(id)) next.add(id);
      return next;
    });

  return (
    <section aria-labelledby={HEADING}>
      <h2 id={HEADING} tabIndex={-1}>
        從這一批找改進
      </h2>
      <p className="note">
        勾選的是開發題：模型只讀它們失敗的試跑，提出改善建議。沒勾的題目留著檢查，模型看不到；套用建議之後用它們重跑新版本，才知道改進是不是真的有用。
      </p>
      <fieldset>
        <legend>開發題</legend>
        {testCases.map((tc) => {
          const id = tc.test_case_id.toLowerCase();
          return (
            <p key={id}>
              <label>
                <input type="checkbox" checked={dev.has(id)} onChange={() => toggle(id)} />{" "}
                {names.get(id) ?? "（測試題已刪除）"}
              </label>
            </p>
          );
        })}
      </fieldset>
      <p>
        <button
          type="button"
          aria-disabled={hold !== undefined || reflect.isPending}
          aria-describedby="batch-improvement-hold"
          onClick={() => {
            if (hold === undefined && !reflect.isPending) reflect.mutate([...dev]);
          }}
        >
          {reflect.isPending ? "找改進中…" : "找改進"}
        </button>
      </p>
      <p className="note" role="status" id="batch-improvement-hold">
        {hold ?? "這次會呼叫模型並扣點；最多讀 8 次失敗的試跑。"}
      </p>
      {reflect.error && (
        <ReadFailure error={reflect.error} what="找改進">
          <p role="alert">{reflectFailureSentence(reflect.error)}</p>
        </ReadFailure>
      )}
      {reflections.isPending && <Loading what="先前找到的改進" />}
      <ReadFailure
        error={reflections.error}
        what="先前找到的改進"
        retry={() => void reflections.refetch()}
        retrying={reflections.isFetching}
      />
      {latest && (
        <ReflectionResult
          skillId={skillId}
          batchId={batchId}
          reflection={latest}
          testCaseIds={testCases.map((tc) => tc.test_case_id)}
        />
      )}
      {latest && latest.suggestions.length > 0 && (
        <CandidateEvaluation
          skillId={skillId}
          batchId={batchId}
          reflection={latest}
          caseCount={testCases.length}
          names={names}
        />
      )}
    </section>
  );
}

function reflectFailureSentence(error: Error): string {
  if (isApplyResultUnknown(error)) return "無法確認這次找改進是否完成；重新整理頁面就會看到結果。";
  return userFacingReason(error) ?? "找改進失敗，可以再試一次。";
}

const APPLY_EFFECT = "會建立一個新版本，不會覆寫這一批跑過的版本。";
const APPLY_OR_EVALUATE =
  "只用你接受的建議建一個新版本，不先試跑，也不覆寫這一批跑過的版本；想先比較各候選，用下方的「評估候選」。";

function ReflectionResult({
  skillId,
  batchId,
  reflection,
  testCaseIds,
}: {
  skillId: string;
  batchId: string;
  reflection: BatchReflection;
  testCaseIds: string[];
}) {
  const apply = useApplyReflection(skillId, batchId);
  const [states, setStates] = useState(() => new Map<string, SuggestionDecisionState>());
  const setState = (id: string, state?: SuggestionDecisionState) =>
    setStates((held) => {
      const next = new Map(held);
      if (state) next.set(id.toLowerCase(), state);
      else next.delete(id.toLowerCase());
      return next;
    });
  const items = reflection.suggestions;
  const unapplied = items.filter((s) => s.decision === "accepted" && !s.applied_skill_version_id);
  const unsettled = [...states.values()].some((s) => s.pending || s.unconfirmed);
  const applyHold = unsettled
    ? "建議的決定尚未確認；等目前操作完成再建立版本。"
    : unapplied.length === 0
      ? "先接受至少一項還沒套用的建議。"
      : undefined;
  const heldOut = heldOutTestCaseIds(testCaseIds, reflection);
  const created = apply.data;

  return (
    <div>
      <h3>最近一次找到的改進</h3>
      <p role="status">
        讀了開發題 {reflection.dev_test_case_ids.length} 題裡 {reflection.runs_read}{" "}
        次失敗的試跑；另外 {heldOut.length} 題留著檢查，沒有給模型看。
        {reflection.runs_not_read > 0 &&
          ` 還有 ${reflection.runs_not_read} 次失敗的試跑沒有讀，這次只看了一部分。`}
      </p>
      {items.length === 0 ? (
        <p>{REFLECTION_OUTCOME_SENTENCE[reflection.outcome]}</p>
      ) : (
        <>
          <p className="note">{PREDICTED_IMPACT_NOTE}</p>
          <MatchLegend evidence={items.flatMap((s) => s.evidence)} />
          {candidateGroups(reflection).map((group) => (
            <div key={group.key}>
              {group.title && <h4>{group.title}</h4>}
              {group.items.length === 0 ? (
                <p>{group.empty}</p>
              ) : (
                <ul className="suggestion-list">
                  {group.items.map((s) => (
                    <SuggestionItem
                      key={s.suggestion_id.toLowerCase()}
                      suggestion={s}
                      listKey={queryKeys.evaluationBatches.reflections(skillId, batchId)}
                      decisionState={states.get(s.suggestion_id.toLowerCase())}
                      onDecisionStateChange={setState}
                    />
                  ))}
                </ul>
              )}
            </div>
          ))}
          <p>
            <button
              type="button"
              aria-disabled={applyHold !== undefined || apply.isPending}
              aria-describedby="batch-improvement-apply-hold"
              onClick={() => {
                if (applyHold === undefined && !apply.isPending)
                  apply.mutate({
                    reflectionId: reflection.reflection_id,
                    suggestionIds: unapplied.map((s) => s.suggestion_id),
                  });
              }}
            >
              {apply.isPending
                ? "建立新版本中…"
                : `以已接受的 ${unapplied.length} 項建議建立新版本`}
            </button>
          </p>
          <p className="note" role="status" id="batch-improvement-apply-hold">
            {applyHold ?? (reflection.evaluation ? APPLY_EFFECT : APPLY_OR_EVALUATE)}
          </p>
        </>
      )}
      {apply.error && (
        <ReadFailure error={apply.error} what="建立新版本">
          <div role="alert">
            <ApplyFailureBody error={apply.error} skillId={skillId} />
          </div>
        </ReadFailure>
      )}
      {created && (
        <p role="status">
          已建立新版本 #{created.version_number}
          {created.duplicate ? "（內容與既有版本相同，沿用該版本）" : ""}。{" "}
          <Link
            to="/skills/$skillId/evaluation-batches"
            params={{ skillId }}
            search={{
              version: created.version_id,
              rerun: batchId,
              reflection: reflection.reflection_id,
            }}
          >
            {`只用沒給模型看的 ${heldOut.length} 題重跑新版本`}
          </Link>
        </p>
      )}
    </div>
  );
}

type CandidateGroup = {
  key: string;
  title: string;
  empty: string;
  items: BatchReflection["suggestions"];
};

function candidateGroups(reflection: BatchReflection): CandidateGroup[] {
  if (reflection.candidates.length === 0)
    return [{ key: "all", title: "", empty: "", items: reflection.suggestions }];
  const byId = new Map(reflection.suggestions.map((s) => [s.suggestion_id.toLowerCase(), s]));
  return reflection.candidates.map((candidate, i) => ({
    key: `${i}:${candidate.focus}`,
    title: `候選 ${i + 1}：${CANDIDATE_FOCUS_LABEL[candidate.focus]}`,
    empty: REFLECTION_OUTCOME_SENTENCE[candidate.outcome],
    items: candidate.suggestion_ids.flatMap((id) => byId.get(id.toLowerCase()) ?? []),
  }));
}
