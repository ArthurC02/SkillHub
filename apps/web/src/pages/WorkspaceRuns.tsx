import { Loading } from "../components/Loading";
import { Timestamp } from "../components/Timestamp";
import { ReadFailure } from "../components/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useRuns, type RunListItem } from "../api/runs";
import { runStatusLabel } from "./RunEvaluation";
import { RunVerdict } from "../components/RunVerdict";
import { ListFreshness } from "../components/ListFreshness";
import { IN_FLIGHT_RUN_STATUSES } from "../api/trace";

export function WorkspaceRuns() {
  const runs = useRuns();
  const rows = runs.data?.pages.flatMap((page) => page.runs) ?? [];

  return (
    <section>
      <h1>Run 歷史</h1>
      <p className="note" data-role="teaching">
        這個工作區跑過的 Run，新的在上面。
      </p>

      {runs.isPending && <Loading what=" Run 歷史" />}
      <ReadFailure error={runs.error} what=" Run 歷史" />
      {runs.data && (
        <ListFreshness
          inFlight={rows.some((run) => IN_FLIGHT_RUN_STATUSES.has(run.status))}
          updatedAt={runs.dataUpdatedAt}
          fetching={runs.isFetching && !runs.isFetchingNextPage}
          refetch={runs.refetch}
        />
      )}

      {runs.data &&
        (rows.length === 0 ? (
          <p>
            還沒有跑過任何 Run。這裡是空的代表沒有發生過，不是紀錄被清掉了—— 要開始，請從{" "}
            <Link to="/lab/test-cases">Test Case</Link> 建立一個再試跑。
          </p>
        ) : (
          <ul className="download-list">
            {rows.map((run) => (
              <RunRow key={run.run_id} run={run} />
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
    </section>
  );
}

export const CLEANUP_BADGE: Record<string, string> = {
  pending: "badge badge-unverified",
  cleaning_up: "badge badge-unverified",
  cleaned: "badge",
  failed: "badge badge-danger",
};

const REASON_EXPECTED = new Set(["failed", "cancelled", "timed_out"]);

function RunRow({ run }: { run: RunListItem }) {
  return (
    <li className="download-item">
      <p>
        <Link to="/runs/$runId" params={{ runId: run.run_id }}>
          <strong>{run.skill_name}</strong>
        </Link>{" "}
      </p>
      <p className="badge-row">
        <RunVerdict verdict={run.evaluation} />
      </p>
      <p className="badge-row">
        <span className="badge">執行狀態：{runStatusLabel(run.status)}</span>{" "}
        <span className={CLEANUP_BADGE[run.cleanup_status.value] ?? "badge badge-unverified"}>
          清理狀態：{run.cleanup_status.label}
        </span>{" "}
        <span className="note">{run.cleanup_status.note}</span>
      </p>
      {run.status_reason ? (
        <p className="note">{run.status_reason}</p>
      ) : (
        REASON_EXPECTED.has(run.status) && (
          <p className="note">伺服器沒有給這個狀態的原因，不是原因被省略顯示。</p>
        )
      )}
      <p className="note">
        建立於 <Timestamp at={run.created_at} />
        {run.finished_at ? (
          <>
            ｜結束於 <Timestamp at={run.finished_at} />
          </>
        ) : (
          "｜尚未結束"
        )}
        ｜Provider {run.provider}
        {run.failure_class
          ? `｜失敗類別 ${run.failure_class.label}`
          : run.status === "failed"
            ? "｜失敗類別未記錄"
            : ""}
      </p>
      {run.failure_class && <p className="note">{run.failure_class.note}</p>}
    </li>
  );
}
