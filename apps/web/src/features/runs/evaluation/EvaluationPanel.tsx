import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useEvaluation, useEvaluationRevisions } from "../evaluation.service";
import { evaluationPanelState } from "./evaluation.model";
import { ExecutionState } from "./components/ExecutionState";
import { EvaluationReport } from "./components/EvaluationReport";
import { FeedbackForm } from "./components/FeedbackForm";
import { SuggestionsPanel } from "./components/SuggestionsPanel";
import { EvaluatingNotice } from "./components/EvaluatingNotice";
import { NotEvaluatedNotice } from "./components/NotEvaluatedNotice";
import { RevisionPicker } from "./components/RevisionPicker";

export function EvaluationPanel({ runId, runStatus }: { runId: string; runStatus?: string }) {
  const { evaluation: revision } = useSearch({ strict: false }) as { evaluation?: string };
  const navigate = useNavigate();
  const awaiting = runStatus === "succeeded" || runStatus === "failed";
  const evaluation = useEvaluation(runId, revision, awaiting);
  const revisions = useEvaluationRevisions(runId);
  const { notEvaluated, stoppedAsking, evaluating } = evaluationPanelState({
    runStatus,
    revision,
    error: evaluation.error,
    errorUpdateCount: evaluation.errorUpdateCount,
    dataStatus: evaluation.data?.status,
  });

  return (
    <section>
      <h2>任務判定</h2>

      {evaluation.isPending && !evaluating && <Loading what="評估結果" />}

      {evaluating && <EvaluatingNotice pendingPollStopped={evaluation.pendingPollStopped} />}

      {notEvaluated && !evaluating && (
        <NotEvaluatedNotice showPollingNote={awaiting && !revision} stoppedAsking={stoppedAsking} />
      )}

      {!notEvaluated && <ReadFailure error={evaluation.error} what="評估結果" />}

      {evaluation.data && !evaluating && (
        <EvaluationReport evaluation={evaluation.data} runStatus={runStatus} />
      )}

      {!evaluation.data && runStatus && <ExecutionState runStatus={runStatus} />}

      {revisions.data && revisions.data.revisions.length > 1 && (
        <RevisionPicker
          revisions={revisions.data.revisions}
          value={revision ?? ""}
          onChange={(evaluationId) =>
            void navigate({
              to: "/runs/$runId",
              params: { runId },
              search: (prev) => ({ ...prev, evaluation: evaluationId }),
            })
          }
        />
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
