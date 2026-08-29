import type { Labelled } from "../api/types";

/**
 * The task verdict as it appears on a list row (ADR-025, 設計 §2.5, 04 丙-32).
 *
 * One component because there are two run histories — the workspace's and one
 * test case's — and they are the same fact about the same run. They used to
 * carry one axis each plus a sentence apologising for it.
 *
 * The wording is entirely the server's (§4.4). This side owns only the tint,
 * and the tint is the second channel, never the first (§2.3).
 */

/*
 * §4.4: `--danger` means 這件事不通過 and `--accent-border` means 這件事未知/未驗證.
 * Only `not_met` is the first. **`evaluation_failed` is emphatically not** — it
 * says nobody judged, and colouring it as a failure would put the misreading
 * back that the note spends a sentence removing. 「部分符合」 and 「無法判斷」 share
 * the unknown tint because there are two tints and four states; §5.3 records
 * that trade rather than inventing a third colour.
 */
export const VERDICT_BADGE: Record<string, string> = {
  met: "badge",
  not_met: "badge badge-danger",
};

/*
 * The note renders for the states where the label alone can be misread, and not
 * for the three that speak for themselves — the same shape as `REASON_EXPECTED`
 * one file over. On a page of fifty rows a sentence per row is noise, and 04
 * 丙-29 裁定① rejected exactly that for `status`; the difference here is that
 * these four sentences are load-bearing (「評估失敗」 reads as "the task failed"
 * and is not, 「無法判斷」 is a verdict the judge reached and 「未評估」 is not).
 */
const NOTE_SHOWN = new Set(["not_evaluated", "evaluating", "evaluation_failed", "undetermined"]);

export function RunVerdict({ verdict }: { verdict: Labelled }) {
  return (
    <>
      <span className={VERDICT_BADGE[verdict.value] ?? "badge badge-unverified"}>
        任務判定：{verdict.label}
      </span>
      {NOTE_SHOWN.has(verdict.value) && <span className="note">{verdict.note}</span>}
    </>
  );
}
