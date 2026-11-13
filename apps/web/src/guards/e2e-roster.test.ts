import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const e2e = join(import.meta.dirname, "..", "..", "e2e");

const BROWSER_ONLY_SPECS = [
  "activity-pagination-focus.spec.ts",
  "admin-agents.spec.ts",
  "admin-operations.spec.ts",
  "audit-cell-layout.spec.ts",
  "audit-pagination-focus.spec.ts",
  "check-editor-layout.spec.ts",
  "check-editor-scroll.spec.ts",
  "comparison-control-names.spec.ts",
  "comparison-input-viewport.spec.ts",
  "comparison-phone-context.spec.ts",
  "continuation-outcome.spec.ts",
  "control-state-contrast.spec.ts",
  "creation-inspection.spec.ts",
  "creation-read-recovery.spec.ts",
  "credit-ledger-layout.spec.ts",
  "credit-statement-pagination-focus.spec.ts",
  "finding-pagination-focus.spec.ts",
  "fork-recovery.spec.ts",
  "header-scroll-clearance.spec.ts",
  "history-pagination-focus.spec.ts",
  "import-outcome-recovery.spec.ts",
  "nav-scroll-cue.spec.ts",
  "navigation-focus.spec.ts",
  "navigation-pending.spec.ts",
  "rendered.spec.ts",
  "route-accessibility.spec.ts",
  "route-error.spec.ts",
  "run-comparison.spec.ts",
  "run-identity-disclosure.spec.ts",
  "run-list-overlap.spec.ts",
  "search-read-recovery.spec.ts",
  "search-reasons-focus.spec.ts",
  "session-boundary.spec.ts",
  "session-readiness.spec.ts",
  "session-tabs.spec.ts",
  "settings-retry-focus.spec.ts",
  "shots.spec.ts",
  "socket-diagnostics.spec.ts",
  "startup-recovery.spec.ts",
  "static-assets.spec.ts",
  "workspace-pagination-focus.spec.ts",
];

const MAX_TEST_CALLS = 179;

const specs = () => readdirSync(e2e).filter((f) => f.endsWith(".spec.ts"));

test("a new e2e spec is a deliberate addition to the browser-only roster", () => {
  expect(
    specs().sort(),
    "頁面的錯誤路徑、空清單、重試與離線寫 vitest（apps/web/AGENTS.md）；只有真瀏覽器才看得到的事才加進這份名冊",
  ).toEqual(BROWSER_ONLY_SPECS);
});

test("the e2e suite does not grow new test cases inside existing specs", () => {
  const calls = specs().reduce(
    (sum, f) => sum + (readFileSync(join(e2e, f), "utf8").match(/\btest\(/g) ?? []).length,
    0,
  );
  expect(calls, "e2e 多了 test()：先確認它只能在真瀏覽器裡驗證，再調高上限").toBeLessThanOrEqual(
    MAX_TEST_CALLS,
  );
});
