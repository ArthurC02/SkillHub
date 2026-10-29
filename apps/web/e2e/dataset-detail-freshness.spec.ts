import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { platformResponse, TEST_CASE, VERSION } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

const stored = platformResponse(`/test-cases/${TEST_CASE}/datasets`).body;
const emptyList = { datasets: [], total_bytes: 0 };
const scenarios = ["pending", "offline"].flatMap((phase) =>
  [false, true].map((empty) => ({ phase, empty })),
);

for (const { phase, empty } of scenarios) {
  for (const width of [375, 1280]) {
    test(`cached detail datasets ${empty ? "empty" : "populated"} during ${phase} wait for current facts at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      let reads = 0;
      const writes: string[] = [];
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.on("request", (request) => {
        if (request.method() !== "GET" && request.url().includes("/test-cases"))
          writes.push(request.url());
      });
      await page.route(`**/test-cases/${TEST_CASE}/datasets`, async (route) => {
        if (route.request().isNavigationRequest()) return route.fallback();
        reads++;
        if (reads === 2) await gate;
        return route.fulfill({
          json: reads === 1 ? (empty ? emptyList : stored) : empty ? stored : emptyList,
        });
      });
      const section = page.getByRole("region", { name: "測試資料", exact: true });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
        if (empty)
          await expect(section.getByText("還沒有上傳任何檔案。", { exact: true })).toBeVisible();
        else await expect(section.locator(".file-tree > li")).toHaveCount(1);
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        if (phase === "offline") await setOffline(page, true);
        await page.goBack();
        const name = page.getByLabel("名稱", { exact: true });
        await name.fill("kept while checking stored files");
        if (phase === "pending") await expect.poll(() => reads).toBe(2);
        const waiting = section.getByRole("status").filter({
          hasText: phase === "offline" ? "等待連線以讀取檔案清單" : "載入檔案清單中",
        });
        await section.scrollIntoViewIfNeeded();
        await expect(waiting).toBeVisible();
        await expect(section.getByText("還沒有上傳任何檔案。", { exact: true })).toHaveCount(0);
        await expect(section.getByText(/目前 \d+ 個檔案/)).toHaveCount(0);
        await expect(section.locator(".file-tree > li")).toHaveCount(0);
        await expect(
          section.getByRole("button", { name: "刪除這個檔案", exact: true }),
        ).toHaveCount(0);
        expect(writes).toEqual([]);
        const audit = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(info.outputPath("dataset-detail-waiting-a11y.json"), JSON.stringify(audit));
        expect(audit.violations).toEqual([]);
        expect(audit.incomplete).toEqual([]);
        await waiting.scrollIntoViewIfNeeded();
        await page.screenshot({ path: info.outputPath("dataset-detail-waiting.png") });
        release();
        if (phase === "offline") await setOffline(page, false);
        await expect(waiting).toHaveCount(0);
        await expect(section.locator(".file-tree > li")).toHaveCount(empty ? 1 : 0);
        if (!empty)
          await expect(section.getByText("還沒有上傳任何檔案。", { exact: true })).toBeVisible();
        await expect(name).toHaveValue("kept while checking stored files");
        const refresh = section.getByRole("button", { name: "重新整理", exact: true });
        await refresh.focus();
        await refresh.press("Enter");
        await expect.poll(() => reads).toBe(3);
        const heading = page.locator("#test-case-datasets-heading");
        await expect(heading).toBeFocused();
        const geometry = await heading.evaluate((element) => ({
          top: element.getBoundingClientRect().top,
          bottom: element.getBoundingClientRect().bottom,
          headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
          viewportHeight: innerHeight,
        }));
        await writeFile(info.outputPath("dataset-focus-geometry.json"), JSON.stringify(geometry));
        expect(geometry.top).toBeGreaterThanOrEqual(geometry.headerBottom);
        expect(geometry.bottom).toBeLessThanOrEqual(geometry.viewportHeight);
        await expect(name).toHaveValue("kept while checking stored files");
        expect(writes).toEqual([]);
        expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({ path: info.outputPath("dataset-detail-refreshed.png") });
      } finally {
        release();
        if (phase === "offline") await setOffline(page, false);
      }
    });
  }
}
