import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { expect, test } from "vitest";

const src = join(import.meta.dirname, "..");
const NOT_PRODUCT = new Set(["guards", "testing"]);

type Source = { abs: string; path: string; zone: string; body: string };
type Import = { from: string; clause: string; typeOnly: boolean };

const pathOf = (abs: string) => relative(src, abs).split(sep).join("/");

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const abs = join(dir, entry.name);
    if (entry.isDirectory()) return NOT_PRODUCT.has(entry.name) ? [] : walk(abs);
    return /\.tsx?$/.test(entry.name) && !/\.(test|spec)\.tsx?$/.test(entry.name) ? [abs] : [];
  });
}

function zoneOf(path: string): string {
  const [first, second] = path.split("/");
  if (first === "features") return `features/${second}`;
  return path.includes("/") ? first : "root";
}

const sources: Source[] = walk(src).map((abs) => {
  const path = pathOf(abs);
  return { abs, path, zone: zoneOf(path), body: readFileSync(abs, "utf8") };
});

function importsOf(body: string): Import[] {
  const statics = [...body.matchAll(/^import\s+(type\s+)?([^;]*?)\s*from\s+"([^"]+)"/gm)].map(
    ([, type, clause, from]) => ({ from, clause, typeOnly: Boolean(type) }),
  );
  const dynamics = [...body.matchAll(/\bimport\(\s*"([^"]+)"\s*\)/g)].map(([, from]) => ({
    from,
    clause: "",
    typeOnly: false,
  }));
  return [...statics, ...dynamics];
}

function resolveImport(source: Source, from: string): string | undefined {
  if (!from.startsWith(".")) return undefined;
  const base = resolve(dirname(source.abs), from);
  const found = [base, `${base}.ts`, `${base}.tsx`, join(base, "index.ts")].find(
    (candidate) => existsSync(candidate) && statSync(candidate).isFile(),
  );
  return pathOf(found ?? base);
}

function edges(): Array<{ source: Source; target: string; typeOnly: boolean; from: string }> {
  return sources.flatMap((source) =>
    importsOf(source.body).flatMap(({ from, typeOnly }) => {
      const target = resolveImport(source, from);
      return target ? [{ source, target, typeOnly, from }] : [];
    }),
  );
}

const isPage = (path: string) => path.endsWith(".page.tsx");
const isService = (path: string) => path.endsWith(".service.ts");
const isFeatureIndex = (path: string) => /^features\/[^/]+\/index\.ts$/.test(path);

function mayImport(source: Source, target: string, typeOnly: boolean): boolean {
  const to = zoneOf(target);
  switch (source.zone) {
    case "root":
      return to === "root" || to === "app";
    case "core":
      return to === "core";
    case "shared":
      return to === "shared" || to === "core";
    case "app":
      return (
        to !== "root" && (!to.startsWith("features/") || isPage(target) || isFeatureIndex(target))
      );
    default:
      if (to === source.zone || to === "shared" || to === "core") return true;
      if (to.startsWith("features/")) return isFeatureIndex(target);
      return typeOnly && target === "app/router.tsx";
  }
}

function valueNames(clause: string): string[] {
  const braces = clause.match(/\{([^}]*)\}/)?.[1] ?? "";
  return braces
    .split(",")
    .map((name) => name.trim())
    .filter((name) => name !== "" && !name.startsWith("type "));
}

test("the walk sees every zone and every role, so an empty scan cannot pass", () => {
  const count = (keep: (s: Source) => boolean) => sources.filter(keep).length;
  expect(
    count((s) => s.zone === "core"),
    "core/",
  ).toBeGreaterThan(3);
  expect(
    count((s) => s.zone === "shared"),
    "shared/",
  ).toBeGreaterThan(10);
  expect(
    count((s) => s.zone === "app"),
    "app/",
  ).toBeGreaterThan(3);
  expect(new Set(sources.map((s) => s.zone).filter((z) => z.startsWith("features/"))).size).toBe(8);
  expect(
    count((s) => isService(s.path)),
    "*.service.ts",
  ).toBeGreaterThan(15);
  expect(
    count((s) => isPage(s.path)),
    "*.page.tsx",
  ).toBeGreaterThan(15);
  expect(
    importsOf('import { a,\n  b } from "./x";\nimport type { C } from "../y";\nimport("./z");'),
  ).toEqual([
    { from: "./x", clause: "{ a,\n  b }", typeOnly: false },
    { from: "../y", clause: "{ C }", typeOnly: true },
    { from: "./z", clause: "", typeOnly: false },
  ]);
});

test("ADR-082 決策 1: core, shared, features and app import only in the directions allowed", () => {
  const violations = edges()
    .filter(({ source, target, typeOnly }) => !mayImport(source, target, typeOnly))
    .map(({ source, from }) => `${source.path} → ${from}`);
  expect(
    violations,
    "core/ imports only core/; shared/ imports shared/ and core/; a feature imports its own " +
      "files, shared/, core/, and another feature only through that feature's index.ts; " +
      "app/ imports pages and index.ts files. Add the export to the other feature's index.ts, " +
      "or move a piece that two features need down into shared/ (UI) or core/ (session, http, keys)",
  ).toEqual([]);
});

