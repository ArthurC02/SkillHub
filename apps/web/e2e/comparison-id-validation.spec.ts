import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { OTHER_RUN, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const existing of [false, true]) {
  for (const method of ["keyboard", "pointer"] as const) {
    test(`comparison ID validation with ${method} ${existing ? "preserves existing results" : "explains an empty submission"}`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: method === "keyboard" ? 375 : 1280, height: 900 });
      await page.emulateMedia({ colorScheme: method === "keyboard" ? "light" : "dark" });
      const reads: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
        if (new URL(request.url()).pathname.endsWith("/comparison")) reads.push(request.url());
      });
      await page.goto(`/runs/${RUN}/compare${existing ? `?against=${OTHER_RUN}` : ""}`);
      const main = page.getByRole("main");
      const table = main.getByRole("table", {
        name: "試跑紀錄任務判定與執行狀態對比",
        exact: true,
      });
      if (existing) {
        await expect(table).toBeVisible();
        await main.getByText("換一個要比較的試跑紀錄", { exact: true }).click();
      }
      const originalURL = page.url();
      const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
      const submit = async () => {
        if (method === "pointer")
          await main.getByRole("button", { name: "比較", exact: true }).click();
        else {
          await field.focus();
          await page.keyboard.press("Enter");
        }
      };
      await field.fill("");
      await submit();
      await expect(field).toBeFocused();
      expect(
        await field.evaluate((element: HTMLInputElement) => element.validity.valueMissing),
      ).toBe(true);
      expect(
        await field.evaluate((element: HTMLInputElement) => element.validationMessage),
      ).not.toBe("");
      await expect(page).toHaveURL(originalURL);
      await expect(table).toHaveCount(existing ? 1 : 0);
      expect(reads).toHaveLength(existing ? 1 : 0);
      const accessibility = await new AxeBuilder({ page }).include("main form").analyze();
      await writeFile(
        testInfo.outputPath("comparison-id-form-accessibility.json"),
        JSON.stringify(accessibility),
      );
      expect(accessibility.violations).toEqual([]);
      expect(accessibility.incomplete).toEqual([]);
      expect(accessibility.passes.length).toBeGreaterThan(5);
      await page.screenshot({ path: testInfo.outputPath("comparison-id-required.png") });
      await field.fill(OTHER_RUN);
      await submit();
      await expect(page).toHaveURL(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
      await expect(table).toBeVisible();
      await expect(page.locator("#run-comparison-heading")).toBeFocused();
      expect(reads).toHaveLength(1);
      expect(new URL(reads[0]).searchParams.get("against")).toBe(OTHER_RUN);
      expect(writes).toEqual([]);
    });
  }
}
