import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { TEST_CASE, TEST_CASE_DRAFT, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function criterionReadFailure(page: Page, status: number) {
  await stubPlatform(page);
  let failing = false;
  let writes = 0;
  let stored = TEST_CASE_DRAFT;
  await page.route(`**/test-cases/${TEST_CASE}`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    if (failing && status === 0) return route.abort("failed");
    return failing
      ? route.fulfill({ status, json: { error: "read unavailable" } })
      : route.fulfill({ json: stored });
  });
  await page.route(`**/test-cases/${TEST_CASE}/criteria`, (route) => {
    writes += 1;
    const added = route.request().postDataJSON() as { text: string };
    stored = {
      ...stored,
      acceptance_criteria: [
        ...stored.acceptance_criteria,
        {
          id: "77777777-7777-4777-8777-777777777777",
          text: added.text,
          source: "user",
          confirmed_at: null,
        },
      ],
    };
    failing = true;
    return route.fulfill({ status: 201, json: stored });
  });
  return {
    recover: () => {
      failing = false;
    },
    writes: () => writes,
  };
}

for (const status of [503, 408, 429, 0]) {
  test(`a successful criterion followed by a failed read ${status} preserves unrelated unsaved test-case fields`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 375, height: 900 });
    const server = await criterionReadFailure(page, status);
    await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
    await page.getByLabel("名稱", { exact: true }).fill("未儲存的新名稱");
    await page.getByLabel("User Prompt", { exact: true }).fill("未儲存的任務提示");
    await page.getByLabel("使用者知道的事", { exact: true }).fill("未儲存的情境事實");
    await page.getByLabel("Rubric 版本", { exact: true }).fill("draft-rubric-v2");
    await page.getByLabel("新增驗收條件", { exact: true }).fill("已經新增的條件");
    await page.getByRole("button", { name: "新增", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("無法讀取測試題");
    await expect(page.locator(".test-case-detail-page")).toBeHidden();
    await expect(page.locator(".test-case-detail-page")).toHaveAttribute("inert", "");
    await expect(page.getByRole("textbox", { name: "名稱", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "新增", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
    server.recover();
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(page.getByLabel("名稱", { exact: true })).toHaveValue("未儲存的新名稱");
    await expect(page.getByLabel("User Prompt", { exact: true })).toHaveValue("未儲存的任務提示");
    await expect(page.getByLabel("使用者知道的事", { exact: true })).toHaveValue(
      "未儲存的情境事實",
    );
    await expect(page.getByLabel("Rubric 版本", { exact: true })).toHaveValue("draft-rubric-v2");
    await expect(
      page.locator(".criterion-list .criterion").filter({ hasText: "已經新增的條件" }),
    ).toHaveCount(1);
    expect(server.writes()).toBe(1);
    expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath("test-case-draft-recovered-phone.png"),
      fullPage: true,
    });
  });
}

for (const status of [401, 403, 404]) {
  test(`a subsequent test-case read ${status} removes the protected editor instead of retaining readable cached content`, async ({
    page,
  }) => {
    await criterionReadFailure(page, status);
    await page.goto(`/lab/test-cases/${TEST_CASE}`);
    await page.getByLabel("名稱", { exact: true }).fill("不能保留的保護內容");
    await page.getByLabel("新增驗收條件", { exact: true }).fill("新條件");
    await page.getByRole("button", { name: "新增", exact: true }).click();
    if (status === 401)
      await expect(page.getByText("測試題需要登入。", { exact: false })).toBeVisible();
    else await expect(page.getByRole("alert")).toBeVisible();
    await expect(page.locator("#edit-name")).toHaveCount(0);
    await expect(page.locator(".test-case-detail-page")).toHaveCount(0);
  });
}
