import { expect, test } from "vitest";
import { renderedApp } from "../../../testing/app";
import { platformResponse, TEST_CASE, VERSION } from "../../../testing/fixtures/platform";

const app = renderedApp();

const stored = platformResponse(`/test-cases/${TEST_CASE}/datasets`).body;
const emptyList = { datasets: [], total_bytes: 0 };
const scenarios = ["pending", "offline"].flatMap((phase) =>
  [false, true].map((empty) => ({ phase, empty })),
);

for (const { phase, empty } of scenarios) {
  test(`cached detail datasets ${empty ? "empty" : "populated"} during ${phase} wait for current facts`, async () => {
    const page = app();
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes = () =>
      page.requests.filter(
        (request) => request.method !== "GET" && request.url.pathname.includes("/test-cases"),
      );
    page.route(`**/test-cases/${TEST_CASE}/datasets`, async () => {
      reads++;
      if (reads === 2) await gate;
      return { json: reads === 1 ? (empty ? emptyList : stored) : empty ? stored : emptyList };
    });
    const section = page.getByRole("region", { name: "測試資料", exact: true });
    try {
      await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
      if (empty)
        await expect(section.getByText("還沒有上傳任何檔案。", { exact: true })).toBeVisible();
      else await expect(section.locator(".file-tree > li")).toHaveCount(1);
      await page.locator('a[href="/policy"]').first().click();
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
      if (phase === "offline") await page.setOffline(true);
      await page.goBack();
      const name = page.getByLabel("名稱", { exact: true });
      await name.fill("kept while checking stored files");
      if (phase === "pending") await page.eventually(() => expect(reads).toBe(2));
      const waiting = section.getByRole("status").filter({
        hasText: phase === "offline" ? "等待連線以讀取檔案清單" : "載入檔案清單中",
      });
      await expect(waiting).toBeVisible();
      await expect(section.getByText("還沒有上傳任何檔案。", { exact: true })).toHaveCount(0);
      await expect(section.getByText(/目前 \d+ 個檔案/)).toHaveCount(0);
      await expect(section.locator(".file-tree > li")).toHaveCount(0);
      await expect(section.getByRole("button", { name: "刪除這個檔案", exact: true })).toHaveCount(
        0,
      );
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
      release();
      if (phase === "offline") await page.setOffline(false);
      await expect(waiting).toHaveCount(0);
      await expect(section.locator(".file-tree > li")).toHaveCount(empty ? 1 : 0);
      if (!empty)
        await expect(section.getByText("還沒有上傳任何檔案。", { exact: true })).toBeVisible();
      await expect(name).toHaveValue("kept while checking stored files");
      const refresh = section.getByRole("button", { name: "重新整理", exact: true });
      await refresh.focus();
      await refresh.press("Enter");
      await page.eventually(() => expect(reads).toBe(3));
      await expect(page.locator("#test-case-datasets-heading")).toBeFocused();
      await expect(name).toHaveValue("kept while checking stored files");
      expect(writes()).toEqual([]);
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
    } finally {
      release();
      if (phase === "offline") await page.setOffline(false);
    }
  });
}
