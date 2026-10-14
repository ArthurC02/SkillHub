import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  CHECK_KINDS,
  PLATFORM_GRADER,
  TEST_CASE,
  VERSION,
  WORKSPACE_GRADER,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("check kinds recover locally without losing the criterion text", async ({ page }) => {
  await stubPlatform(page);
  let reads = 0;
  await page.route("**/evaluation/check-kinds", (route) => {
    reads += 1;
    return reads === 1
      ? route.fulfill({ status: 403, json: { error: "read forbidden" } })
      : route.fulfill({ json: CHECK_KINDS });
  });
  await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
  await page.getByLabel("新增驗收條件", { exact: true }).fill("尚未送出的條件");
  await expect(page.getByRole("alert").filter({ hasText: "檢查方式" })).toBeVisible();
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.getByLabel("從哪一種節點開始", { exact: true })).toBeVisible();
  await expect(page.getByLabel("新增驗收條件", { exact: true })).toHaveValue("尚未送出的條件");
  await expect(page.getByRole("group", { name: "檢查方式與設計", exact: true })).toBeFocused();
  expect(reads).toBe(2);
});

test("grader read failure prevents choosing an unverified option and recovers the existing criterion draft", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/evaluation/graders", (route) => {
    reads += 1;
    return reads === 1
      ? route.fulfill({ status: 403, json: { error: "read forbidden" } })
      : route.fulfill({ json: { graders: [PLATFORM_GRADER, WORKSPACE_GRADER] } });
  });
  await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
  await page.getByLabel("新增驗收條件", { exact: true }).fill("評分器尚未送出的條件");
  await page.getByLabel("從哪一種節點開始", { exact: true }).selectOption("prompt_grader");
  await page.getByRole("button", { name: "開始設計", exact: true }).click();
  const picker = page.getByLabel("評分器版本", { exact: true });
  await expect(page.getByRole("alert").filter({ hasText: "評分器" })).toBeVisible();
  await expect(picker).toBeDisabled();
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(picker).toBeEnabled();
  await picker.selectOption(WORKSPACE_GRADER.version_id);
  await expect(page.getByLabel("新增驗收條件", { exact: true })).toHaveValue(
    "評分器尚未送出的條件",
  );
  await expect(page.getByRole("button", { name: "新增", exact: true })).toBeEnabled();
  expect(reads).toBe(2);
  await expect(page.locator(".check-flow .react-flow__node")).toBeVisible();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page
    .locator(".check-editor")
    .screenshot({ path: testInfo.outputPath("check-editor-recovered-phone.png") });
  await page.emulateMedia({ colorScheme: "dark" });
  await page.evaluate(async () => {
    await Promise.all(
      document
        .getAnimations()
        .filter((animation) => animation.effect?.getTiming().iterations !== Infinity)
        .map((animation) => animation.finished.catch(() => undefined)),
    );
  });
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
});
