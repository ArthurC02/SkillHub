/**
 * 設計 §4.7 / ADR-065 決策 5: the app's icon vocabulary, all of it.
 *
 * Four shapes, one per row of the §4.4 state table — 不通過／未知或未檢查／
 * 降級／通過 — plus the Tip trigger's own in `Tip.tsx`. That is five of the six
 * the rule allows, and `design-system.test.ts` counts `<svg` sites so a sixth
 * has to argue for itself here rather than appear in a page.
 *
 * An icon is the THIRD signal. The word is first (§2.3), the border colour
 * second, and this only lets a row of badges be told apart while scanning —
 * it carries nothing a word does not already carry, which is why it is
 * `aria-hidden` and why `a11y.test.tsx` fails any page where an icon has no
 * visible word beside it. Colour is `currentColor`, i.e. whatever token the
 * badge already wears (`--danger` on a failing badge, `--text-h` otherwise);
 * no new semantic colour (§4.6.5).
 *
 * Not a set (ADR-064 決策 7 still stands): no asset directory, no font, no
 * package — four paths in one file, each with a reason to exist.
 */
export type IconState = "fail" | "unknown" | "degraded" | "pass";

const PATH: Record<IconState, string> = {
  // 不通過: a cross. The one shape a reader already knows means 「no」.
  fail: "M3 3l6 6M9 3l-6 6",
  // 未知或未檢查: a question mark, for 「nobody looked」 — not a warning triangle,
  // which would say 「something is wrong」 about a thing that was never checked.
  unknown: "M4.2 4.5a1.8 1.8 0 1 1 2.6 1.6c-.6.3-.8.6-.8 1.2M6 9v.3",
  // 降級: a dash. The check ran and could not finish — neither the cross nor
  // the tick is honest, and §4.4 gives this row its own (dashed) border.
  degraded: "M3.5 6h5",
  // 通過: a tick.
  pass: "M3 6.2l2 2 4-4.4",
};

export function StateIcon({ state }: { state: IconState }) {
  return (
    <svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12" focusable="false">
      <path
        d={PATH[state]}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
    </svg>
  );
}
