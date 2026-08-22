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

/** §4.1. Nine steps; the five responsive values are the same steps at ≤1024px. */
const TYPE_SCALE = [56, 40, 36, 30, 24, 20, 18, 16, 15, 14, 12];

/** §4.2. The 4px grid. */
const SPACE_SCALE = [0, 4, 8, 12, 16, 20, 24, 32];

/**
 * §5, verbatim. Every entry is a value the document has already argued about;
 * `keep` ones have a derivation, `collect` ones are owed a change. Nothing may
 * be added here to make a build pass — that is the whole point of the list.
 */
const KNOWN_DEVIATIONS: { value: number; kind: "type" | "space"; why: string }[] = [
  { value: 5, kind: "space", why: "control padding — 20px line-height + 5×2 + 2 borders = 32px" },
  { value: 10, kind: "space", why: "card padding 10px 12px, one value shared by four card families" },
];

function allowed(kind: "type" | "space") {
  const scale = kind === "type" ? TYPE_SCALE : SPACE_SCALE;
  return new Set([
    ...scale,
    ...KNOWN_DEVIATIONS.filter((d) => d.kind === kind).map((d) => d.value),
  ]);
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
 * The list is the debt, so it has to be visible as debt. This does not assert a
 * particular length — that would fail on the day someone collects one — but it
 * does fail if the list grows past what §5 currently documents.
 */
test("ADR-039 §5: the deviation list has not grown", () => {
  expect(
    KNOWN_DEVIATIONS.length,
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
 * defect by a different route: it is a colour no contrast test is looking at.
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
 * The blind spot this file's own header declares — "`index.css` being the
 * single stylesheet is what makes that true rather than lucky" — asserted
 * instead of observed. A second stylesheet, and every scale check above keeps
 * passing while covering less of the app.
 */
test("ADR-039 §4: index.css is still the only stylesheet", () => {
  expect(
    readdirSync(import.meta.dirname).filter((f) => f.endsWith(".css")),
    "a second stylesheet — the scale guards above only read index.css",
  ).toEqual(["index.css"]);
});
