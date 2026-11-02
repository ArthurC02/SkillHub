import { expect, test } from "vitest";
import { renderedApp } from "../../../testing/app";
import { TEST_CASE, TEST_CASE_DRAFT } from "../../../testing/fixtures/platform";

const app = renderedApp();

const scenarios = [
  { mode: "pristine-reread", expected: "伺服器已更新的驗收條件", dirty: false, enabled: false },
  { mode: "canonical-save", expected: "已儲存的內容", dirty: false, enabled: false },
  { mode: "dirty-reread", expected: "未送出的自己的草稿", dirty: true, enabled: true },
  { mode: "pending-typing", expected: "正在編輯的下一稿", dirty: true, enabled: true },
  { mode: "pending-revert", expected: "輸出的列數少於輸入", dirty: true, enabled: true },
  { mode: "empty-reread", expected: "", dirty: true, enabled: false },
  { mode: "unchanged-reread", expected: "輸出的列數少於輸入", dirty: false, enabled: false },
  { mode: "failed-save", expected: "  已儲存的內容  ", dirty: true, enabled: true },
];

for (const scenario of scenarios) {
  test(`${scenario.mode} preserves criterion draft ownership`, async () => {
    const page = app();
    const current = structuredClone(TEST_CASE_DRAFT);
    const writes: { method: string; criterion: string; body: unknown }[] = [];
    let reads = 0;
    let release!: () => void;
    let started!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const received = new Promise<void>((resolve) => {
      started = resolve;
    });
    page.route(`**/test-cases/${TEST_CASE}`, () => {
      reads += 1;
      return { json: current };
    });
    page.route(`**/test-cases/${TEST_CASE}/criteria/*`, async (request) => {
      const criterion = request.url.pathname.split("/").at(-1)!;
      const body = request.body;
      writes.push({ method: request.method, criterion, body });
      expect(request.method).toBe("PATCH");
      if (criterion === "c1") {
        expect(body).toEqual({ text: "  已儲存的內容  " });
        if (scenario.mode === "failed-save") {
          return { status: 503, json: { error: "暫時無法更新驗收條件" } };
        }
        if (scenario.mode.startsWith("pending-")) {
          started();
          await held;
        }
        current.acceptance_criteria[0].text = "已儲存的內容";
        current.acceptance_criteria[0].confirmed_at = null;
      } else {
        expect(criterion).toBe("c2");
        expect(body).toEqual({ confirmed: true });
        current.acceptance_criteria[1].confirmed_at = "2026-08-18T01:00:00Z";
        if (scenario.mode !== "unchanged-reread") {
          current.acceptance_criteria[0].text = "伺服器已更新的驗收條件";
          current.acceptance_criteria[0].confirmed_at = null;
        }
      }
      current.updated_at = "2026-08-18T01:00:00Z";
      return { json: current };
    });
    try {
      await page.goto(`/lab/test-cases/${TEST_CASE}`);
      const field = page.locator("#criterion-c1");
      const first = page.locator("li.criterion").filter({ has: field });
      const second = page.locator("li.criterion").filter({ has: page.locator("#criterion-c2") });
      const save = first.getByRole("button", { name: "儲存文字", exact: true });
      await expect(field).toHaveValue("輸出的列數少於輸入");
      await expect(save).toBeDisabled();
      await expect(second).toContainText("尚未確認");
      const saving =
        scenario.mode === "canonical-save" ||
        scenario.mode.startsWith("pending-") ||
        scenario.mode === "failed-save";
      if (saving) {
        await field.fill("  已儲存的內容  ");
        await save.click();
        if (scenario.mode.startsWith("pending-")) {
          await received;
          await field.fill(scenario.expected);
          release();
        }
        if (scenario.mode === "failed-save") {
          await expect(first.getByRole("alert")).toHaveText(
            "操作結果尚未確認；請先確認最新資料，再決定是否重送。",
          );
        } else {
          await expect(first).toContainText("狀態：尚未確認");
        }
      } else {
        if (scenario.dirty) await field.fill(scenario.expected);
        await second.getByRole("button", { name: "確認", exact: true }).click();
        await expect(second).toContainText("狀態：已確認");
      }
      await expect(save).toBeVisible();
      await expect(field).toHaveValue(scenario.expected);
      if (scenario.enabled) await expect(save).toBeEnabled();
      else await expect(save).toBeDisabled();
      await expect(first.locator("#criterion-edited-c1")).toHaveCount(scenario.dirty ? 1 : 0);
      if (scenario.mode === "empty-reread") {
        await expect(save).toHaveAccessibleDescription("驗收條件不能是空白");
      }
      await page.eventually(() => expect(reads).toBe(2));
      expect(writes).toEqual([
        saving
          ? { method: "PATCH", criterion: "c1", body: { text: "  已儲存的內容  " } }
          : { method: "PATCH", criterion: "c2", body: { confirmed: true } },
      ]);
      await field.click();
      await expect(field).toBeFocused();
      expect(await page.axeViolations("li.criterion")).toEqual([]);
    } finally {
      release();
    }
  });
}
