import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const e2e = join(import.meta.dirname, "..", "..", "e2e");

const READS_PASSES = new Set(["audit-cell-layout.spec.ts", "rendered.spec.ts"]);

test("e2e specs reach axe through the project-scoped wrapper unless they read its passes", () => {
  const raw = readdirSync(e2e)
    .filter((f) => f.endsWith(".spec.ts") && !READS_PASSES.has(f))
    .filter((f) => readFileSync(join(e2e, f), "utf8").includes('from "@axe-core/playwright"'));
  expect(raw).toEqual([]);
});

test("every spec that is allowed the raw import still reads passes", () => {
  for (const f of READS_PASSES) {
    const source = readFileSync(join(e2e, f), "utf8");
    expect(source, f).toContain('from "@axe-core/playwright"');
    expect(source, f).toContain(".passes");
  }
});
