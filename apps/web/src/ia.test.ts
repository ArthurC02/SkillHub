import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

/**
 * [docs/design/information-architecture.md](../../../docs/design/information-architecture.md).
 *
 * Same split as `design-system.test.ts`, and written for the same reason:
 * **the document holds the policy, the code holds the fact, and this file
 * compares them.** An IA document nobody compares to the router is a second,
 * less current description of the app — which is worse than none, because
 * people read it.
 *
 * That is not hypothetical here. The document was written at 21:02 and was
 * stale by 21:24: another batch mounted the M5 generation entry point in two
 * pages, and §7 went on saying it did not exist. Nothing failed. This file is
 * what should have failed.
 *
 * FIVE THINGS IT COMPARES. The first four are the ways the IA drifts; the
 * fifth is the one that makes this document lead rather than follow.
 *
 *   1. a route is added or removed          → §1's table
 *   2. the primary nav changes              → §2.1's table
 *   3. a page loses its ways in             → §2.3's 0- and 1-inbound rows
 *   4. an entry point appears behind a flag → §2.4's table
 *   5. a new route breaks §0.1 R2           → §0.2's ledger, which may only shrink
 *   6. a product capability enters the nav  → §0.1 R7
 *
 * The fourth is the one the generation entry needed, and it is the one no
 * route-based check could have caught: a flagged entry point adds no route and
 * may add no link. It is found by the flag, not by the address.
 *
 * The fifth is different in kind. One to four ask 「does the document still
 * describe the app?」 — the app wins. Five asks 「does the app still obey the
 * document?」 — the document wins, and the only way past it is an edit somebody
 * has to read.
 *
 * WHAT THIS CANNOT SEE. Whether the prose is true — that a page is where the
 * document says it belongs in the value stream, that the h1 uses ADR-038's
 * controlled vocabulary, that a single-entry page is deliberate. Those stay
 * human judgement, and §6 of the document says so in the rows that read 「沒有」.
 */
const src = import.meta.dirname;
const doc = readFileSync(
  join(src, "..", "..", "..", "docs", "design", "information-architecture.md"),
  "utf8",
);
const router = readFileSync(join(src, "router.tsx"), "utf8");

/** The text between one `## N.` heading and the next, so tables in different sections cannot vote in each other's assertions. */
function section(heading: string): string {
  const start = doc.indexOf(`\n## ${heading}`);
  expect(start, `information-architecture.md has no 「## ${heading}」 section`).toBeGreaterThan(-1);
  const rest = doc.slice(start + 1);
  const end = rest.indexOf("\n## ");
  return end === -1 ? rest : rest.slice(0, end);
}

/**
 * One address, collapsed so a router pattern and a real URL compare equal:
 * query string dropped, `$skillId` and `${SKILL}` both becoming `*`.
 */
function shapePath(address: string): string {
  return address.split("?")[0].replace(/\$\{[^}]*\}|\$\w+/g, "*");
}

/** Every `` `/…` `` in a stretch of markdown, deduplicated and sorted. */
function paths(markdown: string): string[] {
  return [...new Set([...markdown.matchAll(/`(\/[^`]*)`/g)].map((m) => m[1]))].sort();
}

// --- 1. the route list -------------------------------------------------------

test("IA §1: every route in router.tsx has a row, and every row is a route", () => {
  const actual = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => m[1]).sort();
  // Sentinel: a formatting change that breaks the parse must fail here rather
  // than quietly comparing two empty sets and passing. Deliberately a floor and
  // not an exact count — §1's heading used to carry 「17 條路由」 and a number in
  // a heading is one more copy of a fact to keep in step. The table is the count.
  expect(actual.length).toBeGreaterThanOrEqual(17);

  const documented = paths(
    section("1.")
      .split("\n")
      .filter((line) => line.startsWith("| `/"))
      .join("\n"),
  );

  expect(documented).toEqual(actual);
});

// --- 2. the primary nav ------------------------------------------------------

