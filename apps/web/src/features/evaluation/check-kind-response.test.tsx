import { expect, test, vi } from "vitest";
import { CHECK_KINDS, TEST_CASE, TEST_CASE_DRAFT, VERSION } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

vi.mock("./components/CheckFlow", () => ({ CheckFlow: () => null }));

const app = renderedApp();

for (const [name, invalid] of [
  ["missing kinds", {}],
  ["null kind", { ...CHECK_KINDS, kinds: [null] }],
  ["missing limits", { ...CHECK_KINDS, max_nodes: undefined, max_depth: undefined }],
  [
    "unreadable value bounds",
    { ...CHECK_KINDS, kinds: [{ ...CHECK_KINDS.kinds[0], values: null }] },
  ],
] as const) {
  test(`check kinds with ${name} preserve the optional criterion and recover`, async () => {
    const page = app();
    let reads = 0;
    let release!: () => void;
    const firstRead = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes = () => page.requests.filter((request) => request.method !== "GET");
    page.route("**/evaluation/check-kinds", async () => {
      reads++;
      if (reads === 1) await firstRead;
      return { json: reads === 1 ? invalid : CHECK_KINDS };
    });
    try {
      await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
      const draft = page.getByLabel("新增驗收條件", { exact: true });
      const add = page.getByRole("button", { name: "新增", exact: true });
      await draft.fill("仍可保留的文字驗收條件");
      await expect(add).toBeEnabled();
      release();
      await expect(page.getByRole("status").filter({ hasText: "載入檢查方式中" })).toHaveCount(0);
      const palette = page.getByRole("group", { name: "檢查方式與設計", exact: true });
      await expect(palette.getByRole("alert")).toContainText("暫時無法讀取檢查方式");
      await expect(page.locator("h1").filter({ hasText: TEST_CASE_DRAFT.name })).toBeVisible();
      await expect(draft).toHaveValue("仍可保留的文字驗收條件");
      await expect(add).toBeEnabled();
      await expect(page.getByLabel("從哪一種節點開始", { exact: true })).toHaveCount(0);
      await expect(page.getByRole("button", { name: "重新載入頁面", exact: true })).toHaveCount(0);
      expect(reads).toBe(1);
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
      await palette.getByRole("button", { name: "再試一次", exact: true }).click();
      const picker = page.getByLabel("從哪一種節點開始", { exact: true });
      await expect(picker).toBeVisible();
      await expect(palette.getByRole("alert")).toHaveCount(0);
      await expect(palette).toBeFocused();
      await expect(draft).toHaveValue("仍可保留的文字驗收條件");
      expect(await page.axeViolations("main")).toEqual([]);
      await picker.select("contains_values");
      await page.getByRole("button", { name: "開始設計", exact: true }).click();
      await expect(add).toBeDisabled();
      await page.getByLabel("第 1 個", { exact: true }).fill("必須出現的值");
      await expect(add).toBeEnabled();
      await expect(draft).toHaveValue("仍可保留的文字驗收條件");
      expect(reads).toBe(2);
      expect(writes()).toEqual([]);
    } finally {
      release();
    }
  });
}
