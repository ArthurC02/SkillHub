import { Loading } from "../../../shared/ui/Loading";
import { formatAt } from "../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import {
  EVALUATION_POLL_MAX_404,
  EVALUATION_POLL_MAX_PENDING,
  useEvaluation,
  useEvaluationRevisions,
} from "../evaluation.service";
import { OVERALL_LABEL } from "./evaluation.model";
import { ExecutionState } from "./components/ExecutionState";
import { EvaluationReport } from "./components/EvaluationReport";
import { FeedbackForm } from "./components/FeedbackForm";
import { SuggestionsPanel } from "./components/SuggestionsPanel";

export function EvaluationPanel({ runId, runStatus }: { runId: string; runStatus?: string }) {
  const { evaluation: revision } = useSearch({ strict: false }) as { evaluation?: string };
  const navigate = useNavigate();
  const awaiting = runStatus === "succeeded" || runStatus === "failed";
  const evaluation = useEvaluation(runId, revision, awaiting);
  const revisions = useEvaluationRevisions(runId);
  const notEvaluated = evaluation.error instanceof ApiError && evaluation.error.status === 404;
  const stoppedAsking = notEvaluated && evaluation.errorUpdateCount >= EVALUATION_POLL_MAX_404;
  const evaluating = !revision && evaluation.data?.status === "pending";

  return (
    <section>
      <h2>任務判定</h2>

      {evaluation.isPending && !evaluating && <Loading what="評估結果" />}

      {evaluating && (
        <div className="notice" role="status">
          <p>
            <strong>評估進行中</strong>
            {evaluation.pendingPollStopped
              ? "——這一筆評估說自己還在做，但這一頁已經停止再查了。"
              : "——判定還在做。它會自己完成，不需要你回來按任何東西。"}
          </p>
          <p className="note">可以關掉這一頁（平台在跑，不是你的瀏覽器）</p>
          <p className="note">
            這一段沒有進度可以報——評審不是分批完成的，它要嘛給出判定要嘛失敗，
            而兩種結果都會出現在這裡。
            {evaluation.pendingPollStopped ? (
              <>
                這一頁查了 {EVALUATION_POLL_MAX_PENDING} 次、約{" "}
                {(EVALUATION_POLL_MAX_PENDING * 3) / 60}{" "}
                分鐘，狀態都還是「進行中」，所以它不會再自己更新了。
                停下來的是這一頁的查詢，不是那個 job：重新整理這一頁會再查一次。
                一直停在這裡代表那個工作沒有在推進，而不是判定為未通過。
              </>
            ) : (
              "這一頁每 3 秒自己查一次。"
            )}
          </p>
        </div>
      )}

      {notEvaluated && !evaluating && (
        <div className="notice">
          <p>
            <strong>未評估</strong>
          </p>
          <p>這個 Run 沒有評估結果。未評估不等於通過，也不等於未通過。</p>
          {awaiting && !revision && !stoppedAsking && (
            <p className="note">
              這一頁每 3 秒再查一次；如果有評估正在排隊，結果會自己出現在這裡，不必重新整理。
            </p>
          )}
          {awaiting && !revision && stoppedAsking && (
            <p className="note">
              這一頁已經停止再查了——查了 {EVALUATION_POLL_MAX_404}{" "}
              次都還是沒有評估，所以它不會再自己更新。 這通常表示沒有人替這個 Run
              送出評估，而不是評估失敗。重新整理這一頁會再查一次。
            </p>
          )}
        </div>
      )}

      {!notEvaluated && <ReadFailure error={evaluation.error} what="評估結果" />}

      {evaluation.data && !evaluating && (
        <EvaluationReport evaluation={evaluation.data} runStatus={runStatus} />
      )}

      {!evaluation.data && runStatus && <ExecutionState runStatus={runStatus} />}

      {revisions.data && revisions.data.revisions.length > 1 && (
        <p>
          <label htmlFor="evaluation-revision">評估版本</label>{" "}
          <select
            id="evaluation-revision"
            value={revision ?? ""}
            onChange={(e) =>
              void navigate({
                to: "/runs/$runId",
                params: { runId },
                search: (prev) => ({
                  ...prev,
                  evaluation: e.target.value === "" ? undefined : e.target.value,
                }),
              })
            }
          >
            <option value="">目前的判定</option>
            {revisions.data.revisions.map((r) => (
              <option key={r.evaluation_id} value={r.evaluation_id}>
                {formatAt(r.evaluated_at)}｜{OVERALL_LABEL[r.overall]}｜prompt{" "}
                {r.judge_prompt_version}
                {r.rubric_version ? `｜rubric ${r.rubric_version}` : ""}
                {r.superseded_at ? "（已被取代）" : ""}
              </option>
            ))}
          </select>
        </p>
      )}

      {evaluation.data && evaluation.data.status === "completed" && (
        <SuggestionsPanel runId={runId} />
      )}

      {evaluation.data && (
        <FeedbackForm
          runId={runId}
          evaluation={evaluation.data}
          disabled={Boolean(evaluation.data.superseded_at)}
        />
      )}
    </section>
  );
}
