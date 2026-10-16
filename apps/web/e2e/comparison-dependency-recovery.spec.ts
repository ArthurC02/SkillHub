import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, SKILL } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

for (const dependency of ["self", "candidates", "versions"] as const) {
  for (const moved of [false, true]) {
    test(`comparison ${dependency} retry explains paused reads and ${moved ? "preserves moved focus" : "retains decision context"}`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const pattern =
        dependency === "self"
          ? `**/runs/${RUN}`
          : dependency === "candidates"
            ? "**/runs?*"
            : `**/skills/${SKILL}/versions`;
      const what =
        dependency === "self"
          ? "目前這次試跑"
          : dependency === "candidates"
            ? "可比較的試跑紀錄"
            : "候選試跑紀錄的 Version 編號";
      const reads: string[] = [];
      const writes: string[] = [];
      const comparisonReads: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
        if (new URL(request.url()).pathname.endsWith("/comparison"))
          comparisonReads.push(request.url());
      });
      await page.route(pattern, (route) => {
        reads.push(route.request().url());
        return reads.length === 1
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fallback();
      });
      await page.goto(`/runs/${RUN}/compare`);
      const main = page.getByRole("main");
      await expect(main.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
      const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
      await field.fill("尚未送出的比較 ID");
      const retry = main.getByRole("button", { name: "再試一次", exact: true });
      const heading = page.locator("#run-comparison-heading");
      const destination = page
        .getByRole("navigation", { name: "主要導覽" })
        .getByRole("link", { name: "工作台", exact: true });
      try {
        await setOffline(page, true);
        await retry.focus();
        await page.keyboard.press("Enter");
        await expect(
          main
            .getByRole("status")
            .filter({ hasText: `等待連線以讀取${what}；恢復連線後會繼續讀取。` }),
        ).toBeVisible();
        await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(retry).toHaveCount(0);
        await expect(heading).toBeFocused();
        await expect(field).toHaveValue("尚未送出的比較 ID");
        expect(reads).toHaveLength(1);
        if (moved) await destination.focus();
        const pausedAccessibility = await new AxeBuilder({ page }).analyze();
        await writeFile(
          testInfo.outputPath("dependency-paused-accessibility.json"),
          JSON.stringify(pausedAccessibility),
        );
        expect(pausedAccessibility.violations).toEqual([]);
        expect(pausedAccessibility.incomplete).toEqual([]);
        await page.screenshot({ path: testInfo.outputPath("dependency-paused.png") });
        await setOffline(page, false);
        await expect(main.getByRole("button", { name: /^以 v2、/ })).toBeVisible();
        await expect(main.getByRole("status").filter({ hasText: "等待連線以讀取" })).toHaveCount(0);
        await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(field).toHaveValue("尚未送出的比較 ID");
        if (moved) await expect(destination).toBeFocused();
        else await expect(heading).toBeFocused();
        expect(reads).toHaveLength(2);
        expect(reads[0]).toBe(reads[1]);
        expect(comparisonReads).toEqual([]);
        expect(writes).toEqual([]);
        const recoveredAccessibility = await new AxeBuilder({ page }).analyze();
        await writeFile(
          testInfo.outputPath("dependency-recovered-accessibility.json"),
          JSON.stringify(recoveredAccessibility),
        );
        expect(recoveredAccessibility.violations).toEqual([]);
        expect(recoveredAccessibility.incomplete).toEqual([]);
        await page.screenshot({ path: testInfo.outputPath("dependency-recovered.png") });
      } finally {
        await setOffline(page, false);
      }
    });
  }
}
