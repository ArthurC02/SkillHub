import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { COMPARISON, OTHER_RUN, RUN } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

for (const intent of ["refresh", "retry", "candidate", "manual"] as const) {
  for (const moved of [false, true]) {
    test(`comparison ${intent} explains paused reads and ${moved ? "preserves moved focus" : "retains decision context"}`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const reads: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      await page.route(`**/runs/${RUN}/comparison?*`, (route) => {
        reads.push(new URL(route.request().url()).searchParams.get("against") ?? "");
        if (intent === "retry" && reads.length === 1)
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        return route.fulfill({
          json: {
            ...COMPARISON,
            runs: COMPARISON.runs.map((side) => ({
              ...side,
              final_output:
                intent === "refresh" && reads.length === 1
                  ? "更新前的比較輸出"
                  : "恢復連線後的比較輸出",
            })),
          },
        });
      });
      const picking = intent === "candidate" || intent === "manual";
      await page.goto(`/runs/${RUN}/compare${picking ? "" : `?against=${OTHER_RUN}`}`);
      const main = page.getByRole("main");
      const table = main.getByRole("table", {
        name: "試跑紀錄任務判定與執行狀態對比",
        exact: true,
      });
      const refresh = main.getByRole("button", { name: /^(更新比較結果|更新比較中…|等待連線…)$/ });
      const candidate = main.getByRole("button", { name: new RegExp(`試跑紀錄 ID ${OTHER_RUN}$`) });
      if (picking) await expect(candidate).toBeVisible();
      if (intent === "refresh")
        await expect(table.getByText("更新前的比較輸出", { exact: true })).toHaveCount(2);
      if (intent === "manual") {
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        await page.getByLabel("要比較的另一個試跑紀錄 ID").fill(OTHER_RUN);
      }
      const action =
        intent === "retry"
          ? main.getByRole("button", { name: "再試一次", exact: true })
          : intent === "candidate"
            ? candidate
            : intent === "manual"
              ? main.getByRole("button", { name: "比較", exact: true })
              : refresh;
      await expect(action).toBeVisible();
      try {
        await setOffline(page, true);
        await action.focus();
        await page.keyboard.press("Enter");
        await expect(
          main.getByRole("status").filter({ hasText: "等待連線以讀取比較" }),
        ).toBeVisible();
        await expect(refresh).toHaveText("等待連線…");
        await expect(refresh).toHaveAttribute("aria-disabled", "true");
        await expect(table).toHaveCount(0);
        await expect(main.locator(".verdict")).toHaveCount(0);
        expect(reads).toEqual(picking ? [] : [OTHER_RUN]);
        if (picking || intent === "retry")
          await expect(page.locator("#run-comparison-heading")).toBeFocused();
        else await expect(refresh).toBeFocused();
        const destination = page
          .getByRole("navigation", { name: "主要導覽" })
          .getByRole("link", { name: "工作台", exact: true });
        if (moved) await destination.focus();
        else if (intent === "refresh") await page.keyboard.press("Enter");
        const accessibility = await new AxeBuilder({ page }).analyze();
        await writeFile(
          testInfo.outputPath("paused-accessibility.json"),
          JSON.stringify(accessibility),
        );
        expect(accessibility.violations).toEqual([]);
        expect(accessibility.incomplete).toEqual([]);
        await page.screenshot({
          path: testInfo.outputPath("comparison-paused.png"),
          fullPage: true,
        });
        await setOffline(page, false);
        await expect(table.getByText("恢復連線後的比較輸出", { exact: true })).toHaveCount(2);
        await expect(refresh).toHaveText("更新比較結果");
        await expect(refresh).toHaveAttribute("aria-disabled", "false");
        if (moved) await expect(destination).toBeFocused();
        else if (intent === "refresh") await expect(refresh).toBeFocused();
        else await expect(page.locator("#run-comparison-heading")).toBeFocused();
        expect(reads).toEqual(picking ? [OTHER_RUN] : [OTHER_RUN, OTHER_RUN]);
        expect(writes).toEqual([]);
        await expect(page).toHaveURL(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
      } finally {
        await setOffline(page, false);
      }
    });
  }
}
