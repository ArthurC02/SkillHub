import { useEffect, useRef, useState, type ReactNode } from "react";

/**
 * The two-step delete, in one place (04 丙-22).
 *
 * 02:WS-002 第 3 條 and 02:SEC-006 require a deletion to state its scope *before*
 * it runs, and this app now has seven of them — a download package, a run's
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
  const trigger = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  useEffect(() => {
    if (!confirming && restoreFocus.current) {
      restoreFocus.current = false;
      trigger.current?.focus();
    }
  }, [confirming]);

  if (!confirming) {
    return (
      <button
        ref={trigger}
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
        design §2.8: the scope sentence *is* the disclosure, so it renders before
        the two buttons rather than after them. `aria-describedby` already got a
        screen-reader user to it from either position — the association is by id,
        not by DOM order — but nothing got a sighted user there, whose eye lands
        on 確認刪除, which `autoFocus` has already focused. The disclosure has to
        be upstream of the control it justifies for both readers, not one.
      */}
      <span className="note confirm-scope" id={scopeId}>
        {scope}
      </span>{" "}
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
        {/*
          design §2.4: a disabled control with no stated cause reads as a bug,
          and the cause is the honest part of the feature. Both buttons go dead
          while the request is in flight and neither said why — on the component
          that is this app's reference implementation of careful destructive UX,
          and which every other destructive action in the app routes through.
          The label carries the reason, the way ImportSkill and WorkspaceRuns
          already do. 送出中 rather than 刪除中 because one caller
          (WorkspaceAccount) starts a grace period rather than deleting, and a
          shared control must not word a promise only six of seven callers keep.
        */}
        {pending ? "送出中…" : confirmLabel}
      </button>{" "}
      <button
        type="button"
        disabled={pending}
        onClick={() => {
          restoreFocus.current = true;
          setConfirming(false);
        }}
      >
        {pending ? "已送出，無法取消" : "取消"}
      </button>
    </>
  );
}
