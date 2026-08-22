/**
 * One loading line, in one wording, in a live region.
 *
 * 04 丙-35 ruled that these are **not** §2.12: a 200ms fetch does not need
 * 「會不會自己結束」 or 「能不能離開」, and the one screen that does — a Run in
 * flight — has `InFlight`. What was left was two smaller things, and only one of
 * them is cosmetic:
 *
 *  1. **`role="status"` was missing almost everywhere.** A screen reader was
 *     told nothing at all while a page sat empty, and then the answer appeared
 *     silently. That is an accessibility defect, not a wording preference.
 *  2. Five of the lines said a bare 「載入中…」 with no subject, on pages where
 *     something else was already rendered — so it did not say *what* was still
 *     coming (§2.1).
 *
 * `what` names the thing, and it is required for exactly that reason: the
 * component cannot be used to reintroduce a subjectless line.
 *
 * No spinner and no skeleton, deliberately — `index.css` has no keyframes
 * anywhere and this is not the place to introduce the first one.
 */
export function Loading({ what, className }: { what: string; className?: string }) {
  return (
    // `data-loading` is the stable hook the tests wait on. They used to poll for
    // the literal 「載入中」, which quietly stopped matching the moment a line
    // named its subject — and a wait that always passes is a test that renders
    // its assertions against a half-drawn page.
    <p role="status" data-loading="" className={className}>
      載入{what}中…
    </p>
  );
}
