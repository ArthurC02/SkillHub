import { Loading } from "../components/Loading";
import { Timestamp } from "../components/Timestamp";
import { ReadFailure } from "../components/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useRuns, type RunListItem } from "../api/runs";
import { runStatusLabel } from "./RunEvaluation";
import { RunVerdict } from "../components/RunVerdict";
import { ListFreshness } from "../components/ListFreshness";
import { IN_FLIGHT_RUN_STATUSES } from "../api/trace";

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
      {/*
        設計 §2.13,D 類。這裡原本用 94 字重講一次 ADR-025 的定義,而 §2.5 要的是
        「兩列、判定在前」這個**版面事實**——那由每一列自己的兩個徽章帶著主詞說出來
        （任務判定：／執行狀態：）,不是由這段字說出來。兩列一個字都沒有動。
      */}
      <p className="note" data-role="teaching">
        這個工作區跑過的 Run，新的在上面。
      </p>

      {runs.isPending && <Loading what=" Run 歷史" />}
      <ReadFailure error={runs.error} what=" Run 歷史" />
      {/* 設計 §2.12 第 6 條: a list with a running row is an in-flight screen. */}
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
export const CLEANUP_BADGE: Record<string, string> = {
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
          list until 04 丙-32 landed the field. Each badge carries its own subject
          （任務判定：／執行狀態：）, which is what makes the two rows readable
          without the paragraph that used to define them above the list (§2.13). */}
      <p className="badge-row">
        <RunVerdict verdict={run.evaluation} />
      </p>
      <p className="badge-row">
        <span className="badge">執行狀態：{runStatusLabel(run.status)}</span>{" "}
        {/* §2.1: 已清理 used to render as nothing at all, so the majority case —
            the sandbox was torn down, which is a security fact the owner wants —
            was indistinguishable from a field that was never rendered. This
            file's own header argues cleanup belongs on the row because hiding it
            would make the list a prettier answer than the truth; the guard was
            hiding it for exactly the runs where the answer is good. */}
        <span className={CLEANUP_BADGE[run.cleanup_status.value] ?? "badge badge-unverified"}>
          清理狀態：{run.cleanup_status.label}
        </span>{" "}
        {/* The server's own qualifier, visible rather than `title`-only
            (設計 §2.4 第 3 項). 清理狀態 is a security fact about a sandbox that
            may or may not still exist, and 「已清理」 alone does not say what was
            torn down.

            The badge above carried the SAME string as a `title=` as well, which
            is 設計 §2.13 第 2 條's named instance: §2.4 says the reason may not
            live only in a tooltip, not that it must exist twice. The visible copy
            is the one that survives a touch device, so the `title` went. */}
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
        {/* The server's own words (04 丙-115 ②). This read
            「｜失敗類別 capability_mismatch」 — a raw enum inside a Chinese
            sentence, on the screen a reader opens to find out what went wrong. */}
        {run.failure_class
          ? `｜失敗類別 ${run.failure_class.label}`
          : run.status === "failed"
            ? "｜失敗類別未記錄"
            : ""}
      </p>
      {/*
        Visible, not a `title` — 設計 §2.4 第 3 項, the same call the cleanup
        note above makes, and design-system.test.ts refuses the tooltip-only
        shape outright. The note is where the class stops being read wrong:
        `workload_error` is the Skill failing at its own job and reads as a
        platform fault, `capability_mismatch` is a refusal before anything ran
        and reads as a crash.
      */}
      {run.failure_class && <p className="note">{run.failure_class.note}</p>}
      {/* The per-row restatement is gone: it said what the verdict badge above
          it now says, and 設計 §3 第 14 條 counts a fact worded twice on one row
          as a defect rather than as emphasis. */}
    </li>
  );
}
