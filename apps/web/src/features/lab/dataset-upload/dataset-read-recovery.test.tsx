import { expect, test } from "vitest";
import { renderedApp } from "../../../testing/app";
import { TEST_CASE, VERSION } from "../../../testing/fixtures/platform";
import { chooseCsv } from "../../../testing/files";

const app = renderedApp();

const LIMITS = {
  max_file_bytes: 1_000_000,
  max_test_case_bytes: 10_000,
  max_files_per_test_case: 3,
  retention_days: 7,
  allowed_kinds: ["csv"],
  note: "",
};

for (const owner of ["rules", "usage"]) {
  for (const cached of [false, true]) {
    test(`dataset ${owner} read failure ${cached ? "after a cached read" : "on arrival"} blocks sending and recovers without clearing the chosen file`, async () => {
      const page = app();
      let reads = 0;
      let writes = 0;
      page.route("**/test-cases/limits", () => {
        if (owner === "rules") reads += 1;
        return owner === "rules" && reads === (cached ? 2 : 1)
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: LIMITS };
      });
      page.route(`**/test-cases/${TEST_CASE}/datasets`, (request) => {
        if (request.method === "POST") {
          writes += 1;
          return {
            status: 201,
            json: {
              dataset_id: "11111111-1111-4111-8111-111111111111",
              file_name: "rows.csv",
              size_bytes: 1000,
              content_type: "text/csv",
              content_hash: "a".repeat(64),
              expires_at: "2026-11-15T00:00:00Z",
            },
          };
        }
        if (owner === "usage") reads += 1;
        return owner === "usage" && reads === (cached ? 2 : 1)
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: { datasets: [], total_bytes: 0 } };
      });
      await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
      if (cached) {
        await expect(page.getByRole("button", { name: "上傳", exact: true })).toBeEnabled();
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        await page.goBack();
      }
      await expect(page.getByRole("alert")).toContainText("暫時無法讀取");
      const upload = page.getByRole("button", { name: "上傳", exact: true });
      await expect(upload).toBeVisible();
      await expect(upload).toBeDisabled();
      await expect(page.locator("#upload-read-why")).toContainText("目前無法確認");
      if (owner === "usage") {
        await expect(
          page.getByText("已用容量讀取失敗，剩餘容量目前無法確認。", { exact: true }),
        ).toBeVisible();
        await expect(page.getByText("正在讀這個測試題已經用掉多少…", { exact: true })).toHaveCount(
          0,
        );
        await expect(page.getByText("還可以再上傳", { exact: false })).toHaveCount(0);
      } else {
        await expect(page.locator(".dataset-upload-guide")).toHaveCount(0);
      }
      const file = page.getByLabel("選擇檔案", { exact: true });
      chooseCsv(file, "rows.csv", "n".repeat(1000));
      expect(writes).toBe(0);
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      await expect(upload).toBeEnabled();
      await expect(file).toHaveValue(/rows\.csv$/);
      await expect(page.locator("#dataset-heading")).toBeFocused();
      expect(reads).toBe(cached ? 3 : 2);
      await upload.click();
      await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toContainText(
        "已上傳 rows.csv",
      );
      expect(writes).toBe(1);
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}

test("dataset retries its unreadable test case in place and preserves the selected version", async () => {
  const page = app();
  let reads = 0;
  page.route(`**/test-cases/${TEST_CASE}`, () => {
    reads += 1;
    return reads === 1 ? { status: 503, json: { error: "service unavailable" } } : undefined;
  });
  await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
  await expect(page.getByRole("alert")).toContainText("暫時無法讀取測試題");
  await expect(page.getByRole("heading", { name: "測試資料", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "再試一次", exact: true }).focus();
  await page.press("Enter");
  await expect(page.getByLabel("選擇檔案", { exact: true })).toBeVisible();
  await expect(page.locator("#dataset-heading")).toBeFocused();
  expect(reads).toBe(2);
  expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
});
