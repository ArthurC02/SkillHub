import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { CATALOG, SEARCH, SKILL, SKILL_B } from "../src/testing/fixtures/platform";
import { emptySearchIntent } from "../src/core/api/searchIntent";
import { stubPlatform } from "./stub";

async function assertAccessible(page: Page, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(`response-${state}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
}

for (const mode of ["catalog", "search", "corrected"] as const) {
  for (const width of [375, 1280]) {
    test(`${mode} rejects malformed success and recovers without losing context at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      let phase: "initial" | "failed" | "recovering" = "initial";
      let releaseFailure!: () => void;
      let releaseRecovery!: () => void;
      const failure = new Promise<void>((resolve) => {
        releaseFailure = resolve;
      });
      const recovery = new Promise<void>((resolve) => {
        releaseRecovery = resolve;
      });
      const calls: { method: string; url: string; body: unknown }[] = [];
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      const valid = mode === "catalog" ? CATALOG : SEARCH;
      await page.route(
        mode === "catalog" ? /\/api\/skills\/catalog\?/ : /\/api\/skills\/search(\?|$)/,
        async (route) => {
          const request = route.request();
          if (mode === "catalog" && new URL(request.url()).searchParams.get("limit") === "1")
            return route.fallback();
          const current = phase;
          calls.push({
            method: request.method(),
            url: request.url(),
            body: request.method() === "POST" ? request.postDataJSON() : null,
          });
          if (current === "failed") await failure;
          if (current === "recovering") await recovery;
          const body =
            current !== "failed"
              ? valid
              : mode === "catalog"
                ? null
                : mode === "search"
                  ? { ...SEARCH, results: null }
                  : { ...SEARCH, results: SEARCH.results.map((hit) => ({ ...hit, risk: null })) };
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify(body),
          });
        },
      );
      const params = new URLSearchParams({ compare: `${SKILL},${SKILL_B}` });
      if (mode !== "catalog") params.set("q", "pdf 摘要");
      if (mode === "corrected")
        params.set(
          "correction",
          JSON.stringify({ intent: { ...emptySearchIntent(), input: "PDF" }, keywords: ["PDF"] }),
        );
      try {
        await page.goto(`/?${params}`);
        const main = page.getByRole("main");
        const rows = main.locator(".search-results > li");
        await expect(rows).toHaveCount(valid.results.length);
        await expect(rows.first().getByRole("checkbox")).toBeChecked();
        const originalURL = page.url();
        const originalRequest = calls[0];
        expect(originalRequest.method).toBe(mode === "corrected" ? "POST" : "GET");
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        phase = "failed";
        await page.goBack();
        await expect.poll(() => calls.length).toBe(2);
        const input = main.getByLabel("任務描述", { exact: true });
        await input.fill("保留尚未送出的任務草稿");
        releaseFailure();
        await expect(main.getByRole("alert")).toContainText(
          mode === "catalog" ? "暫時無法讀取目錄" : "暫時無法讀取搜尋結果",
        );
        await expect(rows).toHaveCount(0);
        await expect(input).toHaveValue("保留尚未送出的任務草稿");
        await expect(main.getByText("沒有夠接近的小工具。", { exact: true })).toHaveCount(0);
        expect(page.url()).toBe(originalURL);
        await page.screenshot({ path: info.outputPath("response-failed.png"), fullPage: false });
        await assertAccessible(page, info, "failed");
        phase = "recovering";
        await page.context().setOffline(true);
        try {
          await main.getByRole("button", { name: "再試一次", exact: true }).click();
          await expect(
            main.getByText(
              `等待連線以讀取${mode === "catalog" ? "目錄" : "搜尋結果"}；恢復連線後會繼續讀取。`,
              { exact: true },
            ),
          ).toBeVisible();
          expect(calls).toHaveLength(2);
        } finally {
          await page.context().setOffline(false);
        }
        await expect.poll(() => calls.length).toBe(3);
        await expect(main.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
        releaseRecovery();
        await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(rows).toHaveCount(valid.results.length);
        await expect(rows.first().getByRole("checkbox")).toBeChecked();
        await expect(rows.nth(1).getByRole("checkbox")).toBeChecked();
        await expect(input).toHaveValue("保留尚未送出的任務草稿");
        await expect(page.locator("#catalog-search-heading")).toBeFocused();
        expect(page.url()).toBe(originalURL);
        expect(calls).toEqual([originalRequest, originalRequest, originalRequest]);
        expect(errors).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await assertAccessible(page, info, "recovered");
        await page.screenshot({ path: info.outputPath("response-recovered.png"), fullPage: false });
      } finally {
        releaseFailure();
        releaseRecovery();
      }
    });
  }
}
