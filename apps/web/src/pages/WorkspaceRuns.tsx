import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useRuns, type RunListItem } from "../api/runs";
import { RUN_STATUS_LABEL } from "./RunEvaluation";
import { RunVerdict } from "../components/RunVerdict";

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
    <section>
      <h1>Run 歷史</h1>
      <p className="note">
        這個工作區跑過的 Run，新的在上面。每一列有兩軸：
        <strong>任務判定在前，執行狀態在後</strong>
        ——前者說任務有沒有做到，後者只說工作負載跑完了沒有（ADR-025）。逐條驗收結果在各自的 Run
        頁面上。
      </p>

      {runs.isPending && <Loading what=" Run 歷史" />}
      <ReadFailure error={runs.error} what=" Run 歷史" />

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

/*
 * The label map is gone (04 丙-29 ②). §4.4 wants badge wording from the server so
 * the two planes cannot word one fact two ways (02:NFR-001), and the reason it
 * lived here was a contract gap: `RunListItem.cleanup_status` was a bare enum
 * with no label beside it, unlike `tier` or `trust`. Deleting the map without
 * closing that gap would have put `pending` on screen in English, which
 * 02:NFR-007 forbids.
 *
 * The gap is closed, and closing it also removed the failure the map could not
 * survive: it was keyed on a union that said `cleaning` where the database says
 * `cleaning_up`, so being total over the union proved nothing and a run
 * mid-teardown rendered a **blank**. The server now sends the words for whatever
 * value it holds — an unrecognised one keeps the raw value as its own label
 * rather than becoming the blank §2.9 forbids.
 *
 * Only the tint stays. §4.4 assigns `--accent-border` to 這件事未知／未驗證 and
 * `--danger` to 這件事不通過: 尚未清理 and 清理中 are the first, **清理失敗 is the
 * second**, and one tint for all three made a single visual mean both. The word
 * is the fact and the tint is the second channel (§2.3); presentation is this
 * side's job, wording is not.
 */
const CLEANUP_BADGE: Record<string, string> = {
  pending: "badge badge-unverified",
  cleaning_up: "badge badge-unverified",
  cleaned: "badge",
  failed: "badge badge-danger",
};

/*
 * §2.1. `status_reason` is "why the run entered this status" — on these three it
 * is the whole explanation, so its absence is a fact worth a sentence rather
 * than a blank. Not on the others: `succeeded` and the in-flight states normally
 * carry no reason, and 「沒有記錄原因」 on every row of a healthy list is noise,
 * not evidence.
 */
const REASON_EXPECTED = new Set(["failed", "cancelled", "timed_out"]);

function RunRow({ run }: { run: RunListItem }) {
  return (
    <li className="download-item">
      <p>
        <Link to="/runs/$runId" params={{ runId: run.run_id }}>
          <strong>{run.skill_name}</strong>
        </Link>{" "}
      </p>
      {/* §2.5 wants both axes and the verdict first. It was a footnote under the
          list until 04 丙-32 landed the field — the note above still says what
          the two axes are, because the order alone does not teach anyone the
          difference. */}
      <p className="badge-row">
        <RunVerdict verdict={run.evaluation} />
      </p>
      <p className="badge-row">
        <span className="badge">執行狀態：{RUN_STATUS_LABEL[run.status] ?? run.status}</span>{" "}
        {/* §2.1: 已清理 used to render as nothing at all, so the majority case —
            the sandbox was torn down, which is a security fact the owner wants —
            was indistinguishable from a field that was never rendered. This
            file's own header argues cleanup belongs on the row because hiding it
            would make the list a prettier answer than the truth; the guard was
            hiding it for exactly the runs where the answer is good. */}
        <span
          className={CLEANUP_BADGE[run.cleanup_status.value] ?? "badge badge-unverified"}
          title={run.cleanup_status.note}
        >
          清理狀態：{run.cleanup_status.label}
        </span>
      </p>
      {run.status_reason ? (
        <p className="note">{run.status_reason}</p>
      ) : (
        REASON_EXPECTED.has(run.status) && (
          <p className="note">伺服器沒有給這個狀態的原因，不是原因被省略顯示。</p>
        )
      )}
      <p className="note">
        建立於 {run.created_at}
        {run.finished_at ? `｜結束於 ${run.finished_at}` : "｜尚未結束"}｜Provider {run.provider}
        {run.failure_class
          ? `｜失敗類別 ${run.failure_class}`
          : run.status === "failed"
            ? "｜失敗類別未記錄"
            : ""}
      </p>
      {/* The per-row restatement is gone: it said what the verdict badge above
          it now says, and 設計 §3 第 14 條 counts a fact worded twice on one row
          as a defect rather than as emphasis. */}
    </li>
  );
}
