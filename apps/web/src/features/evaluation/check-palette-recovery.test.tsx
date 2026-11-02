import { expect, test, vi } from "vitest";
import {
  CHECK_KINDS,
  PLATFORM_GRADER,
  TEST_CASE,
  VERSION,
  WORKSPACE_GRADER,
} from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

vi.mock("./components/CheckFlow", () => ({ CheckFlow: () => null }));

const app = renderedApp();
const CASE_PATH = `/lab/test-cases/${TEST_CASE}?version=${VERSION}`;

test("check kinds recover locally without losing the criterion text", async () => {
  const page = app();
  let reads = 0;
  page.route("**/evaluation/check-kinds", () => {
    reads += 1;
    return reads === 1 ? { status: 403, json: { error: "read forbidden" } } : { json: CHECK_KINDS };
  });
  await page.goto(CASE_PATH);
  await page.getByLabel("新增驗收條件", { exact: true }).fill("尚未送出的條件");
  await expect(page.getByRole("alert").filter({ hasText: "檢查方式" })).toBeVisible();
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.getByLabel("從哪一種節點開始", { exact: true })).toBeVisible();
  await expect(page.getByLabel("新增驗收條件", { exact: true })).toHaveValue("尚未送出的條件");
  await expect(page.getByRole("group", { name: "檢查方式與設計", exact: true })).toBeFocused();
  expect(reads).toBe(2);
});

test("grader read failure prevents choosing an unverified option and recovers the existing criterion draft", async () => {
  const page = app();
  let reads = 0;
  page.route("**/evaluation/graders", () => {
    reads += 1;
    return reads === 1
      ? { status: 403, json: { error: "read forbidden" } }
      : { json: { graders: [PLATFORM_GRADER, WORKSPACE_GRADER] } };
  });
  await page.goto(CASE_PATH);
  await page.getByLabel("新增驗收條件", { exact: true }).fill("評分器尚未送出的條件");
  await page.getByLabel("從哪一種節點開始", { exact: true }).select("prompt_grader");
  await page.getByRole("button", { name: "開始設計", exact: true }).click();
  const picker = page.getByLabel("評分器版本", { exact: true });
  await expect(page.getByRole("alert").filter({ hasText: "評分器" })).toBeVisible();
  await expect(picker).toBeDisabled();
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(picker).toBeEnabled();
  await picker.select(WORKSPACE_GRADER.version_id);
  await expect(page.getByLabel("新增驗收條件", { exact: true })).toHaveValue(
    "評分器尚未送出的條件",
  );
  await expect(page.getByRole("button", { name: "新增", exact: true })).toBeEnabled();
  expect(reads).toBe(2);
  expect(await page.axeViolations("main")).toEqual([]);
});
