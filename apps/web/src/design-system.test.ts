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

/**
 * The markup scans below share one reader, and it is deliberately crude.
 *
 * `classesInMarkup` above already established the shape: read the non-test
 * `.tsx` under `src/`, look at what the JSX actually writes, and compare it
 * against a shrink-only list of what is allowed to be off the rule. None of
 * these parse TypeScript — a real parser here would be a dependency and a
 * second thing to keep working, and every one of these rules is about a string
 * that is either in the file or is not.
 */
function componentFiles(): Array<[string, string]> {
  const files: Array<[string, string]> = [];
  for (const entry of readdirSync(import.meta.dirname, { recursive: true })) {
    const file = String(entry).replaceAll("\\", "/");
    if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
    files.push([file, readFileSync(join(import.meta.dirname, file), "utf8")]);
  }
  return files;
}

/**
 * §2.4 第 3 項 and §2.11(c): **a qualification may be in a tooltip; it may not
 * be ONLY in a tooltip.**
 *
 * The only machine that ever looked at `title` was `a11y.test.tsx`'s
 * `[disabled][title]` rule, and not one of the app's fourteen `title=` sites
 * was on a disabled control — so the rule that mattered most had no gate at
 * all. The clearest instance was named in `system.md` by component: the search
 * row's tier badge carried 「收錄不等於精選。」, a sentence whose only job is to
 * stop the badge reading as an endorsement, in a tooltip that does not exist on
 * a touch device.
 *
 * THE RULE. For a `title=` on an element that is not `[disabled]`, the same
 * text must be reachable as visible text in the same file — either the literal
 * appears twice, or the expression is rendered as a child somewhere.
 *
 * WHAT IT CANNOT SEE. Whether the visible copy is in the same block, or on
 * screen at the same time. It compares strings in a file, so a component that
 * renders its `title` text inside a closed `<details>` passes. §2.10 is the
 * rule for that, and §6 records it as having no machine.
 */
const TOOLTIP_ONLY: Record<string, string> = {
  "pages/Home.tsx: title={reason}":
    "the disabled filter's own reason, rendered as visible .note text by the same map " +
    "(UNAVAILABLE_FILTERS) two lines below — the scan cannot follow one identifier to two uses",

  // Five provenance markers on a search row, explained once above the list
  // rather than five times on every card — 設計 §0: 順位低的規則讓步時，讓的是
  // 形式，內容一個字都不能少. The wording differs from these tooltips because it
  // covers all five in one sentence, which is why the scan cannot match it.
  'pages/Home.tsx: title="這段摘要由模型改寫，不是套件作者寫的；你的 Agent 讀的是套件自己的 description"':
    "explained in the 標記說明 line above the results list (pages/Home.tsx)",
  'pages/Home.tsx: title="套件自己的 frontmatter description"':
    "explained in the 標記說明 line above the results list",
  'pages/Home.tsx: title="伺服器沒有回報這段摘要的來源"':
    "explained in the 標記說明 line above the results list",
  'pages/Home.tsx: title="這段說明由模型產生，未經人工核對"':
    "explained in the 標記說明 line above the results list",
  'pages/Home.tsx: title="依查詢與文件的關鍵字重疊組出"':
    "explained in the 標記說明 line above the results list",

  'pages/SkillFiles.tsx: title="此檔案為可執行 Script"':
    "the file tree states it below the list in a longer sentence that also gives the count " +
    "and says the platform never runs them — the visible text 設計 §3 第 4 條 asks for",
};

test("ADR-039 §2.4/§2.11(c): a title is never the only place a qualification exists", () => {
  const offenders: string[] = [];
  let scanned = 0;

  for (const [file, body] of componentFiles()) {
    for (const at of body.matchAll(/title=(?:"([^"]*)"|\{([^}]*)\})/g)) {
      // A disabled control is the one case `a11y.test.tsx` already covers, with
      // a stricter rule than this one (it demands `aria-describedby`).
      const element = body.slice(Math.max(0, body.lastIndexOf("<", at.index)), at.index);
      if (element.includes("disabled")) continue;
      scanned++;

      const literal = at[1];
      const expression = at[2];
      const visible = literal
        ? // The same words somewhere else in the file, as a JSX child.
          body.split(literal).length > 2
        : // `title={x.note}` is visible when `{x.note}` is also rendered — the
          // brace form with no `title=` in front of it.
          new RegExp(
            `(?<!title=)\\{\\s*${expression.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\s*\\}`,
          ).test(body);
      const key = `${file}: ${at[0]}`;
      if (!visible && !(key in TOOLTIP_ONLY)) offenders.push(key);
    }
  }

  // Sentinel, like every other scan in this file: a parse that finds no `title`
  // would pass on markup that is nothing but tooltips.
  expect(scanned, "no title= found at all — the scan broke").toBeGreaterThan(8);
  expect(
    offenders.sort(),
    "a qualification that exists only in a tooltip. Render it as visible text " +
      "(the tooltip may stay), or add a line to TOOLTIP_ONLY saying where the " +
      "reader actually meets it",
  ).toEqual([]);

  // Shrink-only, and it may not rot.
  expect(Object.keys(TOOLTIP_ONLY).length, "the list may only get shorter").toBeLessThanOrEqual(7);
});

