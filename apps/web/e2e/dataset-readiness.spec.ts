import { setOffline } from "./offline";
import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { TEST_CASE, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import type { Dataset } from "../src/features/lab/lab.service";

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
    test(`cached dataset ${owner} in ${phase} waits for fresh allowance and preserves the chosen file`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let checking = false;
      let writes = 0;
      let uploaded = false;
      let release!: () => void;
      let started!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const readStarted = new Promise<void>((resolve) => {
        started = resolve;
      });
      await page.route("**/test-cases/limits", async (route) => {
        if (checking && owner === "rules" && phase === "pending") {
          started();
          await gate;
        }
        return route.fulfill({
          json: { ...limits, max_file_bytes: checking && owner === "rules" ? 500 : 10_000 },
        });
      });
      await page.route(`**/test-cases/${TEST_CASE}/datasets`, async (route) => {
        if (route.request().resourceType() === "document") return route.fallback();
        if (route.request().method() === "POST") {
          writes += 1;
          uploaded = true;
          return route.fulfill({
            status: 201,
            json: {
              dataset_id: "99999999-1111-4111-8111-111111111111",
              file_name: "small.csv",
              size_bytes: 400,
              content_type: "text/csv",
              content_hash: "a".repeat(64),
              expires_at: "2026-11-15T00:00:00Z",
            } satisfies Dataset,
          });
        }
        if (checking && owner === "usage" && phase === "pending") {
          started();
          await gate;
        }
        return route.fulfill({ json: datasetSnapshot(checking && owner === "usage", uploaded) });
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
        if (phase === "offline") await setOffline(page, true);
        await page.goBack();
        if (phase === "pending") await readStarted;
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
        await file.setInputFiles({
          name: "rows.csv",
          mimeType: "text/csv",
          buffer: Buffer.alloc(1000, "n"),
        });
        expect(writes).toBe(0);
        await upload.scrollIntoViewIfNeeded();
        await expect(why).toBeInViewport({ ratio: 1 });
        await page.screenshot({ path: test.info().outputPath("dataset-readiness.png") });
        expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
        if (phase === "offline") await setOffline(page, false);
        else release();
        await expect(upload).toBeEnabled();
        await expect(file).toHaveValue(/rows\.csv$/);
        await expect(why).toHaveCount(0);
        expect(writes).toBe(0);
        await upload.click();
        await expect(page.getByRole("main").getByRole("alert")).toHaveText(
          owner === "rules"
            ? "rows.csv 是 1000 B，超過單一檔案上限 500 B，沒有送出。"
            : "這個測試題還剩 500 B 可用，rows.csv 是 1000 B，沒有送出。請先刪掉一些檔案再上傳。",
        );
        expect(writes).toBe(0);
        await file.setInputFiles({
          name: "small.csv",
          mimeType: "text/csv",
          buffer: Buffer.alloc(400, "n"),
        });
        await upload.click();
        await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toContainText(
          "已上傳 small.csv（400 B）",
        );
        expect(writes).toBe(1);
        expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
      } finally {
        release();
        await setOffline(page, false);
      }
    });
  }
}
