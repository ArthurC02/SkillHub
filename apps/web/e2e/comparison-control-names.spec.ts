import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { CATALOG, SEARCH, SKILL_B, SKILL_C } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const FOURTH = "aaaaaaaa-4444-4444-4444-444444444444";

async function comparisonCandidates(page: Page, surface: "catalog" | "search") {
  await stubPlatform(page);
  const fourth = {
    ...CATALOG.results[2],
    skill_id: FOURTH,
    name: "JSON Formatter",
    summary: "格式化 JSON",
  };
  const hits =
    surface === "catalog"
      ? [...CATALOG.results, fourth]
      : [...SEARCH.results, CATALOG.results[2], fourth];
  const pattern = surface === "catalog" ? "**/api/skills/catalog?*" : "**/api/skills/search**";
  await page.route(pattern, (route) => {
    if (surface === "catalog" && new URL(route.request().url()).searchParams.get("limit") !== "100")
      return route.fallback();
    return route.fulfill({
      json:
        surface === "catalog"
          ? { ...CATALOG, results: hits, total: 4 }
          : { ...SEARCH, results: hits, total: 4, degraded: false, partial_index: false },
    });
  });
}

async function toggleFromKeyboard(page: Page, pick: Locator, checked: boolean) {
  await pick.focus();
  await page.keyboard.press("Space");
  await expect(pick).toBeChecked({ checked });
  await expect(pick).toBeFocused();
  expect(
    await pick.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const top = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
      return Boolean(top && (top === element || element.parentElement?.contains(top)));
    }),
  ).toBe(true);
}

for (const surface of ["catalog", "search"] as const) {
  test(`${surface} comparison controls identify each candidate throughout keyboard selection`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.emulateMedia({ colorScheme: surface === "catalog" ? "light" : "dark" });
    await comparisonCandidates(page, surface);
    let documents = 0;
    page.on("domcontentloaded", () => documents++);
    const list = page.locator(".search-results");
    try {
      await page.goto(surface === "catalog" ? "/" : "/?q=pdf");
      await expect(list.getByRole("checkbox")).toHaveCount(4);
      const [pdf, splitter, csv, formatter] = [
        "PDF Summariser",
        "Doc Splitter",
        "CSV Cleaner",
        "JSON Formatter",
      ].map((name) => page.getByRole("checkbox", { name: new RegExp(name) }));
      for (const pick of [pdf, splitter, csv, formatter]) {
        await expect(pick).toBeVisible();
        await expect(pick).toHaveAccessibleName(/比較/);
        await expect(pick).not.toBeChecked();
      }
      await toggleFromKeyboard(page, pdf, true);
      await toggleFromKeyboard(page, splitter, true);
      await toggleFromKeyboard(page, csv, true);
      await expect(formatter).toBeDisabled();
      await expect(formatter).toHaveAccessibleDescription(/已經選滿 3 個/);
      await expect(pdf).toBeEnabled();
      await toggleFromKeyboard(page, pdf, false);
      await expect(formatter).toBeEnabled();
      await expect(formatter).toHaveAccessibleDescription("");
      await toggleFromKeyboard(page, formatter, true);
      const comparison = page.getByRole("link", { name: "並排比較這 3 個小工具", exact: true });
      const target = new URL((await comparison.getAttribute("href")) ?? "", page.url());
      expect(target.pathname).toBe("/compare");
      expect(target.searchParams.get("ids")).toBe(`${SKILL_B},${SKILL_C},${FOURTH}`);
      const current = new URL(page.url());
      expect(current.searchParams.get("compare")).toBe(`${SKILL_B},${SKILL_C},${FOURTH}`);
      expect(current.searchParams.get("q")).toBe(surface === "search" ? "pdf" : null);
      expect(documents).toBe(1);
      const report = await new AxeBuilder({ page }).analyze();
      await writeFile(
        test.info().outputPath("comparison-control-a11y.json"),
        JSON.stringify(report),
      );
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
        375,
      );
      await page.screenshot({
        path: test.info().outputPath("comparison-control-phone.png"),
        fullPage: true,
      });
    } finally {
      await writeFile(
        test.info().outputPath("comparison-control-state.json"),
        JSON.stringify({ surface, documents, url: page.url(), aria: await list.ariaSnapshot() }),
      );
    }
  });
}
