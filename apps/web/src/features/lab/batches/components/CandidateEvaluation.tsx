import { Link } from "@tanstack/react-router";
import { ApiError, userFacingReason } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { verdictSentence } from "../batches.model";
import {
  useAdoptCandidate,
  useConfirmEveryPair,
  useEvaluateCandidates,
  usePairPreflights,
} from "../batches.service";
import {
  CANDIDATE_FOCUS_LABEL,
  unconfirmedPairs,
  type BatchReflection,
  type CandidateStanding,
  type ReflectionEvaluation,
  type UnconfirmedPair,
} from "../reflection.model";
import { UnconfirmedTestCase } from "./UnconfirmedTestCase";

type Props = {
  skillId: string;
  batchId: string;
  reflection: BatchReflection;
  caseCount: number;
  names: ReadonlyMap<string, string>;
};

function candidateTitle(reflection: BatchReflection, candidate: number): string {
  const focus = reflection.candidates[candidate]?.focus;
  return focus ? `候選 ${candidate + 1}：${CANDIDATE_FOCUS_LABEL[focus]}` : `候選 ${candidate + 1}`;
}

export function CandidateEvaluation(props: Props) {
  const { reflection } = props;
  if (reflection.evaluation) return <Standings {...props} evaluation={reflection.evaluation} />;
  return <EvaluateAction {...props} />;
}

function EvaluateAction({ skillId, batchId, reflection, caseCount, names }: Props) {
  const evaluate = useEvaluateCandidates(skillId, batchId);
  const pending = reflection.suggestions.filter((s) => s.decision === "pending").length;
  const rejected = reflection.suggestions.filter((s) => s.decision === "rejected").length;
  const candidates = Math.max(reflection.candidates.length, 1);
  const pairs =
    evaluate.error instanceof ApiError && evaluate.error.status === 422
      ? unconfirmedPairs(evaluate.error.body)
      : undefined;
  const run = () => {
    if (!evaluate.isPending)
      evaluate.mutate({ reflectionId: reflection.reflection_id, input: undefined });
  };

  return (
    <section aria-labelledby="candidate-evaluation-heading">
      <h3 id="candidate-evaluation-heading">評估候選</h3>
      <p className="note" id="candidate-evaluation-terms">
        {`按下後會接受這一輪還沒決定的 ${pending} 項建議${rejected > 0 ? `（已拒絕的 ${rejected} 項不會用）` : ""}，`}
        {`每個候選各建一個保留中的版本，再用這一批的 ${caseCount} 題各跑一批。`}
        保留中的版本不會變成目前版本，也不能發佈，要等你採用；試跑會扣點。
      </p>
      <p>
        <button
          type="button"
          aria-disabled={evaluate.isPending}
          aria-describedby="candidate-evaluation-terms"
          onClick={run}
        >
          {evaluate.isPending ? "評估中…" : `評估 ${candidates} 個候選`}
        </button>
      </p>
      {pairs ? (
        <Consent skillId={skillId} pairs={pairs} names={names} onConfirmed={run} />
      ) : (
        evaluate.error && (
          <ReadFailure error={evaluate.error} what="評估候選">
            <p role="alert">{userFacingReason(evaluate.error) ?? "評估候選失敗，可以再試一次。"}</p>
          </ReadFailure>
        )
      )}
    </section>
  );
}

function Consent({
  skillId,
  pairs,
  names,
  onConfirmed,
}: {
  skillId: string;
  pairs: UnconfirmedPair[];
  names: ReadonlyMap<string, string>;
  onConfirmed: () => void;
}) {
  const preflights = usePairPreflights(skillId, pairs);
  const confirmAll = useConfirmEveryPair(skillId);
  const shown = pairs.flatMap((pair, i) => {
    const hash = preflights[i]?.data?.summary_hash;
    return hash ? [{ pair, summaryHash: hash }] : [];
  });
  const ready = shown.length === pairs.length;

  return (
    <section aria-labelledby="candidate-consent-heading">
      <h4 id="candidate-consent-heading" tabIndex={-1}>
        候選版本要先確認試跑權限
      </h4>
      <p role="alert">
        {`候選批次還沒開。下面 ${pairs.length} 題在候選版本上可以接觸的範圍要先看過並確認。`}
      </p>
      <p>
        <button
          type="button"
          aria-disabled={!ready || confirmAll.isPending}
          onClick={() => {
            if (ready && !confirmAll.isPending)
              confirmAll.mutate(shown, { onSuccess: onConfirmed });
          }}
        >
          {confirmAll.isPending ? "確認中…" : `我看過了，確認列出的 ${pairs.length} 題`}
        </button>
      </p>
      <ReadFailure error={confirmAll.error} what="權限確認">
        <p role="alert">有幾題的權限沒有確認成功，請再按一次。</p>
      </ReadFailure>
      {pairs.map((pair) => (
        <UnconfirmedTestCase
          key={`${pair.skill_version_id}:${pair.test_case_id}`}
          skillId={skillId}
          versionId={pair.skill_version_id}
          testCaseId={pair.test_case_id}
          name={names.get(pair.test_case_id.toLowerCase())}
        />
      ))}
    </section>
  );
}

