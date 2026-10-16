import { Loading } from "../../../shared/ui/Loading";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useRuns, type RunListItem } from "../runs.service";
import {
  CLEANUP_BADGE,
  runActivityGroup,
  runAttentionAction,
  runStatusLabel,
  type RunActivityGroup,
} from "../runs.model";
import { RunVerdict } from "../components/RunVerdict";
import { RunSourceLinks } from "../components/RunSourceLinks";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { IN_FLIGHT_RUN_STATUSES } from "../trace.service";
import { canRetainLoadedPage } from "../../../core/api/client";
import { usePaginationFocus } from "../../../shared/ui/usePaginationFocus";

function loadMoreLabel(fetching: boolean, paused: boolean) {
  if (paused) return "等待連線…";
  return fetching ? "載入中…" : "載入更多";
}

export function WorkspaceRuns() {
  const runs = useRuns();
  const nextPageFailure = canRetainLoadedPage(runs.error, runs.isFetchNextPageError);
  const data = runs.error && !nextPageFailure ? undefined : runs.data;
  const rows = data?.pages.flatMap((page) => page.runs) ?? [];
  const paused = runs.fetchStatus === "paused";
  const busy = runs.fetchStatus !== "idle";
  const { loadMore, bindTarget, headingRef, row: continuationRow } = usePaginationFocus(runs, rows);

  return (
    <section>
      <h1 id="workspace-runs-heading" tabIndex={-1} ref={headingRef}>
        試跑活動
      </h1>
      <p className="note" data-role="caveat">
        僅收錄試跑；創作、打包、發佈各自保留原物件，可在 <Link to="/activity">活動</Link>
        一起查看。
      </p>

      {(runs.isPending || paused || (data && rows.length === 0 && busy)) && (
        <Loading what="試跑活動" paused={paused} />
      )}
      <ReadFailure
        error={runs.error}
        what="試跑活動"
        retry={() => void (nextPageFailure ? runs.fetchNextPage() : runs.refetch())}
        retrying={busy}
        focusTarget="workspace-runs-heading"
      >
        {nextPageFailure && (
          <p role="alert">更早的試跑讀取失敗；以下僅保留已載入的紀錄，尚未讀完。請再試一次。</p>
        )}
      </ReadFailure>
      {data && !runs.error && (
        <ListFreshness
          inFlight={rows.some((run) => IN_FLIGHT_RUN_STATUSES.has(run.status))}
          updatedAt={runs.dataUpdatedAt}
          fetching={runs.isFetching && !runs.isFetchingNextPage}
          paused={paused}
          refetch={async () => {
            const result = await runs.refetch();
            if (result.error) document.getElementById("workspace-runs-heading")?.focus();
          }}
        />
      )}

      {data &&
        (rows.length === 0 ? (
          !busy && (
            <p role="status">
              還沒有做過任何試跑。這裡是空的代表沒有發生過，不是紀錄被清掉了—— 要開始，請從{" "}
              <Link to="/lab/test-cases">測試題</Link> 建立一個再試跑。
            </p>
          )
        ) : (
          <>
            {ACTIVITY_GROUPS.map((group) => {
              const groupRows = rows.filter(
                (run) => runActivityGroup(run.status, run.evaluation.value) === group.key,
              );
              return (
                <section key={group.key}>
                  <h2>{group.title}</h2>
                  <p className="note">{group.note}</p>
                  {groupRows.length === 0 ? (
                    <p>
                      {runs.hasNextPage || nextPageFailure
                        ? "已載入的紀錄中沒有這類試跑；尚未讀完，無法確認全部紀錄。"
                        : group.empty}
                    </p>
                  ) : (
                    <ul className="download-list" data-role="evidence">
                      {groupRows.map((run) => (
                        <RunRow
                          key={run.run_id}
                          run={run}
                          action={group.action(run)}
                          focusRef={run === continuationRow ? bindTarget : undefined}
                        />
                      ))}
                    </ul>
                  )}
                </section>
              );
            })}
          </>
        ))}
      {runs.hasNextPage && !runs.error && (
        <button
          type="button"
          disabled={runs.isFetchingNextPage || paused}
          ref={bindTarget}
          onClick={loadMore}
        >
          {loadMoreLabel(runs.isFetchingNextPage, paused)}
        </button>
      )}
      <p className="note">
        <Link to="/workspace">回到工作區首頁</Link>｜<Link to="/activity">查看跨來源活動</Link>
      </p>
    </section>
  );
}

const REASON_EXPECTED = new Set(["failed", "cancelled", "timed_out"]);

const ACTIVITY_GROUPS: Array<{
  key: RunActivityGroup;
  title: string;
  note: string;
  empty: string;
  action: (run: RunListItem) => string;
}> = [
  {
    key: "needs_attention",
    title: "需要留意",
    note: "這些試跑有執行失敗、逾時，或結果尚需判斷；先查看事實，再決定下一步。",
    empty: "目前沒有需要你留意的試跑。",
    action: (run) => runAttentionAction(run.status, run.evaluation.value),
  },
  {
    key: "in_flight",
    title: "執行中",
    note: "平台仍在處理；離開後可以從這裡回來。",
    empty: "目前沒有正在執行的試跑。",
    action: () => "查看進度",
  },
  {
    key: "recent",
    title: "最近結束",
    note: "其他已結束的試跑，新的在上面。",
    empty: "目前沒有其他已結束的試跑。",
    action: () => "查看結果",
  },
];

function RunRow({
  run,
  action,
  focusRef,
}: {
  run: RunListItem;
  action: string;
  focusRef?: (element: HTMLLIElement | null) => void;
}) {
  return (
    <li className="download-item" ref={focusRef} tabIndex={focusRef ? -1 : undefined}>
      <p>
        <RunSourceLinks run={run} />｜
        <Link to="/runs/$runId" params={{ runId: run.run_id }}>
          {action}
        </Link>
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
