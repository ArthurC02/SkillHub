import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useRuns, type RunListItem } from "../../../runs";
import { RunVerdict } from "../../../runs";
import { ListFreshness } from "../../../../shared/ui/ListFreshness";
import { IN_FLIGHT_RUN_STATUSES } from "../../../runs";
import { runStatusLabel } from "../../../runs";

export function RunHistory({
  runs,
  history,
}: {
  runs: ReturnType<typeof useRuns>;
  history: RunListItem[];
}) {
  return (
    <>
      <h2>執行歷史</h2>
      {runs.isPending && <Loading what="執行歷史" />}
      <ReadFailure error={runs.error} what="執行歷史" />
      {runs.data && (
        <ListFreshness
          inFlight={history.some((run) => IN_FLIGHT_RUN_STATUSES.has(run.status))}
          updatedAt={runs.dataUpdatedAt}
          fetching={runs.isFetching && !runs.isFetchingNextPage}
          refetch={runs.refetch}
        />
      )}
      {runs.data &&
        (history.length === 0 ? (
          <p>尚無執行。這個 Test Case 還沒有跑過任何 Run。</p>
        ) : (
          <ul className="download-list" data-role="evidence">
            {history.map((run) => (
              <li key={run.run_id} className="download-item">
                <p>
                  <Link to="/runs/$runId" params={{ runId: run.run_id }}>
                    <Timestamp at={run.created_at} />
                  </Link>
                </p>
                <p className="badge-row">
                  <RunVerdict verdict={run.evaluation} />
                </p>
                <p className="badge-row">
                  <span className="badge">執行狀態：{runStatusLabel(run.status)}</span>
                </p>
                {(run.status_reason || run.failure_class) && (
                  <p className="note">
                    {run.status_reason ?? "未測量（伺服器沒有回報原因）"}
                    {run.failure_class && `（分類：${run.failure_class.label}）`}
                  </p>
                )}
                <p className="note">
                  {run.finished_at ? (
                    <>
                      結束於 <Timestamp at={run.finished_at} />
                    </>
                  ) : (
                    "尚未結束"
                  )}
                </p>
                <details>
                  <summary>Skill Version</summary>
                  <code>{run.skill_version_id}</code>
                </details>
              </li>
            ))}
          </ul>
        ))}
      {runs.hasNextPage && (
        <button
          type="button"
          disabled={runs.isFetchingNextPage}
          onClick={() => runs.fetchNextPage()}
        >
          {runs.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}
    </>
  );
}