test("IA §2.1: the primary nav's targets are the ones the document lists", () => {
  const nav = router.slice(router.indexOf('<nav className="app-nav"'), router.indexOf("</nav>"));
  const actual = [...new Set([...nav.matchAll(/to="([^"]+)"/g)].map((m) => m[1]))].sort();
  expect(actual.length).toBeGreaterThanOrEqual(4);

  const row = section("2.")
    .split("\n")
    .find((line) => line.includes("主要導覽"));
  expect(row, "§2.1 has no 主要導覽 row").toBeTruthy();
  // The row names the nav's five labels in one cell and their targets in the
  // next; only the targets are backticked paths.
  expect(paths(row!)).toEqual(actual);

  // §0.1 R7, from the 2026-08-23 ruling 「探索不進導覽列」 and ADR-046 決策 7
  // before it: the nav holds the owner's own material, not the product's
  // capabilities. Mechanically that is R2's list-address test — every nav
  // target lives under /workspace/ or /lab/.
  //
  // Discovery and generation are not places you go, they are what the product
  // does; a nav of six makes them one option out of six. `/` keeps its entry —
  // the product title — and that page is only search, so clicking the product's
  // name lands on the product's core action.
  for (const target of actual) {
    expect(
      target.startsWith("/workspace/") || target.startsWith("/lab/"),
      `${target} is in the primary nav but is not one of the owner's own lists. ` +
        `A product capability in the nav reads as one option among the others ` +
        `(§0.1 R7). If this is deliberate, R7 is what has to change first.`,
    ).toBe(true);
  }
});

// --- 3. reachability ---------------------------------------------------------

/**
 * In-page links only, counted by distinct source file. The nav is excluded on
 * purpose: a page reachable *solely* from the nav is exactly the finding §2.3
 * exists to surface, so counting the nav as an edge would erase it.
 */
function inboundByRoute(): Map<string, number> {
  const sources = new Map<string, Set<string>>();
  for (const dir of ["pages", "components"]) {
    for (const file of readdirSync(join(src, dir))) {
      if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
      const body = readFileSync(join(src, dir, file), "utf8");
      for (const m of new Set([...body.matchAll(/to="(\/[^"]*)"/g)].map((x) => x[1]))) {
        if (!sources.has(m)) sources.set(m, new Set());
        sources.get(m)!.add(file);
      }
    }
  }
  const counts = new Map<string, number>();
  for (const [route, files] of sources) counts.set(route, files.size);
  return counts;
}

/** The paths in the §2.3 row whose first cell is exactly `n`. */
function reachabilityRow(n: string): string[] {
  const row = section("2.")
    .split("\n")
    .find((line) => line.startsWith(`| **${n}** |`));
  expect(row, `§2.3 has no 「${n}」 row`).toBeTruthy();
  return paths(row!);
}

test("IA §0.1 R3 / §2.3: the pages with 0 and with 1 way in are the ones the document names", () => {
  const routes = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => m[1]);
  const inbound = inboundByRoute();
  const withCount = (n: number) => routes.filter((r) => (inbound.get(r) ?? 0) === n).sort();

  // Both directions, both rows. A new orphan fails; so does fixing one without
  // saying so — the second half is what keeps the table from becoming a stale
  // list of problems somebody already solved.
  expect(withCount(0)).toEqual(reachabilityRow("0"));
  expect(withCount(1)).toEqual(reachabilityRow("1"));
});

// --- 5. R2, the rule that decides where the next screen goes -----------------

/**
 * §0.1 R2's mechanically checkable half.
 *
 * R2 has two clauses a machine can settle:
 *
 *   a. a single-item address (one carrying a `$param`) must not live under
 *      `/workspace/` or `/lab/` — the same id is the same thing to its owner
 *      and to a visitor, and what differs is which fields come back, not the
 *      address (WS-006, iron rule 3);
 *   b. a list address (no `$param`, more than one segment) must live under one
 *      of them — a list belongs to whoever is asking for it.
 *
 * The rest of R2 — whether the root really holds only ownerless things, whether
 * a verb belongs in a list position — is judgement, and §0.2's ledger carries
 * those rows without this test being able to confirm them.
 *
 * So the assertion is a SUBSET: every mechanical violation must already be in
 * the ledger. Combined with the cap below, that is the ratchet — a new
 * violation cannot land without an edit to a table somebody has to read.
 */
test("IA §0.1 R2: every route the rule can judge either follows it or is in the ledger", () => {
  const routes = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => m[1]);
  const owned = (p: string) => p.startsWith("/workspace/") || p.startsWith("/lab/");

  const violations = routes
    .filter((p) => {
      const segments = p.split("/").filter(Boolean);
      const isItem = segments.some((seg) => seg.startsWith("$"));
      if (isItem) return owned(p); // (a)
      return segments.length > 1 && !owned(p); // (b)
    })
    .sort();

  const ledger = section("0.")
    .split("\n")
    .filter((line) => line.startsWith("| `/"))
    .map((line) => paths(line)[0]);
  expect(ledger.length, "§0.2's ledger failed to parse").toBeGreaterThan(0);

  for (const bad of violations) {
    expect(
      ledger.includes(bad),
      `${bad} breaks R2 and is not in §0.2's ledger. Either move it, or add a row ` +
        `saying which of R2's premises does not hold for it — 「這樣比較方便」 is not one.`,
    ).toBe(true);
  }

  // The ledger may only get shorter (system.md §5's shape, and query-owners.yaml's
  // before it). Lowering this number is the only edit this line should ever see.
  expect(ledger.length).toBeLessThanOrEqual(3);
});

// --- 6. entry points behind a feature flag -----------------------------------

test("IA §2.4: every feature-flagged entry point is documented", () => {
  const flags = new Set<string>();
  for (const dir of ["api", "pages", "components"]) {
    for (const file of readdirSync(join(src, dir))) {
      if (!/\.tsx?$/.test(file) || file.includes(".test.")) continue;
      const body = readFileSync(join(src, dir, file), "utf8");
      for (const m of body.matchAll(/features[?][.](\w+)/g)) flags.add(m[1]);
    }
  }
  expect(flags.size, "no feature flags found at all — the parse broke").toBeGreaterThan(0);

  const documented = section("2.");
  for (const flag of flags) {
    expect(
      documented.includes("`" + flag + "`"),
      `${flag} gates an entry point and is not in §2.4. A flagged entry adds no route ` +
        `and may add no link, so this is the only place the IA can find out about it.`,
    ).toBe(true);
  }
});

/**
 * §2.4's other half: the flag is documented, and SOMEBODY IS TESTING IT OFF.
 *
 * The test above proves a flag is named in the IA document. It does not prove
 * a single line of code still honours it. `WorkspaceSkills.tsx` mounted the M5
 * generation entry point behind `generateExposed` for days with no assertion
 * anywhere: `generate.test.tsx` covered the search's `no_results` mount and
 * `workspace.test.tsx` had no `generate` in it at all, so changing that mount
 * to `{true && <GenerateSkill />}` left all 224 tests green.
 *
 * That is the ⛔ boundary AGENTS.md names — 開工不等於曝光 — and its failure has
 * no symptom. The page looks correct. What breaks is the meaning of 01 §11.2's
 * first funnel segment, which has one chance and twelve people.
 *
 * SHAPE, same as `db/query-owners.yaml` and system.md §5: a named roster that
 * may only get shorter. A render site absent from it fails; an entry whose file
 * is no longer a render site fails too, so the list cannot rot into a record of
 * mounts somebody already deleted.
 *
 * WHAT IT CANNOT SEE: whether the named test really asserts the off state. That
 * sentence is a human's to read — the roster's value is that a NEW mount now
 * costs a line here and a test somebody has to write, instead of nothing.
 */
const FLAG_OFF_ASSERTED: Record<string, string> = {
  "pages/Home.tsx":
    "generate.test.tsx — 「the generate entry point is absent until /me says the flag is on」",
  "pages/WorkspaceSkills.tsx":
    "workspace.test.tsx — 「⛔ with the flag off, /workspace/skills has no generation entry point」",
  "components/CleanModeNotice.tsx":
    "clean-mode.test.tsx — 「without the flag, the notice renders nothing」",
};

/** The hooks in `api/` that answer a `GET /me` feature flag, by name. */
function flagHooks(): Set<string> {
  const hooks = new Set<string>();
  for (const file of readdirSync(join(src, "api"))) {
    if (!/\.tsx?$/.test(file) || file.includes(".test.")) continue;
    const body = readFileSync(join(src, "api", file), "utf8");
    // Split at each `export function`, so the flag read is attributed to the
    // one it is inside. A brace-counting regex would not do: `useCleanMode`
    // has an early `return` in a block before its `features?.` line.
    const chunks = body.split(/export function (\w+)/);
    for (let i = 1; i < chunks.length; i += 2) {
      if (/features[?][.]\w+/.test(chunks[i + 1] ?? "")) hooks.add(chunks[i]);
    }
  }
  return hooks;
}

test("IA §2.4 / ADR-052: every flagged mount is on the roster of ones tested with the flag off", () => {
  const hooks = flagHooks();
  expect(
    hooks.size,
    "no flag hook found in api/ — the parse broke, and this file would then pass on any mount at all",
  ).toBeGreaterThan(0);

  const sites: string[] = [];
  for (const dir of ["pages", "components"]) {
    for (const file of readdirSync(join(src, dir))) {
      if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
      const body = readFileSync(join(src, dir, file), "utf8");
      // A call, not a mention: the two components the flag *gates* name the
      // hook in their headers without reading it, and they are not the mount.
      if ([...hooks].some((h) => body.includes(`${h}()`))) sites.push(`${dir}/${file}`);
    }
  }
  expect(sites.length, "no flagged mount found at all — the scan broke").toBeGreaterThan(0);

  expect(
    sites.filter((f) => !(f in FLAG_OFF_ASSERTED)).sort(),
    "a mount behind a /me feature flag with no line on the roster. Write the flag-off " +
      "assertion first, then name the test here — a mount nobody tests off is the ⛔ " +
      "boundary failing with no symptom (ADR-052).",
  ).toEqual([]);

  // It may not rot: an entry whose file no longer mounts anything behind a flag
  // is a promise about a test nobody can find any more.
  expect(
    Object.keys(FLAG_OFF_ASSERTED)
      .filter((f) => !sites.includes(f))
      .sort(),
    "a roster entry that is no longer a flagged mount — delete the line",
  ).toEqual([]);

  // Shrink-only, like §0.2's ledger above.
  expect(
    Object.keys(FLAG_OFF_ASSERTED).length,
    "the roster may only get shorter",
  ).toBeLessThanOrEqual(3);
});

// --- 7. the 375px sweep, and whether it still covers what it claims ----------

/**
 * §6's 「375px 不橫向溢出」 row, which both this document and system.md used to
 * credit with 「全部 18 個位址」.
 *
 * `e2e/routes.ts` is a hand-written table, and until now nothing compared it to
 * anything: a new route simply was not swept, and the row in both documents
 * went on claiming it was. On 2026-08-25 both rows were corrected downwards to
 * 「表上的 18 個位址」 — which is honest, and still leaves the table unmaintained.
 *
 * The pattern is already in this repo. `a11y.test.tsx` compares its scan list
 * against `router.routesById` and fails when a route has no case; this is the
 * same ratchet on the same kind of list, fastened here rather than in `e2e/`
 * because Playwright's tier does not run on every change and a coverage ratchet
 * that only fires in the slow suite is a ratchet nobody feels.
 *
 * Compared by SHAPE, not by address: the sweep visits real URLs carrying
 * fixture ids and query strings, the router declares patterns with `$params`.
 * Both sides collapse to `/skills/*`, so an address can carry whatever ids it
 * needs while still being answerable for which route it covers. More than one
 * address per route is fine and deliberate — `/` is swept twice, empty and with
 * results.
 */
test("IA §6: every route in router.tsx is swept at 375px", () => {
  const routes = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => shapePath(m[1]));

  const table = readFileSync(join(src, "..", "e2e", "routes.ts"), "utf8");
  const swept = [...table.matchAll(/^\s*\["[^"]*",\s*(?:"([^"]*)"|`([^`]*)`)\],/gm)].map((m) =>
    shapePath(m[1] ?? m[2]),
  );
  // Sentinel, tied to the other side rather than to a number: a formatting
  // change that breaks the parse must fail here, and 「18」 in an assertion is
  // one more copy of a fact to keep in step (§1's heading learned this already).
  expect(
    swept.length,
    "e2e/routes.ts parsed fewer rows than there are routes — the parse broke, or the table did",
  ).toBeGreaterThanOrEqual(routes.length);

  expect(
    [...new Set(routes)].sort(),
    "a route with no address in e2e/routes.ts — add one, with fixture ids the " +
      "shared stubs answer to. An unswept route is one nobody has ever seen at phone width",
  ).toEqual([...new Set(swept)].sort());
});

// --- 8. §4, the state the URL carries ---------------------------------------

/**
 * §4's table against `router.tsx`'s `validateSearch`, both ways.
 *
 * §4 is a 盤點 — the code wins — and it had gone stale in the way this whole
 * file exists to catch: the `/runs/$id` row read **無**, while the route had
 * carried `evaluation` and `events` for weeks, each with a paragraph in
 * `router.tsx` explaining why it belongs in the URL. Nothing failed. The `/`
 * row was worse than stale, it was unfinishable: 「`q`、`script`、`validation`…」,
 * and an ellipsis cannot be compared to anything.
 *
 * This is the same ratchet as §1's and §2.1's, on the one table in this
 * document that describes something a reader can act on — a URL they are about
 * to paste to somebody else.
 *
 * WHAT IT CANNOT SEE. Whether a param SHOULD be in the URL. That is R4, and
 * §6 records R4 as having no machine: 「你在看哪一份東西」 versus 「你偏好怎麼看」
 * is a judgement, and IA-4 is the argument it produced.
 */
test("IA §4: every route's search params are the ones the document lists", () => {
  // Each `createRoute({…})` block, paired with its path and its validateSearch
  // keys. Split rather than brace-matched: the blocks are top level and the
  // path line is the second or third of each.
  const actual = new Map<string, string[]>();
  for (const block of router.split("createRoute({").slice(1)) {
    const path = /^\s*(?:getParentRoute:[^\n]*\n)?\s*path: "([^"]+)"/m.exec(block);
    if (!path) continue;
    const at = block.indexOf("validateSearch:");
    if (at === -1) continue;

    // The returned object literal, brace-matched from the `({` of `=> ({`.
    const open = block.indexOf("({", at) + 1;
    let depth = 0;
    let end = open;
    while (end < block.length) {
      if (block[end] === "{") depth++;
      else if (block[end] === "}" && --depth === 0) break;
      end++;
    }
    const body = block.slice(open + 1, end);

    // Keys at depth 0 of that literal. A value may itself be a ternary spanning
    // lines, so depth is counted rather than assuming one key per line.
    const keys: string[] = [];
    let nest = 0;
    for (const line of body.split("\n")) {
      const key = /^\s*(\w+):/.exec(line);
      if (nest === 0 && key) keys.push(key[1]);
      for (const ch of line) {
        if (ch === "{" || ch === "[" || ch === "(") nest++;
        else if (ch === "}" || ch === "]" || ch === ")") nest--;
      }
    }
    actual.set(shapePath(path[1]), keys.sort());
  }

  // Sentinel: a parse that finds no route with params would compare two empty
  // maps and pass on any drift at all.
  expect(actual.size, "no validateSearch parsed — the scan broke").toBeGreaterThanOrEqual(7);

  const documented = new Map<string, string[]>();
  for (const line of section("4.").split("\n")) {
    if (!line.startsWith("| `/")) continue;
    const cells = line.split("|").map((c) => c.trim());
    const address = paths(cells[1])[0];
    // Only the SECOND cell: the reason cell names params in prose too, and a
    // row-wide scan would read those as if they were declared.
    const params = [...cells[2].matchAll(/`(\w+)`/g)].map((m) => m[1]).sort();
    documented.set(shapePath(address), params);
  }

  expect(
    [...documented.keys()].sort(),
    "§4 lists a route that has no validateSearch, or omits one that has — the table " +
      "is an inventory of what `router.tsx` actually accepts",
  ).toEqual([...actual.keys()].sort());

  for (const [address, params] of actual) {
    expect(
      documented.get(address),
      `${address}: §4 and router.tsx disagree about which search params this address carries`,
    ).toEqual(params);
  }
});
