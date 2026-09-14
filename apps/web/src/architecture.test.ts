import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { expect, test } from "vitest";

const src = import.meta.dirname;
const NOT_PRODUCT = new Set(["fixtures", "__outlines__"]);

type Source = { abs: string; path: string; layer: string; body: string };
type Import = { from: string; clause: string; typeOnly: boolean };

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const abs = join(dir, entry.name);
    if (entry.isDirectory()) return NOT_PRODUCT.has(entry.name) ? [] : walk(abs);
    return /\.tsx?$/.test(entry.name) && !/\.(test|spec)\.tsx?$/.test(entry.name) ? [abs] : [];
  });
}

function layerOf(abs: string): string {
  const parts = relative(src, abs).split(sep);
  return parts.length === 1 ? "root" : parts[0];
}

const sources: Source[] = walk(src).map((abs) => ({
  abs,
  path: relative(src, abs).split(sep).join("/"),
  layer: layerOf(abs),
  body: readFileSync(abs, "utf8"),
}));

function importsOf(body: string): Import[] {
  return [...body.matchAll(/^import\s+(type\s+)?([^;]*?)\s*from\s+"([^"]+)"/gm)].map(
    ([, type, clause, from]) => ({ from, clause, typeOnly: Boolean(type) }),
  );
}

function targetLayer(source: Source, from: string): string | undefined {
  if (!from.startsWith(".")) return undefined;
  return layerOf(resolve(dirname(source.abs), from));
}

function valueNames(clause: string): string[] {
  const braces = clause.match(/\{([^}]*)\}/)?.[1] ?? "";
  return braces
    .split(",")
    .map((name) => name.trim())
    .filter((name) => name !== "" && !name.startsWith("type "));
}

const LAYERS = ["api", "components", "pages"];
const MAY_IMPORT: Record<string, string[]> = {
  api: ["api"],
  components: ["components", "api"],
  pages: ["components", "api"],
};

test("the walk sees every layer, so an empty scan cannot pass", () => {
  for (const layer of LAYERS) {
    expect(
      sources.filter((s) => s.layer === layer).length,
      `no source found in ${layer}/`,
    ).toBeGreaterThan(5);
  }
  expect(importsOf('import { a,\n  b } from "./x";\nimport type { C } from "../y";')).toEqual([
    { from: "./x", clause: "{ a,\n  b }", typeOnly: false },
    { from: "../y", clause: "{ C }", typeOnly: true },
  ]);
});

test("ADR-081 R1: pages import components and api, components import api, api imports neither", () => {
  const violations = sources
    .filter((s) => s.layer in MAY_IMPORT)
    .flatMap((s) =>
      importsOf(s.body).flatMap(({ from, typeOnly }) => {
        const target = targetLayer(s, from);
        if (!target || MAY_IMPORT[s.layer].includes(target)) return [];
        if (typeOnly && target === "root") return [];
        return [`${s.path} → ${from}`];
      }),
    );
  expect(
    violations,
    "an import that points up a layer or sideways into another page — move the shared piece down into components/ (UI, vocabulary) or api/ (server data)",
  ).toEqual([]);
});

test("ADR-081 R1: api/ holds no component", () => {
  expect(
    sources.filter((s) => s.layer === "api" && s.path.endsWith(".tsx")).map((s) => s.path),
  ).toEqual([]);
});

test("ADR-081 R2: outside api/, react-query supplies only the provider and types", () => {
  const apiHookImports = sources
    .filter((s) => s.layer === "api")
    .flatMap((s) => importsOf(s.body))
    .filter((i) => i.from === "@tanstack/react-query");
  expect(
    apiHookImports.length,
    "api/ imports react-query nowhere — the matcher broke",
  ).toBeGreaterThan(5);

  const violations = sources
    .filter((s) => s.layer !== "api")
    .flatMap((s) =>
      importsOf(s.body)
        .filter((i) => i.from === "@tanstack/react-query" && !i.typeOnly)
        .flatMap((i) => valueNames(i.clause).filter((name) => name !== "QueryClientProvider"))
        .map((name) => `${s.path}: ${name}`),
    );
  expect(
    violations,
    "a query, mutation or cache call outside api/ — wrap it in an api/ hook that declares its own invalidations",
  ).toEqual([]);
});

const LITERAL_KEY = /queryKey:\s*\[|(?:set|get)QueryData(?:<[^>]*>)?\(\s*\[|getQueryState\(\s*\[/g;

test("ADR-081 R3: every cache key is built by api/queryKeys.ts", () => {
  expect('invalidateQueries({ queryKey: ["x"] })').toMatch(LITERAL_KEY);
  expect('setQueryData<Session>(["x"], s)').toMatch(LITERAL_KEY);
  expect(
    sources.filter((s) => s.body.includes("queryKeys.")).length,
    "nothing uses queryKeys — the scan is not reading api/",
  ).toBeGreaterThan(10);

  const violations = sources
    .filter((s) => s.path !== "api/queryKeys.ts")
    .flatMap((s) => [...s.body.matchAll(LITERAL_KEY)].map((m) => `${s.path}: ${m[0]}`));
  expect(violations, "a hand-written cache key — add it to api/queryKeys.ts").toEqual([]);
});

test("ADR-081 R5: retry is decided once, in api/queryClient.ts", () => {
  const client = sources.find((s) => s.path === "api/queryClient.ts");
  expect(client?.body, "api/queryClient.ts moved").toMatch(/retry:\s*false/);
  expect(
    sources.filter((s) => s !== client && /\bretry:/.test(s.body)).map((s) => s.path),
    "a per-hook retry — the default lives in api/queryClient.ts",
  ).toEqual([]);
});
