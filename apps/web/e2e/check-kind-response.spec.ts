import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { CHECK_KINDS, TEST_CASE, TEST_CASE_DRAFT, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ viewport: { width: 375, height: 900 }, trace: "retain-on-failure" });

async function inspect(page: Page, label: string) {
  await page.getByRole("group", { name: "檢查方式與設計", exact: true }).scrollIntoViewIfNeeded();
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await page.screenshot({ path: test.info().outputPath(`${label}.png`) });
}

for (const [name, invalid] of [
  ["missing kinds", {}],
  ["null kind", { ...CHECK_KINDS, kinds: [null] }],
  ["missing limits", { ...CHECK_KINDS, max_nodes: undefined, max_depth: undefined }],
  [
    "unreadable value bounds",
    { ...CHECK_KINDS, kinds: [{ ...CHECK_KINDS.kinds[0], values: null }] },
  ],
] as const) {
  for (const colorScheme of ["light", "dark"] as const) {
    test(`check kinds with ${name} preserve the optional criterion and recover in ${colorScheme} mode`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme });
      await stubPlatform(page);
      let reads = 0;
      let documents = 0;
      let release!: () => void;
      const firstRead = new Promise<void>((resolve) => {
        release = resolve;
      });
      const writes: string[] = [];
      page.on("domcontentloaded", () => documents++);
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      await page.route("**/evaluation/check-kinds", async (route) => {
        reads++;
        if (reads === 1) await firstRead;
        return route.fulfill({ json: reads === 1 ? invalid : CHECK_KINDS });
      });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
        const draft = page.getByLabel("新增驗收條件", { exact: true });
        const add = page.getByRole("button", { name: "新增", exact: true });
        await draft.fill("仍可保留的文字驗收條件");
        await expect(add).toBeEnabled();
        release();
        await expect(page.getByRole("status").filter({ hasText: "載入檢查方式中" })).toHaveCount(0);
        await writeFile(
          test.info().outputPath("check-kind-read-state.json"),
          JSON.stringify(
            await page.evaluate(() => ({
              heading: document.querySelector("main h1")?.textContent,
              draft: document.querySelector<HTMLInputElement>("#new-criterion")?.value,
              pickerCount: document.querySelectorAll(".check-editor-picker").length,
              alerts: Array.from(
                document.querySelectorAll('main [role="alert"]'),
                (alert) => alert.textContent,
              ),
            })),
          ),
        );
        const palette = page.getByRole("group", { name: "檢查方式與設計", exact: true });
        await expect(palette.getByRole("alert")).toContainText("暫時無法讀取檢查方式");
        await expect(
          page.getByRole("heading", { level: 1, name: TEST_CASE_DRAFT.name, exact: true }),
        ).toBeVisible();
        await expect(draft).toHaveValue("仍可保留的文字驗收條件");
        await expect(add).toBeEnabled();
        await expect(page.getByLabel("從哪一種節點開始", { exact: true })).toHaveCount(0);
        await expect(page.getByRole("button", { name: "重新載入頁面", exact: true })).toHaveCount(
          0,
        );
        expect(reads).toBe(1);
        expect(writes).toEqual([]);
        await inspect(page, "unreadable-check-kinds");
        await palette.getByRole("button", { name: "再試一次", exact: true }).click();
        const picker = page.getByLabel("從哪一種節點開始", { exact: true });
        await expect(picker).toBeVisible();
        await expect(palette.getByRole("alert")).toHaveCount(0);
        await expect(palette).toBeFocused();
        await expect(draft).toHaveValue("仍可保留的文字驗收條件");
        await inspect(page, "recovered-check-kinds");
        await picker.selectOption("contains_values");
        await page.getByRole("button", { name: "開始設計", exact: true }).click();
        await expect(add).toBeDisabled();
        await page.getByLabel("第 1 個", { exact: true }).fill("必須出現的值");
        await expect(add).toBeEnabled();
        await expect(draft).toHaveValue("仍可保留的文字驗收條件");
        expect(reads).toBe(2);
        expect(documents).toBe(1);
        expect(writes).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
