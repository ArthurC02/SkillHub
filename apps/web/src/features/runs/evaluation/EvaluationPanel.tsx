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

function focusEvaluationHeading() {
  const heading = document.getElementById("evaluation-heading");
  heading?.focus();
  heading?.scrollIntoView({ block: "center" });
}

function EvaluationReadState({
  query,
  state,
  revision,
}: {
  query: ReturnType<typeof useEvaluation>;
  state: ReturnType<typeof evaluationPanelState>;
  revision?: string;
}) {
  const stopped = query.pendingPollStopped || query.initialPollStopped;
  const retrying = query.fetchStatus !== "idle";
  const reread = () => {
    void query.refetch();
    focusEvaluationHeading();
  };
  return (
    <>
      {(query.isPending || query.error) && query.isFetching && !state.evaluating && (
        <Loading what="評估結果" />
      )}
      {query.fetchStatus === "paused" && (
        <p role="status">等待連線以確認評估結果；恢復連線後會繼續讀取。</p>
      )}
      {state.evaluating && (
        <EvaluatingNotice
          pendingPollStopped={query.pendingPollStopped}
          updatedAt={query.dataUpdatedAt}
        />
      )}
      {query.pendingPollStopped && !state.evaluating && (
        <p role="status">已停止自動確認評估結果。請重新確認目前狀態。</p>
      )}
      {state.notEvaluated && !revision && !state.evaluating && (
        <NotEvaluatedNotice showPollingNote={state.awaiting} stoppedAsking={state.stoppedAsking} />
      )}
      {(!state.notEvaluated || revision) && (
        <ReadFailure
          error={query.isFetching ? undefined : query.error}
          what={revision ? "這份歷史評估" : "評估結果"}
          retry={stopped ? undefined : reread}
          retrying={retrying}
        />
      )}
      {stopped && (
        <button type="button" disabled={retrying} onClick={reread}>
          {retrying ? "重新讀取中…" : "重新確認評估結果"}
        </button>
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
    dataStatus: evaluation.data?.status,
    pending: evaluation.isPending,
  });
  const revisions = useEvaluationRevisions(runId, state.asksHistory);
  const selectRevision = (evaluationId: string | undefined) =>
    void navigate({
      to: "/runs/$runId",
      params: { runId },
      search: (prev) => ({ ...prev, evaluation: evaluationId }),
    }).then(focusEvaluationHeading);

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
        retry={() => {
          void revisions.refetch();
          focusEvaluationHeading();
        }}
        retrying={revisions.fetchStatus !== "idle"}
      />
      {revisions.fetchStatus === "paused" && (
        <p role="status">等待連線以確認歷史評估版本；恢復連線後會繼續讀取。</p>
      )}

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
