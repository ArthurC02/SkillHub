import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

/**
 * ADR-039 / [docs/design/system.md](../../../docs/design/system.md) §4.1, §4.2, §5.
 *
 * The type and spacing scales exist so that a new value is a decision somebody
 * makes rather than one that accumulates. Checklist item 9 was the only rule in
 * that document with no machine behind it, which — by the same document's own
 * §6 — is the state a rule quietly rots in.
 *
 * The shape is deliberately `db/query-owners.yaml`'s: a scale, plus a named
 * list of what is already off it, and the standing rule that **the list may
 * only get shorter**. A value that is neither on the scale nor on the list
 * fails here, so adding one means either putting it on the scale or arguing for
 * it in `system.md` first.
 *
 * What this cannot see: values in `style={{…}}` or in a component's own file.
 * There are none today, and `index.css` being the single stylesheet is what
 * makes that true rather than lucky.
 */
const css = readFileSync(join(import.meta.dirname, "index.css"), "utf8");

/**
 * The scales are READ FROM THE DOCUMENT, not copied into this file.
 *
 * ADR-039 named the cost of writing a design system down: "文件會漂——`index.css`
 * 改了而文件沒改，就變成第二份不可信的來源", and admitted nothing would fail when
 * it did. Hand-copying the scale here would have made that worse: the numbers
 * would then live in three places (system.md §4.1/§4.2/§5, the ADR, and this
 * array) and a green test would prove nothing about whether the document is
 * current.
 *
 * So the split is: **the document holds the policy, `index.css` holds the fact,
 * and this file compares them.** Editing the scale means editing system.md —
 * which is the visible decision the ratchet is supposed to force. The parser
 * only accepts bold `**NNpx**` cells; if the table's formatting drifts, the
 * sentinel assertions below fail rather than the scale silently emptying.
 */
const doc = readFileSync(
  join(import.meta.dirname, "..", "..", "..", "docs", "design", "system.md"),
  "utf8",
);

/** The px values written in bold between one `### n.n` heading and the next. */
function boldPx(section: string): number[] {
  const from = doc.indexOf(`### ${section}`);
  expect(from, `system.md has no section ${section}`).toBeGreaterThan(-1);
  const next = doc.indexOf("### ", from + 4);
  const body = doc.slice(from, next === -1 ? doc.length : next);
  return [...new Set([...body.matchAll(/\*\*(\d+)px\*\*/g)].map(([, n]) => Number(n)))];
}

/** §4.1, every step including the ≤1024px halves — they are values in the file. */
const TYPE_SCALE = boldPx("4.1");

/** §4.2's 4px grid. 0 is not a step; it is the absence of one. */
const SPACE_SCALE = [0, ...boldPx("4.2")];

/** §5.2, verbatim. Every entry is a value the document has already argued about. */
const DEVIATIONS = boldPx("5.2");

test("ADR-039: the document's tables are still machine-readable", () => {
  // Without these, a formatting change would empty the scales and turn every
  // assertion below into a tautology that passes on any stylesheet at all.
  expect(TYPE_SCALE.length, "§4.1 parsed no type steps").toBeGreaterThan(6);
  expect(SPACE_SCALE.length, "§4.2 parsed no spacing steps").toBeGreaterThan(6);
  expect(TYPE_SCALE, "§4.1 lost its body step").toContain(18);
  expect(SPACE_SCALE, "§4.2 lost its base step").toContain(8);
  expect(DEVIATIONS, "§5.2 parsed no deviations").not.toEqual([]);
});

function allowed(kind: "type" | "space") {
  const scale = kind === "type" ? TYPE_SCALE : SPACE_SCALE;
  return new Set([...scale, ...DEVIATIONS]);
}

