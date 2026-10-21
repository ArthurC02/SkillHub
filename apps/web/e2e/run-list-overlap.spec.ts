import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, RUNS, SKILL_VERSIONS, platformResponse } from "../src/testing/fixtures/platform";
import type { RunListItem } from "../src/features/runs/runs.service";
import { stubPlatform } from "./stub";

const BOUNDARY = "444444ab-4444-4444-8444-000000000050";
const FIRST_NEW = "444444ab-4444-4444-8444-000000000051";
const REASON = "沙箱在第 3 步逾時。";

function latest(run: RunListItem, uppercase: boolean) {
  const result = { ...run, run_id: uppercase ? run.run_id.toUpperCase() : run.run_id };
  delete result.status_reason;
  return result;
}

async function stubComparisonSource(page: Page) {
  await stubPlatform(page);
  const sourceVersion = SKILL_VERSIONS.versions.find((version) => version.version_number === 1)!;
  await page.route(`**/runs/${RUN}`, (route) =>
    route.request().resourceType() === "document"
      ? route.fallback()
      : route.fulfill({
          json: {
            ...(platformResponse(`/runs/${RUN}`).body as object),
            skill_version_id: sourceVersion.version_id,
          },
        }),
  );
}

async function capture(page: Page, path: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(path, JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
}

for (const mode of [
  "disjoint",
  "overlap",
  "case-equivalent",
  "overlap-only",
  "more-overlap",
] as const) {
  for (const width of [375, 1280]) {
    test(`${mode} Run pages preserve unique rows and ${width === 375 ? "keyboard continuation" : "moved focus"} at ${width}px`, async ({
      page,
    }, info) => {
      await stubComparisonSource(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const moved = width === 1280;
      const rows: RunListItem[] = Array.from({ length: 51 }, (_, index) => ({
        ...structuredClone(RUNS.runs[1]),
        run_id: `444444ab-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
      }));
      const offsets: number[] = [];
      const writes: string[] = [];
      const comparisons: string[] = [];
      let release!: () => void;
      let started!: () => void;
      const held = new Promise<void>((resolve) => (release = resolve));
      const readStarted = new Promise<void>((resolve) => (started = resolve));
      await page.route("**/runs?*", async (route) => {
        const url = new URL(route.request().url());
        const offset = Number(url.searchParams.get("offset"));
        offsets.push(offset);
        expect(url.searchParams.get("limit")).toBe("51");
        expect(url.searchParams.get("test_case_id")).toBe(RUNS.runs[0].test_case_id);
        if (offset === 0) return route.fulfill({ json: { runs: rows } });
        if (offset === 100 && mode === "more-overlap")
          return route.fulfill({ json: { runs: [rows[50]] } });
        if (offset !== 50) return route.fulfill({ status: 400, json: { error: "invalid offset" } });
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
        return route.fulfill({ json: { runs: next } });
      });
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        if (new URL(request.url()).pathname.endsWith("/comparison"))
          comparisons.push(request.url());
      });
      try {
        await page.goto(`/runs/${RUN}/compare`);
        const listRows = page.locator("main .download-list > .download-item");
        const boundaryButton = page.getByRole("button", {
          name: new RegExp(`試跑紀錄 ID ${BOUNDARY}$`),
        });
        const boundaryRow = listRows.filter({ has: boundaryButton });
        const summary = boundaryRow.getByText("試跑紀錄 ID", { exact: true });
        await expect(page.getByText("比較基準：v1", { exact: true })).toBeVisible();
        await expect(listRows).toHaveCount(50);
        await expect(boundaryRow.getByText(REASON, { exact: true })).toBeVisible();
        if (moved && mode === "case-equivalent") await summary.click();
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        await field.fill("分頁時保留的比較 ID");
        const more = page.getByRole("button", { name: /^(載入更早的試跑紀錄|載入中…)$/ });
        const movedTarget =
          mode === "case-equivalent"
            ? summary
            : page
                .getByRole("navigation", { name: "主要導覽" })
                .getByRole("link", { name: "工作台", exact: true });
        await more.focus();
        await page.keyboard.press("Enter");
        await readStarted;
        await expect(more).toBeDisabled();
        if (moved) await movedTarget.focus();
        release();
        if (mode === "more-overlap") {
          await expect(more).toHaveText("載入更早的試跑紀錄");
          await expect(more).toBeEnabled();
          await expect(listRows).toHaveCount(50);
          await expect(moved ? movedTarget : more).toBeFocused();
          expect(offsets).toEqual([0, 50]);
          await capture(page, info.outputPath("overlap-continuation-a11y.json"));
          await more.focus();
          await page.keyboard.press("Enter");
        }
        await expect(more).toHaveCount(0);
        await expect(listRows).toHaveCount(mode === "overlap-only" ? 50 : 51);
        await expect(boundaryButton).toHaveCount(1);
        if (mode !== "disjoint")
          await expect(boundaryRow.getByText(REASON, { exact: true })).toHaveCount(0);
        if (moved && mode !== "more-overlap") {
          await expect(movedTarget).toBeFocused();
          if (mode === "case-equivalent")
            await expect(summary.locator("..")).toHaveJSProperty("open", true);
        } else {
          const nextId = mode === "overlap-only" ? BOUNDARY : FIRST_NEW;
          const destination = listRows.getByRole("button", {
            name: new RegExp(`試跑紀錄 ID ${nextId}$`),
          });
          await expect(destination).toBeFocused();
          await expect(destination).toBeInViewport({ ratio: 1 });
        }
        await expect(field).toHaveValue("分頁時保留的比較 ID");
        expect(offsets).toEqual(mode === "more-overlap" ? [0, 50, 100] : [0, 50]);
        expect(writes).toEqual([]);
        expect(comparisons).toEqual([]);
        await expect(page.getByRole("alert")).toHaveCount(0);
        await capture(page, info.outputPath("unique-run-pages-a11y.json"));
        await page.screenshot({ path: info.outputPath("unique-run-pages.png") });
      } finally {
        release();
      }
    });
  }
}
