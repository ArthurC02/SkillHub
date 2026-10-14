import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { RUN, RUN_ARTIFACTS, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const empty of [false, true]) {
  test(`artifact read failure withdraws cached ${empty ? "empty" : "populated"} claims and recovers locally`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    await page.route(`**/runs/${RUN}/artifacts`, (route) => {
      reads += 1;
      return reads === 2
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({
            json: {
              ...RUN_ARTIFACTS,
              artifacts: empty && reads === 1 ? [] : RUN_ARTIFACTS.artifacts,
            },
          });
    });
    await page.goto(`/runs/${RUN}`);
    const section = page.locator("#run-artifacts");
    if (empty)
      await expect(
        section.getByText("這次試跑沒有留下任何檔案產出。", { exact: true }),
      ).toBeVisible();
    else await expect(section.locator(".download-item")).toHaveCount(1);
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    await page.goBack();
    await expect(section.getByRole("alert")).toContainText("產出清單");
    await expect(section.locator(".download-item")).toHaveCount(0);
    await expect(section.getByText("這次試跑沒有留下任何檔案產出。", { exact: true })).toHaveCount(
      0,
    );
    await section.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(section.locator(".download-item")).toHaveCount(1);
    await expect(page.locator("#run-artifacts-heading")).toBeFocused();
    expect(reads).toBe(3);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath("run-read-recovered-phone.png"),
      fullPage: true,
    });
  });
}

test("run context failure hides cached source navigation and retries that owner alone", async ({
  page,
}) => {
  await stubPlatform(page);
  let reads = 0;
  let failing = false;
  await page.route(`**/runs/${RUN}`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    reads += 1;
    return failing
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: platformResponse(route.request().url()).body });
  });
  await page.goto(`/runs/${RUN}`);
  await expect(page.getByRole("navigation", { name: "這次試跑的來源", exact: true })).toBeVisible();
  await page.locator('a[href="/policy"]').first().click();
  await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
  failing = true;
  await page.goBack();
  const header = page.locator(".run-workspace-header");
  await expect(header.getByRole("alert")).toContainText("這次試跑");
  await expect(page.getByRole("navigation", { name: "這次試跑的來源", exact: true })).toHaveCount(
    0,
  );
  const beforeRetry = reads;
  failing = false;
  await header.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.getByRole("navigation", { name: "這次試跑的來源", exact: true })).toBeVisible();
  await expect(page.locator("#run-heading")).toBeFocused();
  expect(reads).toBe(beforeRetry + 1);
});

for (const advanced of [false, true]) {
  test(`${advanced ? "advanced" : "general"} trace can retry a failed read without changing its URL context`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let reads = 0;
    const offsets: string[] = [];
    const pattern = advanced ? `**/runs/${RUN}/trace?mode=advanced&*` : `**/runs/${RUN}/trace`;
    await page.route(pattern, (route) => {
      reads += 1;
      if (advanced) offsets.push(new URL(route.request().url()).searchParams.get("after")!);
      return reads === 1
        ? route.fulfill({ status: 403, json: { error: "read forbidden" } })
        : route.fulfill({ json: platformResponse(route.request().url()).body });
    });
    await page.goto(`/runs/${RUN}${advanced ? "?events=1000" : ""}`);
    const section = page.locator("#run-trace");
    await expect(section.getByRole("alert")).toContainText("無法讀取執行紀錄");
    await expect(section.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
    await section.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(section.getByRole("alert")).toHaveCount(0);
    await expect(page.locator("#run-trace-heading")).toBeFocused();
    expect(reads).toBe(2);
    if (advanced) expect(offsets).toEqual(["1000", "1000"]);
    else expect(new URL(page.url()).searchParams.get("events")).toBeNull();
  });
}

test("a failed live trace withdraws cached progress and cancellation until its state is readable", async ({
  page,
}) => {
  await stubPlatform(page);
  let reads = 0;
  await page.route(`**/runs/${RUN}/trace`, (route) => {
    reads += 1;
    return reads === 2
      ? route.fulfill({ status: 403, json: { error: "read forbidden" } })
      : route.fulfill({
          json: {
            ...(platformResponse(route.request().url()).body as object),
            status: reads === 1 ? "running" : "succeeded",
          },
        });
  });
  await page.goto(`/runs/${RUN}`);
  await expect(page.getByText("進行中：執行中", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "取消這次試跑", exact: true })).toBeVisible();
  const section = page.locator("#run-trace");
  await expect(section.getByRole("alert")).toContainText("無法讀取執行紀錄");
  await expect(page.getByText("進行中：執行中", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "取消這次試跑", exact: true })).toHaveCount(0);
  await section.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(section.getByRole("alert")).toHaveCount(0);
  await expect(section.getByText("執行狀態：", { exact: false })).toContainText("執行完成");
  expect(reads).toBe(3);
});