function standingSentence(standing: CandidateStanding): string {
  if (!standing.batch_id) return "沒有可以套用的建議，所以沒有建版本。";
  if (!standing.settled) return "這個候選的批次還在跑，跑完才比較。";
  const heldOut = verdictSentence(standing.held_out.verdict, "比原版好", "比原版差");
  return `${standing.kept ? "在開發題上有勝出，保留" : "在每一題開發題上都沒有勝過其他候選與原版，淘汰"}；沒給模型看的題目：${heldOut}。`;
}

function Standings({
  skillId,
  batchId,
  reflection,
  evaluation,
}: Props & { evaluation: ReflectionEvaluation }) {
  const adopt = useAdoptCandidate(skillId, batchId);
  const settled = evaluation.candidates.every((c) => c.settled || !c.batch_id);
  const adopted = evaluation.adopted;

  return (
    <section aria-labelledby="candidate-standings-heading">
      <h3 id="candidate-standings-heading">候選的評估結果</h3>
      <p className="note">
        保不保留只看開發題；建議採用哪一個只看沒給模型看的題目。開發題的結果列在旁邊做參考，不作為判斷依據。
      </p>
      <ul>
        {evaluation.candidates.map((standing) => (
          <li key={standing.candidate}>
            <p>
              <strong>{candidateTitle(reflection, standing.candidate)}</strong>：
              {standingSentence(standing)}
            </p>
            {standing.batch_id && standing.settled && (
              <p className="note">
                {`開發題（不作為判斷依據）：${verdictSentence(standing.dev.verdict, "比原版好", "比原版差")}。`}
              </p>
            )}
            {standing.skipped_suggestion_ids.length > 0 && (
              <p className="note">{`有 ${standing.skipped_suggestion_ids.length} 項建議沒有放進這個版本：被拒絕，或重新核對時套不上。`}</p>
            )}
            {standing.batch_id && (
              <p>
                <Link
                  to="/skills/$skillId/evaluation-batches/$batchId"
                  params={{ skillId, batchId: standing.batch_id }}
                >
                  {`看${candidateTitle(reflection, standing.candidate)}的批次`}
                </Link>
              </p>
            )}
            {standing.skill_version_id && !adopted && (
              <p>
                <button
                  type="button"
                  aria-disabled={adopt.isPending}
                  onClick={() => {
                    if (!adopt.isPending)
                      adopt.mutate({
                        reflectionId: reflection.reflection_id,
                        input: standing.candidate,
                      });
                  }}
                >
                  {`採用${candidateTitle(reflection, standing.candidate)}`}
                </button>
              </p>
            )}
          </li>
        ))}
      </ul>
      <p role="status">
        {adopted
          ? `已採用${candidateTitle(reflection, adopted.candidate)}，它現在是這個小工具正式路線上的最新版本。`
          : evaluation.recommended !== null
            ? `建議採用${candidateTitle(reflection, evaluation.recommended)}：它在沒給模型看的題目上變好的幅度最大。`
            : settled
              ? "沒有候選在沒給模型看的題目上明顯變好，建議保留目前的版本。"
              : "候選的批次跑完後才會給建議。"}
      </p>
      {adopt.error && (
        <ReadFailure error={adopt.error} what="採用候選">
          <p role="alert">{userFacingReason(adopt.error) ?? "採用失敗，可以再試一次。"}</p>
        </ReadFailure>
      )}
    </section>
  );
}