test("ADR-082 決策 2: a sub-component in components/ is used only by the folder that owns it", () => {
  const violations = edges()
    .filter(({ target }) => target.includes("/components/"))
    .filter(({ source, target }) => {
      const owner = target.slice(0, target.lastIndexOf("/components/"));
      return !source.path.startsWith(`${owner}/`);
    })
    .map(({ source, target }) => `${source.path} → ${target}`);
  expect(
    violations,
    "a sub-component borrowed from outside the folder that owns it — lift it one level " +
      "(into the feature's components/, exported through index.ts) or into shared/ui/",
  ).toEqual([]);
});

test("ADR-082 決策 2: a page is a route — only app/ imports a *.page.tsx, and it imports every one", () => {
  const pages = sources.filter((s) => isPage(s.path)).map((s) => s.path);
  expect(
    pages.filter((p) => !p.startsWith("features/")),
    "a *.page.tsx outside features/",
  ).toEqual([]);

  const pageEdges = edges().filter(({ target }) => isPage(target));
  expect(
    pageEdges
      .filter(({ source }) => source.zone !== "app")
      .map(({ source, target }) => `${source.path} → ${target}`),
    "a page imported by something other than the router — what two screens share is a component",
  ).toEqual([]);
  expect(
    pages.filter((p) => !pageEdges.some(({ target }) => target === p)).sort(),
    "a *.page.tsx no route mounts — without an address it is a component, not a page",
  ).toEqual([]);
});

test("ADR-082 決策 2: index.ts is only a list of what the feature offers", () => {
  const indexes = sources.filter((s) => isFeatureIndex(s.path));
  expect(indexes.length, "no feature index.ts found").toBeGreaterThan(2);
  const stray = indexes.flatMap((s) =>
    s.body
      .split(";")
      .map((statement) => statement.trim())
      .filter((statement) => statement !== "")
      .filter((statement) => !/^export (type )?\{[^}]*\} from "\.\/[^"]+"$/.test(statement))
      .map((statement) => `${s.path}: ${statement}`),
  );
  expect(stray, 'index.ts holds only `export { … } from "./…";` statements').toEqual([]);
});

test("ADR-082 決策 3: react-query lives in *.service.ts; outside them only the provider and types", () => {
  const serviceImports = sources
    .filter((s) => isService(s.path))
    .flatMap((s) => importsOf(s.body))
    .filter((i) => i.from === "@tanstack/react-query");
  expect(
    serviceImports.length,
    "no service imports react-query — the matcher broke",
  ).toBeGreaterThan(10);

  const violations = sources
    .filter((s) => !isService(s.path) && !s.path.startsWith("core/api/"))
    .flatMap((s) =>
      importsOf(s.body)
        .filter((i) => i.from === "@tanstack/react-query" && !i.typeOnly)
        .flatMap((i) => valueNames(i.clause).filter((name) => name !== "QueryClientProvider"))
        .map((name) => `${s.path}: ${name}`),
    );
  expect(
    violations,
    "a query, mutation or cache call outside a *.service.ts — wrap it in a service hook that declares its own invalidations",
  ).toEqual([]);
});

test("ADR-082 決策 3: a service draws nothing — no JSX file, no component import", () => {
  expect(
    sources.filter((s) => /\.service\.tsx$/.test(s.path)).map((s) => s.path),
    "a *.service.tsx",
  ).toEqual([]);
  expect(
    edges()
      .filter(({ source, target }) => isService(source.path) && target.endsWith(".tsx"))
      .map(({ source, target }) => `${source.path} → ${target}`),
    "a service importing a component",
  ).toEqual([]);
});

const LITERAL_KEY = /queryKey:\s*\[|(?:set|get)QueryData(?:<[^>]*>)?\(\s*\[|getQueryState\(\s*\[/g;

test("ADR-082 決策 3: every cache key is built by core/api/queryKeys.ts", () => {
  expect('invalidateQueries({ queryKey: ["x"] })').toMatch(LITERAL_KEY);
  expect('setQueryData<Session>(["x"], s)').toMatch(LITERAL_KEY);
  expect(
    sources.filter((s) => s.body.includes("queryKeys.")).length,
    "nothing uses queryKeys — the scan is not reading the services",
  ).toBeGreaterThan(10);

  const violations = sources
    .filter((s) => s.path !== "core/api/queryKeys.ts")
    .flatMap((s) => [...s.body.matchAll(LITERAL_KEY)].map((m) => `${s.path}: ${m[0]}`));
  expect(violations, "a hand-written cache key — add it to core/api/queryKeys.ts").toEqual([]);
});

test("ADR-082 決策 3: retry is decided once, in core/api/queryClient.ts", () => {
  const client = sources.find((s) => s.path === "core/api/queryClient.ts");
  expect(client?.body, "core/api/queryClient.ts moved").toMatch(/retry:\s*false/);
  expect(
    sources.filter((s) => s !== client && /\bretry:/.test(s.body)).map((s) => s.path),
    "a per-hook retry — the default lives in core/api/queryClient.ts",
  ).toEqual([]);
});
