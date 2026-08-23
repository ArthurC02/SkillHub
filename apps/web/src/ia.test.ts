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

/** Every `` `/…` `` in a stretch of markdown, deduplicated and sorted. */
function paths(markdown: string): string[] {
  return [...new Set([...markdown.matchAll(/`(\/[^`]*)`/g)].map((m) => m[1]))].sort();
}

// --- 1. the route list -------------------------------------------------------

test("IA §1: every route in router.tsx has a row, and every row is a route", () => {
  const actual = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => m[1]).sort();
  // Sentinel: a formatting change that breaks the parse must fail here rather
  // than quietly comparing two empty sets and passing.
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
