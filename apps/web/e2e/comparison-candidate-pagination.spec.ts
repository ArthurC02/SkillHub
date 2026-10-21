import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, RUNS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const ending of ["appended", "filtered", "more", "failure"] as const) {
  for (const moved of [false, true]) {
    test(`comparison candidates ${ending} page ${moved ? "preserves moved focus" : "continues keyboard navigation"}`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const rows = Array.from({ length: 101 }, (_, index) => ({
        ...RUNS.runs[1],
        run_id: `44444444-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
      }));
      const offsets: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route("**/runs?*", async (route) => {
        const url = new URL(route.request().url());
        const offset = url.searchParams.get("offset") ?? "";
        offsets.push(offset);
        expect(url.searchParams.get("limit")).toBe("51");
        expect(url.searchParams.get("test_case_id")).toBe(RUNS.runs[0].test_case_id);
        if (offset === "0") return route.fulfill({ json: { runs: rows.slice(0, 51) } });
        await held;
        if (ending === "failure")
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        return route.fulfill({
          json: {
            runs:
              ending === "more"
                ? rows.slice(50)
                : ending === "filtered"
                  ? [RUNS.runs[0]]
                  : [rows[50]],
          },
        });
      });
      await page.goto(`/runs/${RUN}/compare`);
      const main = page.getByRole("main");
      const more = main.getByRole("button", { name: /^(載入更早的試跑紀錄|載入中…)$/ });
      await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
      const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
      await expect(more).toHaveText("載入更早的試跑紀錄");
      await field.fill("尚未送出的比較 ID");
      await more.focus();
      await page.keyboard.press("Enter");
      const destination = page
        .getByRole("navigation", { name: "主要導覽" })
        .getByRole("link", { name: "工作台", exact: true });
      try {
        await expect.poll(() => offsets).toEqual(["0", "50"]);
        await expect(more).toBeDisabled();
        if (moved) await destination.focus();
      } finally {
        release();
      }
      if (ending === "failure")
        await expect(main.getByRole("alert")).toContainText("暫時無法讀取可比較的試跑紀錄");
      else if (ending === "more") {
        await expect(more).toHaveText("載入更早的試跑紀錄");
        await expect(more).toBeEnabled();
        await expect(main.locator(".download-list > li")).toHaveCount(100);
      } else {
        await expect(more).toHaveCount(0);
        await expect(main.locator(".download-list > li")).toHaveCount(
          ending === "filtered" ? 50 : 51,
        );
      }
      if (moved) await expect(destination).toBeFocused();
      else if (ending === "failure")
        await expect(page.locator("#run-comparison-heading")).toBeFocused();
      else if (ending === "more") await expect(more).toBeFocused();
      else
        await expect(
          main.getByRole("button", {
            name: new RegExp(`試跑紀錄 ID ${rows[ending === "filtered" ? 49 : 50].run_id}$`),
          }),
        ).toBeFocused();
      await expect(field).toHaveValue("尚未送出的比較 ID");
      expect(offsets).toEqual(["0", "50"]);
      expect(writes).toEqual([]);
      const accessibility = await new AxeBuilder({ page }).analyze();
      await writeFile(
        testInfo.outputPath("candidate-pagination-accessibility.json"),
        JSON.stringify(accessibility),
      );
      expect(accessibility.violations).toEqual([]);
      expect(accessibility.incomplete).toEqual([]);
      await page.screenshot({ path: testInfo.outputPath("candidate-pagination-focus.png") });
    });
  }
}
