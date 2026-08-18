import { useState, type ReactNode } from "react";

/**
 * The two-step delete, in one place (04 丙-22).
 *
 * 02:WS-002 第 3 條 and 02:SEC-006 require a deletion to state its scope *before*
 * it runs, and this app now has three of them — a download package, a run's
 * output, and a skill — with an account deletion beside them. The first two grew
 * their own copy of the same markup; the third would have been the point where a
 * fix landed on two of the three. So the mechanism is shared and only the scope
 * sentence differs, which is the part that genuinely does: what survives a
 * deletion is different every time, and that difference is the whole disclosure.
 *
 * Two accessibility behaviours are load-bearing and neither is decoration:
 *
 * 1. `autoFocus` on 確認刪除. The button it replaces leaves the DOM, so without it
 *    focus falls back to `<body>` and a keyboard user has to tab in from the top
 *    of the page to reach a confirmation they just asked for (QA-009).
 * 2. `aria-describedby` pointing at the scope sentence, so the announcement
 *    carries the same fact the visible copy gives a sighted reader (NFR-007).
 *
 * The confirming flag is local state on purpose: each of these lives in a list
 * row, one row's confirmation is nothing to do with another's, and hoisting it
 * gave every caller the same "clear it on success" bookkeeping to get right. A
 * successful delete removes the row, which unmounts this; a failed one leaves it
 * confirming, which is where the user was.
 *
 * Renders a fragment rather than a wrapper so it drops into an existing `<p>`.
 */
export function ConfirmDelete({
  scopeId,
  scope,
  pending,
  onAsk,
  onConfirm,
  label = "刪除",
  confirmLabel = "確認刪除",
}: {
  /** Unique id for the scope sentence; `aria-describedby` points at it. */
  scopeId: string;
  /** What is destroyed and what survives it. Shown before anything happens. */
  scope: ReactNode;
  pending: boolean;
  /** Called when the first button is pressed — a page uses it to clear a stale status line. */
  onAsk?: () => void;
  onConfirm: () => void;
  label?: string;
  confirmLabel?: string;
}) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming) {
    return (
      <button
        type="button"
        onClick={() => {
          onAsk?.();
          setConfirming(true);
        }}
      >
        {label}
      </button>
    );
  }

  return (
    <>
      {/*
        The key is what makes `autoFocus` fire. React applies it on mount and
        only on mount, and an unkeyed fragment returned as a component's whole
        output is unwrapped by the reconciler — so this button would be matched
        against the 刪除 button it replaces, updated in place, and never mounted.
        A key it does not share forces the new DOM node the focus depends on.
      */}
      <button
        key="confirm"
        type="button"
        autoFocus
        aria-describedby={scopeId}
        disabled={pending}
        onClick={onConfirm}
      >
        {confirmLabel}
      </button>{" "}
      <button type="button" disabled={pending} onClick={() => setConfirming(false)}>
        取消
      </button>
      <span className="note" id={scopeId}>
        {scope}
      </span>
    </>
  );
}
