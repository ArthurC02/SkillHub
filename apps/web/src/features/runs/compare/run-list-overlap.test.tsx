import { expect, test } from "vitest";
import { RUN, RUNS, SKILL_VERSIONS, platformResponse } from "../../../testing/fixtures/platform";
import { Locator, renderedApp } from "../../../testing/app";
import type { RunListItem } from "../runs.service";

const app = renderedApp();

const BOUNDARY = "444444ab-4444-4444-8444-000000000050";
const FIRST_NEW = "444444ab-4444-4444-8444-000000000051";
const REASON = "沙箱在第 3 步逾時。";

function latest(run: RunListItem, uppercase: boolean) {
  const result = { ...run, run_id: uppercase ? run.run_id.toUpperCase() : run.run_id };
  delete result.status_reason;
  return result;
}

function rowHolding(rows: Locator, button: Locator): Locator {
  return new Locator(() =>
    rows.all().filter((row) => button.all().some((candidate) => row.contains(candidate))),
  );
}

function stubComparisonSource(page: ReturnType<typeof app>) {
  const sourceVersion = SKILL_VERSIONS.versions.find((version) => version.version_number === 1)!;
  page.route(`**/runs/${RUN}`, () => ({
    json: {
      ...(platformResponse(`/runs/${RUN}`).body as object),
      skill_version_id: sourceVersion.version_id,
    },
  }));
}

for (const mode of [
  "disjoint",
  "overlap",
  "case-equivalent",
  "overlap-only",
  "more-overlap",
] as const) {
  for (const width of [375, 1280]) {
    test(`${mode} Run pages preserve unique rows and ${width === 375 ? "keyboard continuation" : "moved focus"} at ${width}px`, async () => {
      const page = app();
      stubComparisonSource(page);
      const moved = width === 1280;
      const rows: RunListItem[] = Array.from({ length: 51 }, (_, index) => ({
        ...structuredClone(RUNS.runs[1]),
        run_id: `444444ab-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
      }));
      const offsets: number[] = [];
      let release!: () => void;
      let started!: () => void;
      const held = new Promise<void>((resolve) => (release = resolve));
      const readStarted = new Promise<void>((resolve) => (started = resolve));
      page.route("**/runs?*", async ({ url }) => {
        const offset = Number(url.searchParams.get("offset"));
        offsets.push(offset);
        expect(url.searchParams.get("limit")).toBe("51");
        expect(url.searchParams.get("test_case_id")).toBe(RUNS.runs[0].test_case_id);
        if (offset === 0) return { json: { runs: rows } };
        if (offset === 100 && mode === "more-overlap") return { json: { runs: [rows[50]] } };
        if (offset !== 50) return { status: 400, json: { error: "invalid offset" } };
        started();
        await held;
        const updated = latest(rows[49], mode === "case-equivalent");
        const next =
          mode === "disjoint"
            ? [rows[50]]
            : mode === "overlap-only"
              ? [updated]
              : mode === "more-overlap"
                ? [...rows.slice(0, 49), updated, rows[50]]
                : [updated, rows[50]];
        return { json: { runs: next } };
      });
      const writes = () =>
        page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
      const comparisons = () =>
        page.requests.filter((request) => request.url.pathname.endsWith("/comparison"));
      try {
        await page.goto(`/runs/${RUN}/compare`);
        const listRows = page.locator("main .download-list > .download-item");
        const boundaryButton = page.getByRole("button", {
          name: new RegExp(`試跑紀錄 ID ${BOUNDARY}$`),
        });
        const boundaryRow = rowHolding(listRows, boundaryButton);
        const reason = boundaryRow.locator("p.note").filter({ hasText: REASON });
        const summary = boundaryRow.locator("summary").filter({ hasText: "試跑紀錄 ID" });
        await page.eventually(() => {
          expect(page.getByText("比較基準：v1", { exact: true }).count()).toBe(1);
          expect(listRows.count()).toBe(50);
          expect(reason.count()).toBe(1);
        });
        if (moved && mode === "case-equivalent") await summary.click();
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        await field.fill("分頁時保留的比較 ID");
        const more = page.getByRole("button", { name: /^(載入更早的試跑紀錄|載入中…)$/ });
        const movedTarget =
          mode === "case-equivalent"
            ? summary
            : page.locator('nav[aria-label="主要導覽"] a').filter({ hasText: "工作台" });
        await more.focus();
        await page.press("Enter");
        await readStarted;
        await expect(more).toBeDisabled();
        if (moved) await movedTarget.focus();
        release();
        if (mode === "more-overlap") {
          await page.eventually(() => {
            expect(more.text()).toBe("載入更早的試跑紀錄");
            expect(more.isDisabled()).toBe(false);
            expect(listRows.count()).toBe(50);
            expect((moved ? movedTarget : more).isFocused()).toBe(true);
          });
          expect(offsets).toEqual([0, 50]);
          expect(await page.axeViolations("body")).toEqual([]);
          await more.focus();
          await page.press("Enter");
        }
        await page.eventually(() => {
          expect(more.count()).toBe(0);
          expect(listRows.count()).toBe(mode === "overlap-only" ? 50 : 51);
          expect(boundaryButton.count()).toBe(1);
          if (mode !== "disjoint") expect(reason.count()).toBe(0);
        });
        if (moved && mode !== "more-overlap") {
          await expect(movedTarget).toBeFocused();
          if (mode === "case-equivalent")
            expect((summary.element().parentElement as HTMLDetailsElement).open).toBe(true);
        } else {
          const nextId = mode === "overlap-only" ? BOUNDARY : FIRST_NEW;
          const destination = page.getByRole("button", {
            name: new RegExp(`試跑紀錄 ID ${nextId}$`),
          });
          await expect(destination).toBeFocused();
        }
        await expect(field).toHaveValue("分頁時保留的比較 ID");
        expect(offsets).toEqual(mode === "more-overlap" ? [0, 50, 100] : [0, 50]);
        expect(writes()).toEqual([]);
        expect(comparisons()).toEqual([]);
        await expect(page.getByRole("alert")).toHaveCount(0);
        expect(await page.axeViolations("body")).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
