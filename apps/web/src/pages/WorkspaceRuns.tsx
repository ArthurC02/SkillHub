import { Link } from "@tanstack/react-router";
import { useRuns, type RunListItem } from "../api/runs";
import { RUN_STATUS_LABEL } from "./RunEvaluation";

/**
 * 02:WS-002 第 1 條「Run 歷史」, served by GET /runs (WS-004).
 *
 * The one rule this page exists to keep: **`succeeded` is worded as execution
 * and never as a pass** (ADR-025). A history list is where that mistake is
 * cheapest to make and most expensive to read — a column of green 成功 would
 * tell the owner their tasks were done, when what finished was the workload.
 * The task verdict lives on the run's own page, behind the evaluation.
 *
 * `cleanup_status` is on the row for the same reason it is a column in the
 * database: a run whose sandbox was never cleaned up is a fact about this
 * workspace, and hiding it until someone opens the run would make the list a
 * prettier answer than the truth.
 */
export function WorkspaceRuns() {
  const runs = useRuns();
  const rows = runs.data?.pages.flatMap((page) => page.runs) ?? [];

  return (
    <section className="page">
      <h1>Run 歷史</h1>
      <p className="note">
        這個工作區跑過的 Run，新的在上面。
        <strong>這裡寫的是「執行狀態」，不是「任務有沒有做到」</strong>
        ——後者是評估的判定，在各自的 Run 頁面上。
      </p>

      {runs.isPending && <p>載入 Run 歷史中…</p>}
      {runs.error && <p role="alert">無法讀取 Run 歷史：{runs.error.message}</p>}

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
        <button type="button" disabled={runs.isFetchingNextPage} onClick={() => runs.fetchNextPage()}>
          {runs.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}
    </section>
  );
}

const CLEANUP_LABEL: Record<RunListItem["cleanup_status"], string> = {
  pending: "尚未清理",
  cleaning: "清理中",
  cleaned: "已清理",
  failed: "清理失敗",
};

function RunRow({ run }: { run: RunListItem }) {
  return (
    <li className="download-item">
      <p>
        <Link to="/runs/$runId" params={{ runId: run.run_id }}>
          <strong>{run.skill_name}</strong>
        </Link>{" "}
        <span className="badge">執行狀態：{RUN_STATUS_LABEL[run.status] ?? run.status}</span>
        {run.cleanup_status !== "cleaned" && (
          <>
            {" "}
            <span className="badge badge-unverified">{CLEANUP_LABEL[run.cleanup_status]}</span>
          </>
        )}
      </p>
      {run.status_reason && <p className="note">{run.status_reason}</p>}
      <p className="note">
        建立於 {run.created_at}
        {run.finished_at ? `｜結束於 ${run.finished_at}` : "｜尚未結束"}｜Provider {run.provider}
        {run.failure_class ? `｜失敗類別 ${run.failure_class}` : ""}
      </p>
      <p className="note">
        <code>{run.status}</code> 說的是工作負載跑完了沒有。任務達成與否要看這個 Run 的評估判定。
      </p>
    </li>
  );
}
