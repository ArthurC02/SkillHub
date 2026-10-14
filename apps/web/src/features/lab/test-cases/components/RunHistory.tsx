import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { formatAt } from "../../../../shared/ui/Timestamp.model";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useRuns, type RunListItem } from "../../../runs";
import { RunVerdict } from "../../../runs";
import { ListFreshness } from "../../../../shared/ui/ListFreshness";
import { IN_FLIGHT_RUN_STATUSES } from "../../../runs";
import { runStatusLabel } from "../../../runs";
import { canRetainLoadedPage } from "../../../../core/api/client";

export function RunHistory({
  runs,
  history,
}: {
  runs: ReturnType<typeof useRuns>;
  history: RunListItem[];
}) {
  const nextPageFailure = canRetainLoadedPage(runs.error, runs.isFetchNextPageError);
  const readable = runs.data && (!runs.error || nextPageFailure);
  return (
    <>
      <h2 id="test-case-history-heading" tabIndex={-1}>
        執行歷史
      </h2>
      {runs.isPending && <Loading what="執行歷史" />}
      <ReadFailure
        error={runs.error}
        what="執行歷史"
        retry={() => void (nextPageFailure ? runs.fetchNextPage() : runs.refetch())}
        retrying={runs.isFetching}
        focusTarget="test-case-history-heading"
      >
        {nextPageFailure && (
          <p role="alert">更早的執行歷史讀取失敗；以下僅保留已載入的紀錄，尚未讀完。請再試一次。</p>
        )}
      </ReadFailure>
      {readable && !runs.error && (
        <ListFreshness
          inFlight={history.some((run) => IN_FLIGHT_RUN_STATUSES.has(run.status))}
          updatedAt={runs.dataUpdatedAt}
          fetching={runs.isFetching && !runs.isFetchingNextPage}
          refetch={async () => {
            const result = await runs.refetch();
            if (result.error) document.getElementById("test-case-history-heading")?.focus();
          }}
        />
      )}
      {readable &&
        (history.length === 0 ? (
          <p>尚無執行。這個測試題還沒有做過任何試跑。</p>
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
                <p>
                  <Link
                    to="/skills/$skillId/versions/$versionId"
                    params={{ skillId: run.skill_id, versionId: run.skill_version_id }}
                  >
                    查看這次的版本
                  </Link>{" "}
                  ·{" "}
                  <Link
                    to="/runs/$runId/compare"
                    params={{ runId: run.run_id }}
                    aria-label={`以建立於 ${formatAt(run.created_at)} 的試跑紀錄開始比較`}
                  >
                    以這次試跑開始比較
                  </Link>
                </p>
                <details>
                  <summary>小工具 Version</summary>
                  <code>{run.skill_version_id}</code>
                </details>
              </li>
            ))}
          </ul>
        ))}
      {runs.hasNextPage && !runs.error && (
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
