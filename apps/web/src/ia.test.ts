import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const src = import.meta.dirname;
const doc = readFileSync(
  join(src, "..", "..", "..", "docs", "design", "information-architecture.md"),
  "utf8",
);
const router = readFileSync(join(src, "router.tsx"), "utf8");

function section(heading: string): string {
  const start = doc.indexOf(`\n## ${heading}`);
  expect(start, `information-architecture.md has no 「## ${heading}」 section`).toBeGreaterThan(-1);
  const rest = doc.slice(start + 1);
  const end = rest.indexOf("\n## ");
  return end === -1 ? rest : rest.slice(0, end);
}

function shapePath(address: string): string {
  return address.split("?")[0].replace(/\$\{[^}]*\}|\$\w+/g, "*");
}

function paths(markdown: string): string[] {
  return [...new Set([...markdown.matchAll(/`(\/[^`]*)`/g)].map((m) => m[1]))].sort();
}

test("IA §1: every route in router.tsx has a row, and every row is a route", () => {
  const actual = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => m[1]).sort();
  expect(actual.length).toBeGreaterThanOrEqual(17);

  const documented = paths(
    section("1.")
      .split("\n")
      .filter((line) => line.startsWith("| `/"))
      .join("\n"),
  );

  expect(documented).toEqual(actual);
});

test("IA §2.1: the primary nav's targets are the ones the document lists", () => {
  const nav = router.slice(router.indexOf('<nav className="app-nav"'), router.indexOf("</nav>"));
  const actual = [...new Set([...nav.matchAll(/to="([^"]+)"/g)].map((m) => m[1]))].sort();
  expect(actual.length).toBeGreaterThanOrEqual(4);

  const row = section("2.")
    .split("\n")
    .find((line) => line.includes("主要導覽"));
  expect(row, "§2.1 has no 主要導覽 row").toBeTruthy();
  expect(paths(row!)).toEqual(actual);

  for (const target of actual) {
    expect(
      target.startsWith("/workspace/") || target.startsWith("/lab/"),
      `${target} is in the primary nav but is not one of the owner's own lists. ` +
        `A product capability in the nav reads as one option among the others ` +
        `(§0.1 R7). If this is deliberate, R7 is what has to change first.`,
    ).toBe(true);
  }
});

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

  expect(withCount(0)).toEqual(reachabilityRow("0"));
  expect(withCount(1)).toEqual(reachabilityRow("1"));
});

test("IA §0.1 R2: every route the rule can judge either follows it or is in the ledger", () => {
  const routes = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => m[1]);
  const owned = (p: string) => p.startsWith("/workspace/") || p.startsWith("/lab/");

  const violations = routes
    .filter((p) => {
      const segments = p.split("/").filter(Boolean);
      const isItem = segments.some((seg) => seg.startsWith("$"));
      if (isItem) return owned(p);
      return segments.length > 1 && !owned(p);
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

  expect(ledger.length).toBeLessThanOrEqual(3);
});

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

const FLAG_OFF_ASSERTED: Record<string, string> = {
  "pages/Home.tsx":
    "generate.test.tsx — 「the generate entry point is absent until /me says the flag is on」",
  "pages/WorkspaceSkills.tsx":
    "workspace.test.tsx — 「⛔ with the flag off, /workspace/skills has no generation entry point」",
  "components/CleanModeNotice.tsx":
    "clean-mode.test.tsx — 「without the flag, the notice renders nothing」",
  "pages/CreateSkill.tsx":
    "create-skill.test.tsx — 「⛔ with the flag off, /workspace/creations is not a workbench and says so」",
};

function flagHooks(): Set<string> {
  const hooks = new Set<string>();
  for (const file of readdirSync(join(src, "api"))) {
    if (!/\.tsx?$/.test(file) || file.includes(".test.")) continue;
    const body = readFileSync(join(src, "api", file), "utf8");
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

  expect(
    Object.keys(FLAG_OFF_ASSERTED)
      .filter((f) => !sites.includes(f))
      .sort(),
    "a roster entry that is no longer a flagged mount — delete the line",
  ).toEqual([]);

  expect(
    Object.keys(FLAG_OFF_ASSERTED).length,
    "the roster may only get shorter",
  ).toBeLessThanOrEqual(4);
});

test("IA §6: every route in router.tsx is swept at 375px", () => {
  const routes = [...router.matchAll(/^\s*path: "([^"]+)"/gm)].map((m) => shapePath(m[1]));

  const table = readFileSync(join(src, "..", "e2e", "routes.ts"), "utf8");
  const swept = [...table.matchAll(/^\s*\["[^"]*",\s*(?:"([^"]*)"|`([^`]*)`)\],/gm)].map((m) =>
    shapePath(m[1] ?? m[2]),
  );
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

test("IA §4: every route's search params are the ones the document lists", () => {
  const actual = new Map<string, string[]>();
  for (const block of router.split("createRoute({").slice(1)) {
    const path = /^\s*(?:getParentRoute:[^\n]*\n)?\s*path: "([^"]+)"/m.exec(block);
    if (!path) continue;
    const at = block.indexOf("validateSearch:");
    if (at === -1) continue;

    const open = block.indexOf("({", at) + 1;
    let depth = 0;
    let end = open;
    while (end < block.length) {
      if (block[end] === "{") depth++;
      else if (block[end] === "}" && --depth === 0) break;
      end++;
    }
    const body = block.slice(open + 1, end);

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

  expect(actual.size, "no validateSearch parsed — the scan broke").toBeGreaterThanOrEqual(7);

  const documented = new Map<string, string[]>();
  for (const line of section("4.").split("\n")) {
    if (!line.startsWith("| `/")) continue;
    const cells = line.split("|").map((c) => c.trim());
    const address = paths(cells[1])[0];
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
