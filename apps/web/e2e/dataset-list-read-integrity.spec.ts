import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const ROW = {
  dataset_id: "11111111-1111-4111-8111-111111111111",
  file_name: "existing.csv",
  content_type: "text/csv",
  content_hash: "a".repeat(64),
  size_bytes: 1000,
  expires_at: "2026-11-15T00:00:00Z",
};
const MALFORMED: [string, unknown][] = [
  ["string files", { datasets: "", total_bytes: 0 }],
  ["missing file expiry", { datasets: [{ ...ROW, expires_at: undefined }], total_bytes: 1000 }],
  ["inconsistent total", { datasets: [ROW], total_bytes: 0 }],
];

for (const width of [375, 1280]) {
  for (const [owner, phase] of [
    ["detail", "initial"],
    ["detail", "cached"],
    ["upload", "initial"],
    ["upload", "cached"],
  ]) {
    for (const [name, malformed] of MALFORMED) {
      test(`dataset ${owner} with ${name} on ${phase} withholds unsupported decisions and recovers at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        let current: unknown =
          phase === "initial" ? malformed : { datasets: [ROW], total_bytes: 1000 };
        let reads = 0;
        const writes: string[] = [];
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        await page.route(`**/test-cases/${TEST_CASE}/datasets`, (route) => {
          if (route.request().isNavigationRequest()) return route.fallback();
          reads++;
          return route.fulfill({
            status: 200,
            body: JSON.stringify(current),
            contentType: "application/json",
          });
        });
        const path = `/lab/test-cases/${TEST_CASE}${owner === "upload" ? "/datasets" : ""}?version=${VERSION}`;
        await page.goto(path);
        const upload = page.getByRole("button", { name: "上傳", exact: true });
        if (phase === "cached") {
          if (owner === "upload") await expect(upload).toBeEnabled();
          else await expect(page.getByText("existing.csv", { exact: false })).toBeVisible();
          await page.locator('a[href="/policy"]').first().click();
          await expect(
            page.getByRole("heading", { name: "資料保存政策", exact: true }),
          ).toBeVisible();
          current = malformed;
          await page.goBack();
        }
        await expect.poll(() => reads).toBe(phase === "initial" ? 1 : 2);
        const failure = page.getByRole("alert").filter({
          hasText: owner === "upload" ? "暫時無法讀取測試資料已用容量" : "暫時無法讀取檔案清單",
        });
        await expect(failure).toBeVisible();
        await expect(page.getByText("還沒有上傳任何檔案。", { exact: true })).toHaveCount(0);
        await expect(page.getByText("existing.csv", { exact: false })).toHaveCount(0);
        await expect(page.getByRole("button", { name: "刪除這個檔案", exact: true })).toHaveCount(
          0,
        );
        if (owner === "upload") {
          await expect(upload).toBeDisabled();
          await expect(page.locator("#upload-read-why")).toContainText("目前無法確認");
          await expect(page.getByText("還可以再上傳", { exact: false })).toHaveCount(0);
          await page.getByLabel("選擇檔案", { exact: true }).setInputFiles({
            name: "chosen.csv",
            mimeType: "text/csv",
            buffer: Buffer.alloc(1000, "n"),
          });
        }
        const report = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(
          info.outputPath("dataset-list-unreadable-a11y.json"),
          JSON.stringify(report),
        );
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        await failure.scrollIntoViewIfNeeded();
        await page.screenshot({ path: info.outputPath("dataset-list-unreadable.png") });
        current = { datasets: [], total_bytes: 0 };
        const retry = page.getByRole("button", { name: "再試一次", exact: true });
        await retry.focus();
        await expect(retry).toBeFocused();
        await retry.press("Enter");
        await expect(failure).toHaveCount(0);
        if (owner === "upload") {
          await expect(upload).toBeEnabled();
          await expect(page.getByLabel("選擇檔案", { exact: true })).toHaveValue(/chosen\.csv$/);
          await expect(page.locator("#dataset-heading")).toBeFocused();
        } else {
          await expect(page.getByText("還沒有上傳任何檔案。", { exact: true })).toBeVisible();
          await expect(page.locator("#test-case-datasets-heading")).toBeFocused();
        }
        expect(reads).toBe(phase === "initial" ? 2 : 3);
        expect(writes).toEqual([]);
        expect(errors).toEqual([]);
        expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({ path: info.outputPath("dataset-list-recovered.png") });
      });
    }
  }
}
