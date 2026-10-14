import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  SKILL,
  SKILL_B,
  SKILL_C,
  skillDetail,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("run comparison withholds cached decisions during refresh and recovers after failure", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  let releaseRead!: () => void;
  const held = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  await page.route(`**/runs/${RUN}/comparison?*`, async (route) => {
    reads += 1;
    if (reads === 2) {
      await held;
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    }
    return route.fulfill({
      json: {
        ...COMPARISON,
        runs: COMPARISON.runs.map((side) => ({
          ...side,
          final_output: reads === 1 ? "舊的輸出" : "更新後的輸出",
        })),
      },
    });
  });
  await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
  const table = page.getByRole("table", { name: "試跑紀錄任務判定與執行狀態對比" });
  await expect(table.getByText("舊的輸出", { exact: true })).toHaveCount(2);
  const refresh = page.getByRole("button", { name: /^更新比較/ });
  await refresh.focus();
  await page.keyboard.press("Enter");
  try {
    await expect.poll(() => reads).toBe(2);
    await expect(page.locator("main .verdict")).toHaveCount(0);
    await expect(table).toHaveCount(0);
    await expect(refresh).toBeFocused();
    await expect(refresh).toHaveAttribute("aria-disabled", "true");
    await page.keyboard.press("Enter");
    await expect(page.getByRole("status").filter({ hasText: "載入比較中" })).toBeVisible();
  } finally {
    releaseRead();
  }
  await expect(page.getByRole("alert")).toContainText("暫時無法讀取比較結果");
  await expect(table).toHaveCount(0);
  await expect(page.locator("main .verdict")).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("run-comparison-failed-phone.png") });
  await page.getByRole("button", { name: "再試一次" }).focus();
  await page.keyboard.press("Enter");
  await expect(table.getByText("更新後的輸出", { exact: true })).toHaveCount(2);
  await expect(page.locator("#run-comparison-heading")).toBeFocused();
  expect(reads).toBe(3);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("run-comparison-recovered-phone.png"),
    fullPage: true,
  });
});

test("skill comparison excludes a failed cached column and retries only that skill", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  const ids = [SKILL, SKILL_B, SKILL_C];
  const names = ["摘要工具", "失敗項目", "翻譯工具"];
  const reads = [0, 0, 0];
  let releaseRead!: () => void;
  const held = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  for (const [index, id] of ids.entries()) {
    await page.route(`**/api/skills/${id}?view=embedded`, async (route) => {
      reads[index] += 1;
      if (index === 1 && reads[index] === 2)
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      if (index === 1 && reads[index] === 3) await held;
      return route.fulfill({ json: skillDetail(id, names[index]) });
    });
  }
  await page.goto(`/compare?ids=${ids.join(",")}`);
  const table = page.getByRole("table", { name: /並排比較/ });
  await expect(table.getByRole("columnheader")).toHaveText(["比較項目", ...names]);
  await page.getByRole("button", { name: "更新比較資料" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "有 1 個小工具讀取失敗" })).toBeVisible();
  await expect(table.getByRole("columnheader")).toHaveText(["比較項目", "摘要工具", "翻譯工具"]);
  await expect(table.getByRole("link", { name: "失敗項目", exact: true })).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("skill-comparison-partial-phone.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "再試一次" }).focus();
  await page.keyboard.press("Enter");
  try {
    await expect.poll(() => reads).toEqual([2, 3, 2]);
    await expect(table).toHaveCount(0);
    await expect(page.locator("#skill-comparison-heading")).toBeFocused();
    await expect(page.getByRole("status").filter({ hasText: "載入中" })).toBeVisible();
  } finally {
    releaseRead();
  }
  await expect(table.getByRole("columnheader")).toHaveText(["比較項目", ...names]);
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(reads).toEqual([2, 3, 2]);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("skill-comparison-recovered-phone.png"),
    fullPage: true,
  });
});

for (const dependency of ["self", "candidates", "versions"] as const) {
  test(`comparison can retry unreadable ${dependency} without losing its ID draft`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let fail = true;
    const path =
      dependency === "self"
        ? `**/runs/${RUN}`
        : dependency === "candidates"
          ? "**/runs?*"
          : `**/skills/${SKILL}/versions`;
    await page.route(path, (route) =>
      fail
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fallback(),
    );
    await page.goto(`/runs/${RUN}/compare`);
    const what =
      dependency === "self"
        ? "目前這次試跑"
        : dependency === "candidates"
          ? "可比較的試跑紀錄"
          : "候選試跑紀錄的 Version 編號";
    await expect(page.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
    const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
    await field.fill("尚未送出的 ID");
    fail = false;
    await page.getByRole("button", { name: "再試一次" }).focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^以 v2、/ })).toBeVisible();
    await expect(field).toHaveValue("尚未送出的 ID");
    await expect(page.locator("#run-comparison-heading")).toBeFocused();
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  });
}
