import { expect, test, type Page, type TestInfo } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { CATALOG } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const CASES = [
  { name: "cached zero", initialTotal: 0, recoveredTotal: 3, malformed: false },
  { name: "cached count", initialTotal: 3, recoveredTotal: 0, malformed: false },
  { name: "malformed count", initialTotal: 3, recoveredTotal: 3, malformed: true },
];
const CATEGORY_COUNTS = ["全部（3）", "文件（1）", "寫作（1）", "資料（1）"];

async function stubCounts(page: Page, scenario: (typeof CASES)[number]) {
  let phase: "initial" | "failed" | "recovering" = "initial";
  let releaseFailure!: () => void;
  let releaseRecovery!: () => void;
  const failure = new Promise<void>((resolve) => {
    releaseFailure = resolve;
  });
  const recovery = new Promise<void>((resolve) => {
    releaseRecovery = resolve;
  });
  const reads: Record<string, string[]> = { initial: [], failed: [], recovering: [] };
  const primary: string[] = [];
  await page.route("**/api/skills/catalog?*", async (route) => {
    const url = new URL(route.request().url());
    const category = url.searchParams.get("category");
    const count = url.searchParams.get("limit") === "1";
    const current = phase;
    expect(route.request().method()).toBe("GET");
    if (count) {
      reads[current].push(category ?? "all");
      if (current === "failed") await failure;
      if (current === "recovering") await recovery;
      if (current === "failed" && !scenario.malformed) {
        await route.fulfill({ status: 503, body: "" });
        return;
      }
    } else primary.push(route.request().url());
    const empty = count
      ? (current === "initial" ? scenario.initialTotal : scenario.recoveredTotal) === 0
      : current === "initial" && scenario.initialTotal === 0;
    const rows = empty
      ? []
      : CATALOG.results.filter((row) => !category || row.category.value === category);
    await route.fulfill({
      json: {
        ...CATALOG,
        results: count ? rows.slice(0, 1) : rows,
        limit: count ? 1 : 100,
        total: count && current === "failed" && scenario.malformed ? "0" : rows.length,
        truncated: count && rows.length > 1,
      },
    });
  });
  return {
    reads,
    primary,
    releaseFailure,
    releaseRecovery,
    advance: (next: "failed" | "recovering") => {
      phase = next;
    },
  };
}

async function assertAccessible(page: Page, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(`category-${state}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
}

for (const scenario of CASES) {
  for (const width of [375, 1280]) {
    test(`category counts ${scenario.name} recover in place at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const { reads, primary, releaseFailure, releaseRecovery, advance } = await stubCounts(
        page,
        scenario,
      );
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      try {
        await page.goto("/");
        const main = page.getByRole("main");
        const nav = main.getByRole("navigation", { name: "分類", exact: true });
        await expect.poll(() => reads.initial.length).toBe(4);
        await expect(main.locator(".search-results > li")).toHaveCount(scenario.initialTotal);
        if (scenario.initialTotal === 0) await expect(nav).toHaveCount(0);
        else await expect(nav.getByRole("link")).toHaveText(CATEGORY_COUNTS);
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        advance("failed");
        await page.goBack();
        await expect.poll(() => reads.failed.length).toBe(4);
        const rows = main.locator(".search-results > li");
        await expect(rows).toHaveCount(3);
        const input = main.getByLabel("任務描述", { exact: true });
        await input.fill("保留我的任務草稿");
        const checkbox = rows.last().getByRole("checkbox");
        await checkbox.focus();
        await page.keyboard.press("Space");
        await expect(checkbox).toBeChecked();
        const selectedURL = page.url();
        await checkbox.evaluate((node) => node.scrollIntoView({ block: "end" }));
        releaseFailure();
        await expect(nav.getByRole("link")).toHaveText([
          "全部（測量失敗）",
          "文件（測量失敗）",
          "寫作（測量失敗）",
          "資料（測量失敗）",
        ]);
        await expect(main.getByRole("alert")).toContainText("暫時無法讀取分類數量");
        await expect(checkbox).toBeFocused();
        await writeFile(
          info.outputPath("category-focus.json"),
          JSON.stringify(
            await checkbox.evaluate((node) => ({
              top: node.getBoundingClientRect().top,
              bottom: node.getBoundingClientRect().bottom,
              height: innerHeight,
            })),
          ),
        );
        await page.screenshot({
          path: info.outputPath("category-count-failed.png"),
          fullPage: false,
        });
        expect(
          await checkbox.evaluate((node) => {
            const rect = node.getBoundingClientRect();
            return rect.top >= 0 && rect.bottom <= innerHeight;
          }),
        ).toBe(true);
        await expect(input).toHaveValue("保留我的任務草稿");
        expect(page.url()).toBe(selectedURL);
        await assertAccessible(page, info, "failed");
        advance("recovering");
        await page.context().setOffline(true);
        try {
          await main.getByRole("button", { name: "再試一次", exact: true }).click();
          await expect(
            main.getByText("等待連線以讀取分類數量；恢復連線後會繼續讀取。", { exact: true }),
          ).toBeVisible();
          await expect(
            main.getByRole("button", { name: "重新讀取中…", exact: true }),
          ).toBeDisabled();
          expect(reads.recovering).toEqual([]);
          await expect(page.locator("#catalog-search-heading")).toBeFocused();
        } finally {
          await page.context().setOffline(false);
        }
        await expect.poll(() => reads.recovering.length).toBe(4);
        await expect(main.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
        await page.keyboard.press("Enter");
        expect(reads.recovering).toHaveLength(4);
        releaseRecovery();
        await expect(main.getByRole("alert")).toHaveCount(0);
        if (scenario.recoveredTotal === 0) await expect(nav).toHaveCount(0);
        else await expect(nav.getByRole("link")).toHaveText(CATEGORY_COUNTS);
        await expect(checkbox).toBeChecked();
        await expect(input).toHaveValue("保留我的任務草稿");
        await expect(page.locator("#catalog-search-heading")).toBeFocused();
        expect(page.url()).toBe(selectedURL);
        expect(primary).toHaveLength(2);
        for (const calls of Object.values(reads))
          expect(calls.slice().sort()).toEqual(["all", "data", "documents", "writing"]);
        expect(errors).toEqual([]);
        await assertAccessible(page, info, "recovered");
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({
          path: info.outputPath("category-count-recovered.png"),
          fullPage: false,
        });
      } finally {
        releaseFailure();
        releaseRecovery();
      }
    });
  }
}
