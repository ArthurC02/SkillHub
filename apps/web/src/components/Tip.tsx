import { useId, useState, type KeyboardEvent, type ReactNode } from "react";

/**
 * 設計 §1.3 的第四種揭露機制（ADR-065 決策 3）：點擊觸發的 toggletip。
 *
 * It hides a WHY, never a WHAT. `<details>` folds a detail that belongs to a
 * fact on this page (a hash, a version id); a Tip folds the reason behind a
 * sentence that has already been said in full. The test in §1.3 is the one to
 * apply before reaching for it: delete every Tip on the page and every
 * decision must still be makeable. Anything that fails that test is §2.10
 * content and does not go in here — `a11y.test.tsx` and `detail.test.tsx`
 * check the closed list against `[data-tip]`.
 *
 * Shape is ARIA APG *Disclosure*, not Tooltip (§2.13 第 2 條): the trigger is a
 * real `<button>` with visible text, `aria-expanded` and `aria-controls`; the
 * content is in the DOM from the first render, `hidden` until pressed; Esc
 * closes and focus stays on the button. There is no hover path at all —
 * §2.4 第 3 項 was written about text that lived only in `title`, and a hover
 * tooltip is that failure with better styling.
 *
 * The content is positioned, so opening it does not move a neighbour (§2.13
 * 第 2 條: 「開合不改變其他內容的版面高度——會改的就是 `<details>`」). It is a
 * `<p>` on purpose (§4.7): `main p` is what the 40em measure and the e2e
 * line-length assertion select, and a `<div>` would be a new corner nobody
 * measures.
 *
 * `data-role="teaching"` marks the content as 設計 §2.13 class D so the budget
 * machine in `e2e/rendered.spec.ts` can find it — folded here it counts as
 * zero flat characters, which is the whole point of moving it.
 */
export function Tip({
  anchor,
  children,
}: {
  /** Visible trigger text. Must stand on its own (§2.13 第 3 條): a subject or a
   *  quantity, never 「詳情」「說明」「?」. */
  anchor: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const id = useId();

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key === "Escape" && open) {
      e.stopPropagation();
      setOpen(false);
    }
  };

  return (
    <span className="tip" data-tip onKeyDown={onKeyDown}>
      <button
        type="button"
        className="tip-trigger"
        aria-expanded={open}
        aria-controls={id}
        // WebKit does not focus a button on click (Safari's long-standing
        // behaviour; the Windows and macOS ports, not the Linux one CI runs).
        // Without the focus the keydown below fires on <body>, outside this
        // span, and Escape closes nothing. The APG shape says focus stays on
        // the trigger, so put it there ourselves rather than trust the engine.
        onClick={(e) => {
          e.currentTarget.focus();
          setOpen((v) => !v);
        }}
      >
        {/* §4.7: one of the app's at-most-six shapes, the Tip trigger's own.
            Decorative — the visible text is the name; the shape only says
            「this opens」 at a glance. */}
        <svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12" focusable="false">
          <circle cx="6" cy="6" r="5" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <path d="M6 5.2v3.3M6 3.4v.3" stroke="currentColor" strokeWidth="1.5" fill="none" />
        </svg>
        {anchor}
      </button>
      <p className="tip-content" id={id} hidden={!open} data-role="teaching">
        {children}
      </p>
    </span>
  );
}
