import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, RUNS } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

for (const retrying of [false, true]) {
  for (const moved of [false, true]) {
    test(`comparison candidate ${retrying ? "retry" : "next page"} explains paused reads and ${moved ? "preserves moved focus" : "continues keyboard navigation"}`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const rows = Array.from({ length: 51 }, (_, index) => ({
        ...RUNS.runs[1],
        run_id: `44444444-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
      }));
      const offsets: string[] = [];
      const writes: string[] = [];
      let release!: () => void;
      const resumedRead = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      await page.route("**/runs?*", async (route) => {
        const offset = new URL(route.request().url()).searchParams.get("offset") ?? "";
        offsets.push(offset);
        if (retrying && offset === "50")
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        if (retrying ? offsets.length === 3 : offset === "50") await resumedRead;
        return route.fulfill({ json: { runs: offset === "0" ? rows : [rows[50]] } });
      });
      await page.goto(`/runs/${RUN}/compare`);
      const main = page.getByRole("main");
      const more = main.getByRole("button", { name: /^(載入更早的試跑紀錄|載入中…|等待連線…)$/ });
      await expect(more).toHaveText("載入更早的試跑紀錄");
      const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
      await field.fill("尚未送出的比較 ID");
      if (retrying) {
        await more.focus();
        await page.keyboard.press("Enter");
        await expect(main.getByRole("alert")).toContainText("暫時無法讀取可比較的試跑紀錄");
      }
      const retry = main.getByRole("button", { name: "再試一次", exact: true });
      const destination = page
        .getByRole("navigation", { name: "主要導覽" })
        .getByRole("link", { name: "工作台", exact: true });
      try {
        await setOffline(page, true);
        await (retrying ? retry : more).focus();
        await page.keyboard.press("Enter");
        await expect(
          main
            .getByRole("status")
            .filter({ hasText: "等待連線以讀取可比較的試跑紀錄；恢復連線後會繼續讀取。" }),
        ).toBeVisible();
        await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(retry).toHaveCount(0);
        expect(offsets).toEqual(retrying ? ["0", "50"] : ["0"]);
        if (retrying) await expect(page.locator("#run-comparison-heading")).toBeFocused();
        else {
          await expect(more).toHaveText("等待連線…");
          await expect(more).toBeDisabled();
          await expect(main.locator(".download-list > li")).toHaveCount(50);
        }
        if (moved) await destination.focus();
        const pausedAccessibility = await new AxeBuilder({ page }).analyze();
        await writeFile(
          testInfo.outputPath("candidate-paused-accessibility.json"),
          JSON.stringify(pausedAccessibility),
        );
        expect(pausedAccessibility.violations).toEqual([]);
        expect(pausedAccessibility.incomplete).toEqual([]);
        await page.screenshot({ path: testInfo.outputPath("candidate-paused.png") });
        await setOffline(page, false);
        await expect.poll(() => offsets).toEqual(retrying ? ["0", "50", "0"] : ["0", "50"]);
        await expect(main.getByRole("status").filter({ hasText: "等待連線以讀取" })).toHaveCount(0);
        await expect(main.getByRole("alert")).toHaveCount(0);
        if (retrying)
          await expect(
            main.getByRole("status").filter({ hasText: "載入可比較的試跑紀錄中…" }),
          ).toBeVisible();
        else {
          await expect(more).toHaveText("載入中…");
          await expect(more).toBeDisabled();
        }
        release();
        if (retrying) {
          await expect(more).toHaveText("載入更早的試跑紀錄");
          await expect(more).toBeEnabled();
          await expect(main.locator(".download-list > li")).toHaveCount(50);
        } else {
          await expect(more).toHaveCount(0);
          await expect(main.locator(".download-list > li")).toHaveCount(51);
        }
        if (moved) await expect(destination).toBeFocused();
        else if (retrying) await expect(page.locator("#run-comparison-heading")).toBeFocused();
        else
          await expect(
            main.getByRole("button", { name: new RegExp(`試跑紀錄 ID ${rows[50].run_id}$`) }),
          ).toBeFocused();
        await expect(field).toHaveValue("尚未送出的比較 ID");
        expect(offsets).toEqual(retrying ? ["0", "50", "0"] : ["0", "50"]);
        expect(writes).toEqual([]);
        const recoveredAccessibility = await new AxeBuilder({ page }).analyze();
        await writeFile(
          testInfo.outputPath("candidate-recovered-accessibility.json"),
          JSON.stringify(recoveredAccessibility),
        );
        expect(recoveredAccessibility.violations).toEqual([]);
        expect(recoveredAccessibility.incomplete).toEqual([]);
        await page.screenshot({ path: testInfo.outputPath("candidate-recovered.png") });
      } finally {
        release();
        await setOffline(page, false);
      }
    });
  }
}
