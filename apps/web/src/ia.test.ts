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
 * FOUR THINGS IT COMPARES, and they are the four ways the IA changes:
 *
 *   1. a route is added or removed          → §1's table
 *   2. the primary nav changes              → §2.1's table
 *   3. a page becomes unreachable in-app    → §2.3's orphan row
 *   4. an entry point appears behind a flag → §2.4's table
 *
 * The fourth is the one the generation entry needed, and it is the one no
 * route-based check could have caught: a flagged entry point adds no route and
 * may add no link. It is found by the flag, not by the address.
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
 * In-page links only. The nav is excluded on purpose: a page reachable *solely*
 * from the nav is exactly the finding §2.3 exists to surface, so counting the
 * nav as an edge would erase it.
 */
function inPageLinkTargets(): Set<string> {
  const targets = new Set<string>();
  for (const dir of ["pages", "components"]) {
    for (const file of readdirSync(join(src, dir))) {
      if (!file.endsWith(".tsx") || file.includes(".test.")) continue;
      const body = readFileSync(join(src, dir, file), "utf8");
      for (const m of body.matchAll(/to="(\/[^"]*)"/g)) targets.add(m[1]);
    }
  }
  return targets;
}

test("IA §2.3: the pages reachable only from the nav are the ones the document names", () => {
  const routes = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => m[1]);
  const linked = inPageLinkTargets();
  const orphans = routes.filter((r) => !linked.has(r)).sort();

  const row = section("2.")
    .split("\n")
    .find((line) => line.includes("只有導覽列"));
  expect(row, "§2.3 has no 0-inbound row").toBeTruthy();

  // Both directions. A new orphan fails, and so does fixing one without saying
  // so — the second half is what keeps the row from becoming a stale list of
  // problems somebody already solved.
  expect(orphans).toEqual(paths(row!));
});

// --- 4. entry points behind a feature flag -----------------------------------

test("IA §2.4: every feature-flagged entry point is documented", () => {
  const flags = new Set<string>();
  for (const dir of ["api", "pages", "components"]) {
    for (const file of readdirSync(join(src, dir))) {
      if (!/\.tsx?$/.test(file) || file.includes(".test.")) continue;
      const body = readFileSync(join(src, dir, file), "utf8");
      for (const m of body.matchAll(/features\?\.(\w+)/g)) flags.add(m[1]);
    }
  }

  const documented = section("2.");
  for (const flag of flags) {
    expect(
      documented.includes(`\`${flag}\``),
      `${flag} gates an entry point and is not in §2.4. A flagged entry adds no route ` +
        `and may add no link, so this is the only place the IA can find out about it.`,
    ).toBe(true);
  }
});
