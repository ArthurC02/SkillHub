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
 *
 * `recursive` because the first version of this was not: `readdirSync` on `src`
 * alone sees the top level, so `src/components/foo.css` passed it in silence
 * while covering none of it. §6 recorded that gap on 2026-08-25 as 「它是 `src/`
 * 頂層的不變式，不是全 app 的」. Now the assertion is as wide as the sentence.
 */
test("ADR-039 §4: index.css is still the only stylesheet", () => {
  expect(
    readdirSync(import.meta.dirname, { recursive: true })
      .map((f) => String(f).replaceAll("\\", "/"))
      .filter((f) => f.endsWith(".css"))
      .sort(),
    "a second stylesheet — the scale guards above only read index.css",
  ).toEqual(["index.css"]);
});

/**
 * §3 checklist item 16's fourth clause: 「markup 裡的 class 在 CSS 裡有規則」.
 *
 * That checklist counted it as one of four automated guards until 2026-08-25,
 * when the count was corrected to three because nothing in `apps/web` asserted
 * it. §6's row for it read 「沒有 — 一列都沒有」. This is that row.
 *
 * WHAT IT IS NOT. Not 「every class has a rule」 — that check is loud rather
 * than useful, because eight classes in this app deliberately have none and the
 * honest answers for them are two different answers:
 *
 *   - **a settled decision.** `badge-source-package` takes no `--accent-border`
 *     on purpose (`index.css` above `.badge-source-model`): 作者原文 is a
 *     settled fact, and tinting it as uncertain would make the colour
 *     contradict the word. Plain `.badge` IS its visual.
 *   - **a test hook.** A class is the only handle on a node, and a test selects
 *     it. Inventing a rule for one would be inventing design; deleting one
 *     would take a passing test with it.
 *
 * So the shape is `db/query-owners.yaml`'s again, and §5's: a named list of
 * what is already off the rule, each entry saying which of those two it is.
 * The value is not that the list is empty — it is that a NEW class now costs
 * either a rule in `index.css` or a line here with a reason, instead of
 * nothing. A class that is neither styled nor explained fails.
 *
 * WHAT IT CANNOT SEE. A class assembled from an interpolation —
 * `` `badge badge-${kind}-${value.value}` `` in `LabelledBadge` — is dropped
 * whole rather than guessed at, so the `badge-trust-*` / `badge-compat-*`
 * family is out of reach. Only the used → defined direction is checked; dead
 * rules in `index.css` are not, and cannot be while that family exists.
 */
const UNSTYLED: Record<string, string> = {
  // Settled decision — the one entry that is not a test hook.
  "badge-source-package":
    "settled decision: index.css says 作者原文 is a settled fact, so this badge " +
    "deliberately does not take --accent-border's 未知／未驗證 tint. Plain .badge is the visual.",

  // Test hooks. Each is selected by a test named beside it.
  "app-shell":
    "test hook: disc.test.tsx and generate.test.tsx select .app-shell to tell 已掛載 apart from 還沒",
  "feedback-entry":
    "test hook: a11y.test.tsx selects .feedback-entry form and .feedback-entry [role=alert] (BETA-004)",
  "risk-infos":
    "test hook: disc.test.tsx selects details.risk-infos to assert the info findings stay collapsed",
  "license-expression":
    "test hook: disc.test.tsx asserts an unknown license shows no expression (DISC-008); the class is the only handle",
  "badge-license-source":
    "test hook: the other half of the same DISC-008 assertion in disc.test.tsx",
  "badge-source-template":
    "test hook: disc.test.tsx counts this selector to assert template copy does not borrow the model marker",
};

/**
 * Every class token this app puts in the DOM, and the files that put it there.
 *
 * Read from `className=` positions only, not from every string in the file: a
 * guard that collected all literals would report `title` copy and comparison
 * operands as missing classes, which is the loud-not-useful failure mode again.
 */
function classesInMarkup(): Map<string, string[]> {
  const used = new Map<string, string[]>();
  for (const entry of readdirSync(import.meta.dirname, { recursive: true })) {
    const file = String(entry).replaceAll("\\", "/");
    if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
    const body = readFileSync(join(import.meta.dirname, file), "utf8");

    for (const at of body.matchAll(/className=/g)) {
      const start = at.index + at[0].length;
      let expr: string;
      if (body[start] === '"') {
        expr = body.slice(start, body.indexOf('"', start + 1) + 1);
      } else if (body[start] === "{") {
        // Naive depth counting is enough: a `${` inside a template literal
        // opens and closes its own brace, so it balances like any other.
        let depth = 0;
        let end = start;
        while (end < body.length) {
          if (body[end] === "{") depth++;
          else if (body[end] === "}" && --depth === 0) break;
          end++;
        }
        expr = body.slice(start, end + 1);
      } else {
        continue;
      }

      for (const lit of expr.matchAll(/"([^"]*)"|'([^']*)'|`([^`]*)`/g)) {
        // `value === "scanned" ? "badge" : …` — the operand of a comparison is
        // a value being tested, not a class being applied.
        if (/[=!]==?\s*$/.test(expr.slice(0, lit.index))) continue;
        // An interpolation is MARKED, not removed, so the token around it is
        // dropped whole rather than leaving a half-name like `badge-source-`
        // behind. Static tokens beside it are still checked.
        const raw = (lit[1] ?? lit[2] ?? lit[3]).replace(/\$\{[^}]*\}/g, " ");
        for (const token of raw.split(/\s+/)) {
          if (!token || token.includes(" ")) continue;
          if (!used.has(token)) used.set(token, []);
          if (!used.get(token)!.includes(file)) used.get(token)!.push(file);
        }
      }
    }
  }
  return used;
}

test("ADR-039 §3 第 16 條: every class in the markup has a rule, or a reason", () => {
  const used = classesInMarkup();
  // Sentinel, for the same reason §4.1's is: a parse that silently returns
  // nothing would pass this file on any markup at all.
  expect(used.size, "no class found in any .tsx — the className scan broke").toBeGreaterThan(40);

  const defined = new Set(
    [...css.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/\.([a-zA-Z][\w-]*)/g)].map(([, c]) => c),
  );

  const unexplained = [...used.keys()].filter((c) => !defined.has(c) && !(c in UNSTYLED)).sort();
  expect(
    unexplained.map((c) => `${c} (${used.get(c)!.join(", ")})`),
    "a class with no rule in index.css and no line in UNSTYLED — either give it a " +
      "visual, or say there which of the two reasons it has for not having one",
  ).toEqual([]);

  // Shrink-only, like §5's deviation list. An entry leaves by getting a rule or
  // by leaving the markup; a new one arriving is what this guard exists to stop.
  expect(
    Object.keys(UNSTYLED).length,
    "the unstyled list may only get shorter; a new class belongs in index.css",
  ).toBeLessThanOrEqual(7);

  // And it may not rot: an entry whose class is gone, or which has since been
  // given a rule, is a reason nobody needs to read any more.
  expect(
    Object.keys(UNSTYLED)
      .filter((c) => defined.has(c) || !used.has(c))
      .sort(),
    "an entry that is now styled or no longer in the markup — delete the line",
  ).toEqual([]);
});