test("ADR-039 §4.1: every font-size is on the type scale or named in §5", () => {
  const sizes = [...css.matchAll(/font-size:\s*(\d+)px/g)].map(([, n]) => Number(n));
  expect(sizes.length, "no font-size found — the regex or the file moved").toBeGreaterThan(5);
  const ok = allowed("type");
  expect(
    [...new Set(sizes)].filter((s) => !ok.has(s)).sort((a, b) => a - b),
    "a font-size that is neither on the scale nor in the deviation list",
  ).toEqual([]);
});

test("ADR-039 §4.2: every padding/margin/gap length is on the 4px grid or named in §5", () => {
  const decls = [...css.matchAll(/(?:padding|margin|gap)(?:-\w+)?:\s*([^;]+);/g)];
  expect(decls.length, "no spacing declaration found").toBeGreaterThan(20);
  const values = decls.flatMap(([, v]) => [...v.matchAll(/(\d+)px/g)].map(([, n]) => Number(n)));
  const ok = allowed("space");
  expect(
    [...new Set(values)].filter((v) => !ok.has(v)).sort((a, b) => a - b),
    "a spacing value that is neither on the grid nor in the deviation list",
  ).toEqual([]);
});

/**
 * The ratchet, fastened to the array that can actually launder debt.
 *
 * The previous version capped only the deviation list — while §5's documented
 * way to retire a deviation is 「改回尺度上」, i.e. move it into the scale. That
 * made the escape route the one unguarded array: add a value to TYPE_SCALE and
 * everything stayed green. Now the scales come from the document, so growing
 * one is an edit to system.md; this caps the total anyway, so the edit has to
 * be argued for rather than slipped in.
 */
test("ADR-039 §5: the vocabulary may shrink, not grow", () => {
  expect(
    TYPE_SCALE.length,
    "a tenth type step — merge it into an existing one or argue for it in §4.1",
  ).toBeLessThanOrEqual(11);
  expect(
    SPACE_SCALE.length,
    "an eighth spacing step — 4px grid, or argue for it in §4.2",
  ).toBeLessThanOrEqual(8);
  expect(
    DEVIATIONS.length,
    "the deviation list may only shrink; a new value belongs on the scale, not here",
  ).toBeLessThanOrEqual(2);
});

/**
 * §2.7. The one principle with no row in §6 at all — neither credited nor
 * listed as unenforced. Its real state until now was "satisfied by every line
 * of the file, guarded by nothing", which is the state `contrast.test.ts`
 * describes in its own "What it does not prove" list: *nothing stops them
 * coming back*. Something does now.
 *
 * Both halves are ratchets on a regression this file has already suffered.
 * `opacity` was removed twice (QA-009) because a multiplier lands where no
 * colour token can follow it, and `contrast.test.ts` measures static hex — it
 * cannot see a multiplier, and says so. A literal outside `:root` is the same
 * defect by a different route: a colour no contrast test is looking at.
 */
test("ADR-039 §2.7: colour lives in tokens, and nothing multiplies it", () => {
  const body = css
    // Comments first — QA-009's reasoning quotes the hex values it deleted.
    .replace(/\/\*[\s\S]*?\*\//g, "")
    // Then the token declarations themselves; those are where colour belongs.
    .replace(/--[\w-]+:[^;]+;/g, "");
  expect(
    body.match(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/g) ?? [],
    "a colour literal outside the token blocks — contrast.test.ts cannot see it",
  ).toEqual([]);
  expect(
    body.match(/(^|[;{\s])opacity\s*:/g) ?? [],
    "opacity on anything: removed twice already, and no colour token can follow a multiplier",
  ).toEqual([]);
});

/**
 * The blind spot this file's own header used to declare — "`index.css` being
 * the single stylesheet is what makes that true rather than lucky" — asserted
 * instead of observed. A second stylesheet, and every check above keeps passing
 * while covering less of the app.
 */
test("ADR-039 §4: index.css is still the only stylesheet", () => {
  expect(
    readdirSync(import.meta.dirname).filter((f) => f.endsWith(".css")),
    "a second stylesheet — the scale guards above only read index.css",
  ).toEqual(["index.css"]);
});
