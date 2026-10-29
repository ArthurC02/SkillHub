import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { SKILL, SKILL_B, SKILL_C, skillDetail } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

for (const cached of [false, true]) {
  for (const width of [375, 1280]) {
    test(`skill comparison ${cached ? "cached" : "initial"} offline read waits for current columns at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const ids = [SKILL, SKILL_B, SKILL_C];
      const oldNames = ["摘要工具", "拆分工具", "清理工具"];
      const currentNames = ["更新摘要工具", "更新拆分工具", "更新清理工具"];
      const reads = [0, 0, 0];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      for (const [index, id] of ids.entries()) {
        await page.route(`**/api/skills/${id}?view=embedded`, (route) => {
          reads[index]++;
          return route.fulfill({
            json: skillDetail(
              id,
              cached && reads[index] === 1 ? oldNames[index] : currentNames[index],
            ),
          });
        });
      }
      const table = page.getByRole("table", { name: /並排比較/ });
      const refresh = page.getByRole("button", { name: /更新比較資料|等待連線/ });
      try {
        if (cached) {
          await page.goto(`/compare?ids=${ids.join(",")}`);
          await expect(table.getByRole("columnheader")).toHaveText(["比較項目", ...oldNames]);
          await page.locator('a[href="/policy"]').first().click();
          await expect(
            page.getByRole("heading", { name: "資料保存政策", exact: true }),
          ).toBeVisible();
          await setOffline(page, true);
          await page.goBack();
        } else {
          await page.goto("/compare?ids=");
          await expect(page.getByRole("status").filter({ hasText: "請從首頁" })).toBeVisible();
          await page.getByRole("link", { name: "回到首頁的目錄", exact: true }).click();
          const picks = page.locator(".search-results").getByRole("checkbox");
          await expect(picks).toHaveCount(3);
          for (const pick of await picks.all()) {
            await pick.focus();
            await pick.press("Space");
            await expect(pick).toBeChecked();
          }
          expect(reads).toEqual([0, 0, 0]);
          const comparison = page.getByRole("link", { name: "並排比較這 3 個小工具", exact: true });
          await expect(comparison).toBeVisible();
          await setOffline(page, true);
          await comparison.focus();
          await comparison.press("Enter");
        }
        await expect(page.locator("#skill-comparison-heading")).toBeVisible();
        const waiting = page
          .getByRole("status")
          .filter({ hasText: "等待連線以讀取小工具比較資料" });
        await expect(waiting).toBeVisible();
        await expect(table).toHaveCount(0);
        await expect(refresh).toHaveAttribute("aria-disabled", "true");
        await refresh.focus();
        await refresh.press("Enter");
        expect(reads).toEqual(cached ? [1, 1, 1] : [0, 0, 0]);
        const audit = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(
          info.outputPath("skill-comparison-paused-a11y.json"),
          JSON.stringify(audit),
        );
        expect(audit.violations).toEqual([]);
        expect(audit.incomplete).toEqual([]);
        await page.screenshot({ path: info.outputPath("skill-comparison-paused.png") });
        await setOffline(page, false);
        await expect(table.getByRole("columnheader")).toHaveText(["比較項目", ...currentNames]);
        await expect(waiting).toHaveCount(0);
        await expect(refresh).toHaveAttribute("aria-disabled", "false");
        await expect(refresh).toBeFocused();
        expect(reads).toEqual(cached ? [2, 2, 2] : [1, 1, 1]);
        expect(new URL(page.url()).searchParams.get("ids")).toBe(ids.join(","));
        expect(writes).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({ path: info.outputPath("skill-comparison-restored.png") });
      } finally {
        await setOffline(page, false);
      }
    });
  }
}

for (const width of [375, 1280]) {
  test(`skill comparison cached paused retry with two readable columns waits for the third at ${width}px`, async ({
    page,
  }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
    const ids = [SKILL, SKILL_B, SKILL_C];
    const names = ["摘要工具", "拆分工具", "清理工具"];
    const reads = [0, 0, 0];
    const writes: string[] = [];
    page.on("request", (request) => {
      if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
    });
    for (const [index, id] of ids.entries()) {
      await page.route(`**/api/skills/${id}?view=embedded`, (route) => {
        reads[index]++;
        if (index === 1 && reads[index] === 2)
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        return route.fulfill({ json: skillDetail(id, names[index]) });
      });
    }
    const table = page.getByRole("table", { name: /並排比較/ });
    const refresh = page.getByRole("button", { name: /更新比較資料|等待連線/ });
    try {
      await page.goto(`/compare?ids=${ids.join(",")}`);
      await expect(table.getByRole("columnheader")).toHaveText(["比較項目", ...names]);
      await page.getByRole("button", { name: "更新比較資料", exact: true }).click();
      await expect(page.getByRole("alert")).toContainText("有 1 個小工具讀取失敗");
      await expect(table.getByRole("columnheader")).toHaveText(["比較項目", names[0], names[2]]);
      await setOffline(page, true);
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await retry.press("Enter");
      const waiting = page.getByRole("status").filter({ hasText: "等待連線以讀取小工具比較資料" });
      await expect(waiting).toBeVisible();
      await expect(table).toHaveCount(0);
      await expect(page.getByRole("alert")).toHaveCount(0);
      await expect(refresh).toHaveAttribute("aria-disabled", "true");
      await refresh.focus();
      await refresh.press("Enter");
      expect(reads).toEqual([2, 2, 2]);
      const audit = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(info.outputPath("skill-comparison-paused-a11y.json"), JSON.stringify(audit));
      expect(audit.violations).toEqual([]);
      expect(audit.incomplete).toEqual([]);
      await page.screenshot({ path: info.outputPath("skill-comparison-paused.png") });
      await setOffline(page, false);
      await expect(table.getByRole("columnheader")).toHaveText(["比較項目", ...names]);
      await expect(refresh).toBeFocused();
      expect(reads).toEqual([2, 3, 2]);
      expect(new URL(page.url()).searchParams.get("ids")).toBe(ids.join(","));
      expect(writes).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: info.outputPath("skill-comparison-restored.png") });
    } finally {
      await setOffline(page, false);
    }
  });
}
