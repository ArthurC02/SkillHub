import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUNS, TEST_CASE, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

type Arrival = "populated" | "empty" | "more" | 503;

function runId(index: number) {
  return `dddddddd-dddd-4ddd-8ddd-${String(index).padStart(12, "0")}`;
}

async function historyPages(page: Page, arrival: Arrival) {
  await stubPlatform(page);
  const offsets: number[] = [];
  const writes: string[] = [];
  const first = Array.from({ length: 51 }, (_, index) => ({
    ...RUNS.runs[0],
    run_id: runId(index + 1),
  }));
  const next = Array.from({ length: arrival === "more" ? 51 : 1 }, (_, index) => ({
    ...RUNS.runs[0],
    run_id: runId(index + 101),
  }));
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  let release!: () => void;
  let recovered = false;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/runs?*", async (route) => {
    const url = new URL(route.request().url());
    expect(url.searchParams.get("limit")).toBe("51");
    expect(url.searchParams.get("test_case_id")).toBe(TEST_CASE);
    const offset = Number(url.searchParams.get("offset"));
    offsets.push(offset);
    if (offset === 0) return route.fulfill({ json: { runs: first } });
    if (offset === 100) return route.fulfill({ json: { runs: [] } });
    expect(offset).toBe(50);
    await gate;
    if (arrival === 503 && !recovered)
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    return route.fulfill({ json: { runs: arrival === "empty" ? [] : next } });
  });
  return {
    offsets,
    writes,
    release,
    recover: () => {
      recovered = true;
    },
  };
}

for (const arrival of ["populated", "empty", "more", 503] as const) {
  for (const moved of [false, true]) {
    test(`history ${arrival} pagination ${moved ? "preserves moved focus" : "continues keyboard focus"}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const { offsets, writes, release, recover } = await historyPages(page, arrival);
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
        const region = page.locator(".test-case-history-panel");
        const rows = region.locator("[data-role=evidence] > li");
        const heading = region.getByRole("heading", { name: "執行歷史", exact: true });
        const more = region.getByRole("button").filter({ hasText: /^(載入更多|載入中…)$/ });
        const target = (index: number) =>
          rows.filter({ has: page.locator(`a[href="/runs/${runId(index)}"]`) });
        await expect(rows).toHaveCount(50);
        await more.focus();
        await page.keyboard.press("Enter");
        await expect(more).toHaveText("載入中…");
        await expect(more).toBeDisabled();
        const navigation = page
          .getByRole("navigation", { name: "主要導覽", exact: true })
          .getByRole("link", { name: "工作台", exact: true });
        if (moved) await navigation.focus();
        release();
        if (arrival === 503) {
          await expect(region.getByRole("alert")).toContainText("尚未讀完");
          await expect(rows).toHaveCount(50);
          await expect(more).toHaveCount(0);
          if (!moved) {
            await expect(heading).toBeFocused();
            await page.screenshot({
              path: test.info().outputPath("history-pagination-failed.png"),
            });
            recover();
            await region.getByRole("button", { name: "再試一次", exact: true }).focus();
            await page.keyboard.press("Enter");
            await expect(rows).toHaveCount(51);
            await expect(region.getByRole("alert")).toHaveCount(0);
            await expect(heading).toBeFocused();
          }
        } else {
          await expect(rows).toHaveCount(arrival === "empty" ? 50 : arrival === "more" ? 100 : 51);
          if (arrival !== "more") await expect(more).toHaveCount(0);
          if (!moved && arrival === "more") {
            await expect(more).toBeFocused();
            await page.keyboard.press("Enter");
            await expect(more).toHaveCount(0);
            await expect(target(150)).toBeFocused();
          } else if (!moved) await expect(target(arrival === "empty" ? 50 : 101)).toBeFocused();
        }
        if (moved) await expect(navigation).toBeFocused();
        expect(offsets).toEqual(
          arrival === 503 && !moved
            ? [0, 50, 50]
            : arrival === "more" && !moved
              ? [0, 50, 100]
              : [0, 50],
        );
        expect(writes).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        const accessibility = await new AxeBuilder({ page }).analyze();
        await writeFile(
          test.info().outputPath("history-pagination-a11y.json"),
          JSON.stringify(accessibility),
        );
        expect(accessibility.violations).toEqual([]);
        expect(accessibility.incomplete).toEqual([]);
        await page.screenshot({ path: test.info().outputPath("history-pagination.png") });
      } finally {
        release();
        await writeFile(
          test.info().outputPath("history-pagination-state.json"),
          JSON.stringify({
            arrival,
            moved,
            offsets,
            writes,
            focus: await page.evaluate(() => ({
              tag: document.activeElement?.tagName,
              id: document.activeElement?.id,
            })),
          }),
        );
      }
    });
  }
}
