import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE, TEST_CASE_DRAFT } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const scenarios = [
  { name: "unchanged check", initialCheck: true, edit: "none", text: "", check: null },
  { name: "newer text", initialCheck: true, edit: "text", text: "下一條草稿", check: " 42 " },
  {
    name: "newer check",
    initialCheck: false,
    edit: "check",
    text: "已送出的這一條",
    check: "下一條的值",
  },
  {
    name: "removed check",
    initialCheck: true,
    edit: "remove",
    text: "已送出的這一條",
    check: null,
  },
  {
    name: "failed newer text",
    initialCheck: true,
    edit: "text",
    text: "下一條草稿",
    check: " 42 ",
  },
];

async function inspectCriterionControls(page: Page, info: TestInfo, failed: boolean) {
  const fullRegion = await new AxeBuilder({ page })
    .include('section[aria-label="驗收與評分設計"]')
    .analyze();
  await writeFile(
    info.outputPath("criterion-draft-full-region-a11y.json"),
    JSON.stringify(fullRegion),
  );
  expect(fullRegion.violations).toEqual([]);
  const controls = new AxeBuilder({ page }).include("p:has(#new-criterion)");
  controls.include(
    failed
      ? 'section[aria-label="驗收與評分設計"] p[role="alert"]'
      : 'section[aria-label="驗收與評分設計"] > p[role="status"]',
  );
  const axe = await controls.analyze();
  await writeFile(info.outputPath("criterion-draft-a11y.json"), JSON.stringify(axe));
  expect(axe.violations).toEqual([]);
  expect(axe.incomplete).toEqual([]);
  await page.screenshot({ path: info.outputPath("criterion-draft.png") });
}

for (const scenario of scenarios) {
  for (const width of [375, 1280]) {
    test(`new criterion preserves ${scenario.name} during submission at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const current = structuredClone(TEST_CASE_DRAFT);
      const writes: unknown[] = [];
      let reads = 0;
      let release!: () => void;
      let started!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      const received = new Promise<void>((resolve) => {
        started = resolve;
      });
      const failed = scenario.name.startsWith("failed");
      await page.route(`**/test-cases/${TEST_CASE}`, (route) => {
        if (route.request().isNavigationRequest()) return route.fallback();
        reads += 1;
        return route.fulfill({ json: current });
      });
      await page.route(`**/test-cases/${TEST_CASE}/criteria`, async (route) => {
        writes.push({ method: route.request().method(), body: route.request().postDataJSON() });
        started();
        await held;
        if (failed) return route.fulfill({ status: 500, json: { error: "新增驗收條件失敗" } });
        current.acceptance_criteria.push({
          ...current.acceptance_criteria[0],
          id: "c-added",
          text: "已送出的這一條",
          source: "user",
          confirmed_at: null,
        });
        return route.fulfill({ status: 201, json: current });
      });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}`);
        const field = page.locator("#new-criterion");
        const checkField = page.getByRole("textbox", { name: "第 1 個", exact: true });
        const add = page.getByRole("button", { name: /^新增(?:中…)?$/ });
        await expect(field).toHaveValue("");
        await field.fill("已送出的這一條");
        if (scenario.initialCheck) {
          await page.getByRole("button", { name: "開始設計", exact: true }).click();
          await checkField.fill(" 42 ");
        }
        await expect(add).toBeEnabled();
        await add.click();
        await received;
        await expect(add).toHaveText("新增中…");
        await expect(add).toBeDisabled();
        await expect(field).toBeEditable();
        if (scenario.edit === "text") await field.fill("下一條草稿");
        else if (scenario.edit === "check") {
          await page.getByRole("button", { name: "開始設計", exact: true }).click();
          await checkField.fill("下一條的值");
        } else if (scenario.edit === "remove") {
          await page.getByRole("button", { name: "拿掉整個檢查", exact: true }).click();
        }
        release();
        if (failed) await expect(page.getByRole("alert")).toHaveText("無法新增，可以再試一次。");
        else await expect(page.locator("#criterion-c-added")).toHaveValue("已送出的這一條");
        await expect(add).toHaveText("新增");
        await expect(field).toHaveValue(scenario.text);
        if (scenario.check === null) await expect(checkField).toHaveCount(0);
        else await expect(checkField).toHaveValue(scenario.check);
        if (scenario.text) await expect(add).toBeEnabled();
        else await expect(add).toBeDisabled();
        const receipt = page.getByRole("status").filter({ hasText: "已新增驗收條件。" });
        if (failed) await expect(receipt).toHaveCount(0);
        else
          await expect(receipt).toHaveText(
            scenario.edit === "none"
              ? "已新增驗收條件。"
              : "已新增驗收條件。目前欄位裡的草稿尚未新增。",
          );
        expect(reads).toBe(failed ? 1 : 2);
        expect(writes).toEqual([
          {
            method: "POST",
            body: {
              source: "user",
              text: "已送出的這一條",
              ...(scenario.initialCheck
                ? { check: { kind: "contains_values", values: ["42"] } }
                : {}),
            },
          },
        ]);
        await field.click();
        await expect(field).toBeFocused();
        await expect(field).toBeInViewport({ ratio: 1 });
        if (failed) {
          const failure = page.getByRole("alert");
          await failure.scrollIntoViewIfNeeded();
          await expect(failure).toBeInViewport({ ratio: 1 });
          await expect(field).toBeInViewport({ ratio: 1 });
        }
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await inspectCriterionControls(page, info, failed);
      } finally {
        release();
      }
    });
  }
}
