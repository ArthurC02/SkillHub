import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { platformResponse, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const fixture = platformResponse("/test-cases").body as {
  test_cases: Record<string, unknown>[];
};
const row = fixture.test_cases[0];
const scenarios = ["initial", "refresh", "next"].flatMap((phase) =>
  ["missing rubric observation", "unreadable row"].map((failure) => ({ phase, failure })),
);

for (const { phase, failure } of scenarios) {
  for (const width of [375, 1280]) {
    test(`test-case list ${phase} with ${failure} stays unknown and recovers at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const paged = Array.from({ length: 51 }, (_, index) => ({
        ...row,
        test_case_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
      }));
      const malformed = {
        test_cases: failure === "unreadable row" ? [null] : [{ ...row, has_rubric: undefined }],
      };
      const offsets: number[] = [];
      const writes: string[] = [];
      const errors: string[] = [];
      let recovered = false;
      page.on("pageerror", (error) => errors.push(error.message));
      page.on("request", (request) => {
        if (request.method() !== "GET" && request.url().includes("/test-cases"))
          writes.push(request.url());
      });
      await page.route("**/test-cases?*", (route) => {
        if (route.request().isNavigationRequest()) return route.fallback();
        const offset = Number(new URL(route.request().url()).searchParams.get("offset"));
        offsets.push(offset);
        const failed = !recovered && (phase === "initial" || offsets.length > 1);
        return route.fulfill({
          json: failed
            ? malformed
            : { test_cases: phase === "next" ? (offset === 0 ? paged : [paged[50]]) : [row] },
        });
      });
      await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
      const index = page.locator(".test-case-index");
      const rows = index.locator("[data-role=evidence] > li");
      const name = page.getByLabel("名稱", { exact: true });
      const prompt = page.getByLabel("User Prompt", { exact: true });
      if (phase !== "initial") {
        await expect(rows).toHaveCount(phase === "next" ? 50 : 1);
        await name.fill("kept new test-case name");
        await prompt.fill("kept new test-case prompt");
        await index
          .getByRole("button", { name: phase === "next" ? "載入更多" : "重新整理", exact: true })
          .click();
      }
      const alert = index.getByRole("alert");
      await expect(alert).toBeVisible();
      await expect(alert).toContainText(phase === "next" ? "尚未讀完" : "無法讀取測試題");
      await expect(rows).toHaveCount(phase === "next" ? 50 : 0);
      await expect(index.getByText("還沒有測試題。", { exact: true })).toHaveCount(0);
      await expect(index.getByText(/Rubric\s*無/)).toHaveCount(0);
      await expect(index.getByRole("button", { name: "載入更多", exact: true })).toHaveCount(0);
      if (phase === "initial") {
        await name.fill("kept new test-case name");
        await prompt.fill("kept new test-case prompt");
      }
      const audit = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(info.outputPath("test-case-list-a11y.json"), JSON.stringify(audit));
      expect(audit.violations).toEqual([]);
      expect(audit.incomplete).toEqual([]);
      await alert.scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath("test-case-list-unreadable.png") });
      recovered = true;
      const retry = index.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await retry.press("Enter");
      await expect(rows).toHaveCount(phase === "next" ? 51 : 1);
      await expect(alert).toHaveCount(0);
      await expect(page.locator("#existing-test-cases-heading")).toBeFocused();
      await expect(name).toHaveValue("kept new test-case name");
      await expect(prompt).toHaveValue("kept new test-case prompt");
      expect(offsets).toEqual(
        phase === "next" ? [0, 50, 50] : phase === "refresh" ? [0, 0, 0] : [0, 0],
      );
      expect(writes).toEqual([]);
      expect(errors).toEqual([]);
      expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: info.outputPath("test-case-list-recovered.png") });
    });
  }
}

for (const width of [375, 1280]) {
  test(`an unnamed test-case list row remains named for assistive technology at ${width}px`, async ({
    page,
  }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
    await page.route("**/test-cases?*", (route) => {
      if (route.request().isNavigationRequest()) return route.fallback();
      return route.fulfill({ json: { test_cases: [{ ...row, name: "", skill_name: "" }] } });
    });
    await page.goto(`/lab/test-cases?version=${VERSION}`);
    const index = page.locator(".test-case-index");
    const link = index.getByRole("link", { name: "未命名的測試題", exact: true });
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute(
      "href",
      `/lab/test-cases/${row.test_case_id}?version=${VERSION}`,
    );
    await expect(index.getByText("這個小工具已經不在你的清單裡（已刪除，或已下架）")).toBeVisible();
    const audit = await new AxeBuilder({ page }).include("main").analyze();
    await writeFile(info.outputPath("test-case-list-a11y.json"), JSON.stringify(audit));
    expect(audit.violations).toEqual([]);
    expect(audit.incomplete).toEqual([]);
    await page.screenshot({ path: info.outputPath("test-case-list-unnamed.png") });
  });
}
