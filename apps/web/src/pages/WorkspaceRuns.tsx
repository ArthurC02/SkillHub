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
 * design §4.4 wants badge wording to come from the server, so the two planes
 * cannot word one fact two ways (02:NFR-001) — and for this one the server has
 * no wording to give: `RunListItem.cleanup_status` in
 * contracts/openapi/public.yaml is a bare enum with no label beside it, unlike
 * e.g. `tier` or `trust`, which ship `{value, label, note}`. **That is a
 * contract gap, not a choice made here**: deleting this map would put `pending`
 * on screen in English, and 02:NFR-007 forbids taking text away. It stays keyed
 * on the union so a new server value is a compile error here rather than an
 * `undefined` on the row, which is the same guard RUN_STATUS_LABEL carries.
 */
const CLEANUP_LABEL: Record<RunListItem["cleanup_status"], string> = {
  pending: "尚未清理",
  cleaning: "清理中",
  cleaned: "已清理",
  failed: "清理失敗",
};

/*
 * §4.4 assigns `--accent-border` to 這件事未知／未驗證 and `--danger` to 這件事不
 * 通過. 尚未清理 and 清理中 are genuinely the first; **清理失敗 is the second**,
 * and giving all three the same tint made one visual mean both "pending" and
 * "failed". The word is still the fact and the tint is the second channel
 * (§2.3), so this only stops the colour from contradicting the word.
 */
const CLEANUP_BADGE: Record<RunListItem["cleanup_status"], string> = {
  pending: "badge badge-unverified",
  cleaning: "badge badge-unverified",
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
        <span className="badge">執行狀態：{RUN_STATUS_LABEL[run.status] ?? run.status}</span>{" "}
        {/* §2.1: 已清理 used to render as nothing at all, so the majority case —
            the sandbox was torn down, which is a security fact the owner wants —
            was indistinguishable from a field that was never rendered. This
            file's own header argues cleanup belongs on the row because hiding it
            would make the list a prettier answer than the truth; the guard was
            hiding it for exactly the runs where the answer is good. */}
        <span className={CLEANUP_BADGE[run.cleanup_status]}>
          清理狀態：{CLEANUP_LABEL[run.cleanup_status]}
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
      <p className="note">
        <code>{run.status}</code> 說的是工作負載跑完了沒有。任務達成與否要看這個 Run 的評估判定。
      </p>
    </li>
  );
}