/**
 * §2.12 第 3 條 and 設計 §3 第 14 條: **the server's ISO 8601 UTC string is not
 * a time a reader can act on.**
 *
 * Twenty-nine places interpolated `created_at` and its siblings straight into a
 * Chinese sentence — 「建立於 2026-08-17T00:00:00Z」 — in somebody else's
 * timezone, with no `<time dateTime>` anywhere in the app and four different
 * spellings of the one fact. `components/Timestamp.tsx` is the one wording now.
 *
 * THE RULE. A `_at`-suffixed value may not be rendered directly. Two shapes are
 * flagged, and only two, because they are the ones that put the raw string on
 * screen: a bare `{x.y_at}` as a JSX child, and `${x.y_at}` inside a template
 * literal. A guard (`{x.y_at && …}`), a comparison and a sort key all keep more
 * syntax inside the braces and are not flagged — they are not renders.
 */
const RAW_TIMESTAMP: Record<string, string> = {
  "components/Timestamp.tsx: ${at}":
    "the component itself: `<time dateTime={at}>` is where the exact instant belongs",
  "pages/Compare.tsx: ${skill.version.created_at}":
    "a row's `signature`, which is the comparison key the 有差異 highlight is computed from. " +
    "Never rendered — the cell beside it uses <Timestamp>",
  "pages/Downloads.tsx: ${r.downloaded_at}":
    "a React `key`, not a child. The same row renders the instant with <Timestamp>",
};

