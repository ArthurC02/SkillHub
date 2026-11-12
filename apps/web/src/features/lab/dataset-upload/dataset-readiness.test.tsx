import { expect, test } from "vitest";
import { renderedApp } from "../../../testing/app";
import { TEST_CASE, VERSION } from "../../../testing/fixtures/platform";
import type { Dataset } from "../lab.service";
import { chooseCsv } from "../../../testing/files";

const app = renderedApp();

const limits = {
  max_file_bytes: 10_000,
  max_test_case_bytes: 20_000,
  max_files_per_test_case: 5,
  retention_days: 7,
  allowed_kinds: ["csv"],
  note: "",
};

function datasetSnapshot(extra: boolean, uploaded: boolean) {
  const sizes = [9500, 9500, ...(extra ? [500] : []), ...(uploaded ? [400] : [])];
  return {
    datasets: sizes.map((size_bytes, index): Dataset => ({
      dataset_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
      file_name: `existing-${index}.csv`,
      size_bytes,
      content_type: "text/csv",
      content_hash: "a".repeat(64),
      expires_at: "2026-11-15T00:00:00Z",
    })),
    total_bytes: sizes.reduce((total, size) => total + size, 0),
  };
}

for (const owner of ["rules", "usage"]) {
  for (const phase of ["pending", "offline"]) {
    test(`cached dataset ${owner} in ${phase} waits for fresh allowance and preserves the chosen file`, async () => {
      const page = app();
      let checking = false;
      let writes = 0;
      let uploaded = false;
      let started = false;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route("**/test-cases/limits", async () => {
        if (checking && owner === "rules" && phase === "pending") {
          started = true;
          await gate;
        }
        return {
          json: { ...limits, max_file_bytes: checking && owner === "rules" ? 500 : 10_000 },
        };
      });
      page.route(`**/test-cases/${TEST_CASE}/datasets`, async (request) => {
        if (request.method === "POST") {
          writes += 1;
          uploaded = true;
          return {
            status: 201,
            json: {
              dataset_id: "99999999-1111-4111-8111-111111111111",
              file_name: "small.csv",
              size_bytes: 400,
              content_type: "text/csv",
              content_hash: "a".repeat(64),
              expires_at: "2026-11-15T00:00:00Z",
            } satisfies Dataset,
          };
        }
        if (checking && owner === "usage" && phase === "pending") {
          started = true;
          await gate;
        }
        return { json: datasetSnapshot(checking && owner === "usage", uploaded) };
      });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
        const upload = page.getByRole("button", { name: "上傳", exact: true });
        await expect(upload).toBeEnabled();
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        checking = true;
        if (phase === "offline") await page.setOffline(true);
        await page.goBack();
        if (phase === "pending") await page.eventually(() => expect(started).toBe(true));
        const why = page.locator("#upload-read-why");
        await expect(why).toHaveText(
          phase === "offline"
            ? "等待連線以確認上傳規則與已用容量；確認後才能上傳。"
            : "正在確認上傳規則與已用容量；確認後才能上傳。",
        );
        await expect(upload).toBeDisabled();
        if (owner === "usage") {
          await expect(page.getByText("還可以再上傳", { exact: false })).toHaveCount(0);
        } else await expect(page.locator(".dataset-upload-guide")).toHaveCount(0);
        const file = page.getByLabel("選擇檔案", { exact: true });
        chooseCsv(file, "rows.csv", "n".repeat(1000));
        expect(writes).toBe(0);
        expect(await page.axeViolations("body")).toEqual([]);
        if (phase === "offline") await page.setOffline(false);
        else release();
        await expect(upload).toBeEnabled();
        await expect(file).toHaveValue(/rows\.csv$/);
        await expect(why).toHaveCount(0);
        expect(writes).toBe(0);
        await upload.click();
        await expect(page.getByRole("alert")).toHaveText(
          owner === "rules"
            ? "rows.csv 是 1000 B，超過單一檔案上限 500 B，沒有送出。"
            : "這個測試題還剩 500 B 可用，rows.csv 是 1000 B，沒有送出。請先刪掉一些檔案再上傳。",
        );
        expect(writes).toBe(0);
        chooseCsv(file, "small.csv", "n".repeat(400));
        await upload.click();
        await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toContainText(
          "已上傳 small.csv（400 B）",
        );
        expect(writes).toBe(1);
        expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
      } finally {
        release();
        await page.setOffline(false);
      }
    });
  }
}
