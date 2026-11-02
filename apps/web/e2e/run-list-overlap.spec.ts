import AxeBuilder from "./axe";
import { expect, test, type Page } from "@playwright/test";
import { RUN, RUNS, SKILL_VERSIONS, platformResponse } from "../src/testing/fixtures/platform";
import type { RunListItem } from "../src/features/runs/runs.service";
import { stubPlatform } from "./stub";

const FIRST_NEW = "444444ab-4444-4444-8444-000000000051";

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

for (const mode of ["disjoint", "more-overlap"] as const) {
  test(`${mode} Run pages preserve unique rows and keyboard continuation at 375px`, async ({
    page,
  }) => {
    await stubComparisonSource(page);
    await page.setViewportSize({ width: 375, height: 900 });
    const rows: RunListItem[] = Array.from({ length: 51 }, (_, index) => ({
      ...structuredClone(RUNS.runs[1]),
      run_id: `444444ab-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
    }));
    await page.route("**/runs?*", async (route) => {
      const offset = Number(new URL(route.request().url()).searchParams.get("offset"));
      if (offset === 0) return route.fulfill({ json: { runs: rows } });
      if (offset === 100) return route.fulfill({ json: { runs: [rows[50]] } });
      const next = mode === "disjoint" ? [rows[50]] : [...rows.slice(0, 49), rows[49], rows[50]];
      return route.fulfill({ json: { runs: next } });
    });
    await page.goto(`/runs/${RUN}/compare`);
    const listRows = page.locator("main .download-list > .download-item");
    const more = page.getByRole("button", { name: /^(載入更早的試跑紀錄|載入中…)$/ });
    await expect(listRows).toHaveCount(50);
    await more.focus();
    await page.keyboard.press("Enter");
    if (mode === "more-overlap") {
      await expect(more).toHaveText("載入更早的試跑紀錄");
      await more.focus();
      await page.keyboard.press("Enter");
    }
    await expect(more).toHaveCount(0);
    await expect(listRows).toHaveCount(51);
    const destination = listRows.getByRole("button", {
      name: new RegExp(`試跑紀錄 ID ${FIRST_NEW}$`),
    });
    await expect(destination).toBeFocused();
    await expect(destination).toBeInViewport({ ratio: 1 });
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  });
}