test("ADR-039 §2.12: no page prints a raw server timestamp", () => {
  // `_at|_since` and not `_at`: `SkillSource.unavailable_since` is a
  // `format: date-time` field like every other, and it printed
  // 「來源已失效，自 2026-08-01T18:00:00Z 起無法取得」 two lines from a sibling
  // sentence that used <Timestamp> — while this ratchet, matching only `_at`,
  // watched it go past.
  const bare = /(?<![=$])\{\s*[A-Za-z0-9_.?[\]]*[A-Za-z0-9_]+_(at|since)\s*\}/g;
  const interpolated = /\$\{[A-Za-z0-9_.?[\]]*[A-Za-z0-9_]+_(at|since)\}/g;
  // The other way past this gate, and the one that was actually used: slice the
  // string. `{hit.verified_at.slice(0, 10)}` is not `{…_at}`, so the two regexes
  // above never saw it — and it shipped on EVERY search and catalogue row, the
  // app's highest-traffic timestamp, under-reporting a day for UTC+8 readers
  // and carrying no `<time dateTime>` for assistive technology. Cutting a
  // timestamp up by hand is the defect, whatever the cut looks like.
  const sliced = /[A-Za-z0-9_]+_(at|since)\s*\.\s*(slice|substring|substr|split)\s*\(/g;

  const offenders: string[] = [];
  for (const [file, body] of componentFiles()) {
    for (const m of [
      ...body.matchAll(bare),
      ...body.matchAll(interpolated),
      ...body.matchAll(sliced),
    ]) {
      const key = `${file}: ${m[0]}`;
      if (!(key in RAW_TIMESTAMP)) offenders.push(key);
    }
  }

  expect(
    offenders.sort(),
    "a server timestamp rendered as-is. Use <Timestamp at={…} /> (or formatAt for " +
      "an <option> label), so the reader gets their own clock and the DOM keeps the instant",
  ).toEqual([]);
  expect(Object.keys(RAW_TIMESTAMP).length, "the list may only get shorter").toBeLessThanOrEqual(3);
});

/**
 * 資訊架構 §5 IA-6 的棘輪，也就是那條裁定自己說它缺的東西。
 *
 * §6 records it verbatim: 「**新頁面仍然沒有棘輪**——沒有任何東西阻止下一個人在新
 * 頁面直接印 `error.message`；要那個得再加一條像 `design-system.test.ts` 第 16 條
 * 那樣掃 markup 的守衛」. This is that condition, met.
 *
 * THE RULE. A file that renders `role="alert"` with 失敗 in it must import
 * `ReadFailure` — because a read that failed has a 401 case, and a page that
 * writes its own sentence has almost always dropped it. The exceptions are the
 * failures that are not reads: a mutation the user just triggered, a sign-out,
 * a generation that ran and refused.
 */
const OWN_FAILURE_COPY: Record<string, string> = {
  "components/AuthControls.tsx":
    "a sign-out mutation, not a read — a 401 here means it already worked",
  "components/GenerateSkill.tsx":
    "POST /skills/generate: a mutation the user just pressed, and its refusals are the " +
    "contract's own GenerationFailure values (generateFailureSentence.ts), not read failures",
  "pages/ImportSkill.tsx":
    "POST /skills/import/*: a mutation, and the rejection body is the acceptance criterion " +
    "(CategorizedFindings). LoginRequired covers this page's signed-out arrival before the form",
  "components/SignIn.tsx":
    "POST /auth/dev/login: a sign-IN mutation, the mirror of the sign-out entry above. A 401 " +
    "here cannot mean 「需要登入」 — that is what the user just tried to do — so ReadFailure " +
    "would render the one sentence that is certainly wrong",
};

test("IA-6: a page that writes its own read-failure sentence has to be listed", () => {
  const offenders: string[] = [];
  let scanned = 0;

  for (const [file, body] of componentFiles()) {
    for (const at of body.matchAll(/role="alert"/g)) {
      // The rendered text of that one alert, up to whatever closes it.
      const rest = body.slice(at.index, at.index + 400);
      const text = rest.slice(0, Math.max(rest.indexOf("</p>"), rest.indexOf("</h")));
      if (!text.includes("失敗")) continue;
      scanned++;
      // **附近**，不是整個檔案。這一行以前是 `body.includes("ReadFailure")`：
      // 一個檔案只要在**任何地方**提過正確做法，它裡面每一句自己寫的失敗文案就
      // 全部放行。兩處實際的缺陷就是這樣溜過去的——`SkillDetail.tsx` 第 3 行
      // import 了 ReadFailure，所以它的 Fork 失敗那一行（把封測 403、名稱衝突
      // 409、session 過期全講成「請稍後再試」）從來沒有被這支測試看過；
      // `Packaging.tsx` 同理，它用一句「找不到這個 Skill，或載入失敗」回答包含
      // 410 在內的每一種狀態，而那正是 IA-6 判掉的那一句。
      // `ReadFailure` 包住一則 alert 時，開標籤就在它前面幾行。
      // 前後都看：共用元件可以包住這則 alert（開標籤在前），也可以緊跟在它後面
      // ——`Compare.tsx` 就是後者，那一句是**計數**（「3 個裡有 2 個讀取失敗」，
      // 沒有任何單一錯誤元件說得出來），底下才接 `ReadFailure` 交出第一個失敗的
      // 真正答案。相鄰就是相鄰，不分方向。
      if (body.slice(Math.max(0, at.index - 300), at.index + 700).includes("ReadFailure")) continue;
      if (file in OWN_FAILURE_COPY) continue;
      offenders.push(file);
    }
  }

  expect(scanned, "no failure alert found at all — the scan broke").toBeGreaterThan(4);
  expect(
    [...new Set(offenders)].sort(),
    "a read failure worded by hand. Use <ReadFailure error={…} what={…} /> so a 401 says " +
      "「需要登入」 and every other status keeps the server's own message; if this failure " +
      "is a mutation rather than a read, add a line to OWN_FAILURE_COPY saying which",
  ).toEqual([]);
  // 3 -> 4 on 2026-09-02, and the reason has to survive here or the ratchet has
  // been loosened rather than moved: the fourth entry is not a read-failure that
  // leaked past ReadFailure, it is a NEW mutation surface that did not exist —
  // offline sign-in, added because the app's only sign-in affordance was a
  // GitHub link that goes nowhere on the machine 02:PORT-005 is about. It sits
  // in the same category as the sign-out entry it mirrors. The bound may still
  // only move when a genuinely new mutation appears; a read-failure worded by
  // hand is still an offender, which is what the assertion above enforces.
  expect(Object.keys(OWN_FAILURE_COPY).length, "the list may only get shorter").toBeLessThanOrEqual(
    4,
  );
});
