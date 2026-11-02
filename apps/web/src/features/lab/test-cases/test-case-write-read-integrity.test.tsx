import { expect, test } from "vitest";
import { TEST_CASE, TEST_CASE_DRAFT, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

for (const operation of ["prompt", "criterion"]) {
  test(`an unreadable ${operation} write response rereads without replay and preserves the draft`, async () => {
    const page = app();
    let written = false;
    let failing = true;
    let reads = 0;
    const writes: string[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const stored = {
      ...TEST_CASE_DRAFT,
      name: "server name after recovery",
      acceptance_criteria: TEST_CASE_DRAFT.acceptance_criteria.map((criterion) => ({
        ...criterion,
        text: "server criterion after recovery",
      })),
    };
    page.route(`**/test-cases/${TEST_CASE}`, async ({ method, url }) => {
      if (method === "PATCH") {
        written = true;
        writes.push(url.href);
        return { status: 500, json: { error: "讀取失敗" } };
      }
      reads++;
      if (!written) return { json: TEST_CASE_DRAFT };
      if (!failing) return { json: stored };
      await gate;
      return { status: 500, json: { error: "讀取失敗" } };
    });
    page.route(`**/test-cases/${TEST_CASE}/criteria/c1`, ({ url }) => {
      written = true;
      writes.push(url.href);
      return { status: 500, json: { error: "更新驗收條件失敗" } };
    });
    try {
      await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
      const field =
        operation === "prompt"
          ? page.getByLabel("名稱", { exact: true })
          : page.locator("#criterion-c1");
      const draft = operation === "prompt" ? "kept name draft" : "kept criterion draft";
      await field.fill(draft);
      const submit =
        operation === "prompt"
          ? page.getByRole("button", { name: "儲存", exact: true })
          : page.getByRole("button", { name: "儲存文字", exact: true }).first();
      await submit.click();
      const unknown = page
        .getByRole("alert")
        .filter({ hasText: "操作結果尚未確認；請先確認最新資料，再決定是否重送。" });
      await page.eventually(() => {
        expect(writes).toHaveLength(1);
        expect(unknown.count()).toBe(1);
        expect(reads).toBe(2);
        expect(field.value()).toBe(draft);
        expect(submit.isDisabled()).toBe(false);
      });
      expect(await page.axeViolations("main")).toEqual([]);
      release();
      await page.eventually(() => {
        expect(page.getByRole("alert").text()).toContain("目前無法確認最新內容，輸入已保留");
        expect(page.locator(".test-case-detail-page").count()).toBe(0);
        expect(document.querySelector(".test-case-detail-page")?.hasAttribute("inert")).toBe(true);
      });
      failing = false;
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await retry.press("Enter");
      await page.eventually(() => {
        expect(reads).toBe(3);
        expect(field.value()).toBe(draft);
        expect(unknown.count()).toBe(1);
        expect(page.getByText("已儲存。", { exact: true }).count()).toBe(0);
        expect(page.locator("#main-content").isFocused()).toBe(true);
      });
      expect(writes).toHaveLength(1);
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
    } finally {
      release();
    }
  });
}
