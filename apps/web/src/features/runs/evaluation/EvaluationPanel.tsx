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

function EvaluationReadState({
  query,
  state,
  revision,
}: {
  query: ReturnType<typeof useEvaluation>;
  state: ReturnType<typeof evaluationPanelState>;
  revision?: string;
}) {
  return (
    <>
      {(query.isPending || (query.error && query.isFetching)) && !state.evaluating && (
        <Loading what="評估結果" />
      )}
      {state.evaluating && <EvaluatingNotice pendingPollStopped={query.pendingPollStopped} />}
      {state.notEvaluated && !revision && !state.evaluating && (
        <NotEvaluatedNotice showPollingNote={state.awaiting} stoppedAsking={state.stoppedAsking} />
      )}
      {(!state.notEvaluated || revision) && (
        <ReadFailure
          error={query.isFetching ? undefined : query.error}
          what={revision ? "這份歷史評估" : "評估結果"}
          retry={() => void query.refetch()}
          retrying={query.isFetching}
          focusTarget="evaluation-heading"
        />
      )}
    </>
  );
}

export function EvaluationPanel({ runId, runStatus }: { runId: string; runStatus?: string }) {
  const { evaluation: revision } = useSearch({ strict: false }) as { evaluation?: string };
  const navigate = useNavigate();
  const awaiting = runStatus === "succeeded" || runStatus === "failed";
  const evaluation = useEvaluation(runId, revision, awaiting);
  const report = evaluation.error ? undefined : evaluation.data;
  const state = evaluationPanelState({
    runStatus,
    revision,
    error: evaluation.error,
    errorUpdateCount: evaluation.errorUpdateCount,
    dataStatus: report?.status,
    pending: evaluation.isPending,
  });
  const revisions = useEvaluationRevisions(runId, state.asksHistory);
  const selectRevision = (evaluationId: string | undefined) =>
    void navigate({
      to: "/runs/$runId",
      params: { runId },
      search: (prev) => ({ ...prev, evaluation: evaluationId }),
    }).then(() => document.getElementById("evaluation-heading")?.focus());

  return (
    <section>
      <h2 id="evaluation-heading" tabIndex={-1}>
        任務判定
      </h2>

      {revision && (
        <p>
          <button type="button" onClick={() => selectRevision(undefined)}>
            回到目前的判定
          </button>
        </p>
      )}

      <EvaluationReadState query={evaluation} state={state} revision={revision} />

      <ReadFailure
        error={revisions.error}
        what="歷史評估版本"
        retry={() => void revisions.refetch()}
        retrying={revisions.isFetching}
        focusTarget="evaluation-heading"
      />

      {revisions.data && !revisions.error && revisions.data.revisions.length > 1 && (
        <RevisionPicker
          revisions={revisions.data.revisions}
          value={revision ?? ""}
          onChange={selectRevision}
        />
      )}

      {report && !state.evaluating && (
        <EvaluationReport evaluation={report} runStatus={runStatus} />
      )}

      {!report && runStatus && <ExecutionState runStatus={runStatus} />}

      {report?.status === "completed" && (
        <SuggestionsPanel
          key={`suggestions:${report.evaluation_id}`}
          runId={runId}
          evaluationId={report.evaluation_id}
        />
      )}

      {report && (
        <FeedbackForm
          key={`feedback:${report.evaluation_id}`}
          runId={runId}
          evaluation={report}
          disabled={Boolean(report.superseded_at)}
        />
      )}
    </section>
  );
}
