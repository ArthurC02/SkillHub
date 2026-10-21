import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  DOWNLOADS,
  SKILL,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import { creationSnapshot } from "../src/testing/fixtures/creation";

const SESSION = {
  id: "44444444-4444-4444-8444-444444444444",
  revision: 7,
  state: "saved",
  snapshot: creationSnapshot("整理採購文件並產生摘要"),
  created_at: "2026-09-27T09:00:00Z",
  updated_at: "2026-09-28T09:30:00Z",
  expires_at: "2099-09-27T09:00:00Z",
  deadline: "2026-09-27T10:00:00Z",
};

for (const empty of [false, true]) {
  test(`version reread failures hide cached ${empty ? "empty" : "populated"} deliveries and creation history and retry each owner`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    await page.route("**/me", (route) =>
      route.fulfill({
        json: {
          ...(platformResponse(route.request().url()).body as object),
          features: { creation_skill: true },
        },
      }),
    );
    const owners = [
      {
        pattern: "**/downloads",
        heading: "version-deliverables-title",
        rows: "[data-version-deliverable]",
        initial: empty ? { downloads: [] } : DOWNLOADS,
        recovered: DOWNLOADS,
        count: 2,
        absent: "這一版還沒有交付套件。",
      },
      {
        pattern: "**/creation-sessions?*",
        heading: "version-creation-title",
        rows: ".version-creation-list > li",
        initial: empty ? [] : [SESSION],
        recovered: [SESSION],
        count: 1,
        absent: "沒有仍可開啟的 Studio 會話",
      },
    ];
    const reads = [0, 0];
    const retrying = [false, false];
    let fail = false;
    for (const [index, owner] of owners.entries()) {
      await page.route(owner.pattern, (route) => {
        reads[index] += 1;
        return fail && !retrying[index]
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fulfill({ json: retrying[index] ? owner.recovered : owner.initial });
      });
    }
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    await expect.poll(() => reads).toEqual([1, 1]);
    for (const owner of owners) {
      await expect(page.locator(owner.rows)).toHaveCount(empty ? 0 : owner.count);
      if (empty) await expect(page.getByText(owner.absent, { exact: false })).toBeVisible();
    }
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    fail = true;
    await page.goBack();
    await expect(page.getByRole("alert")).toHaveCount(2);
    for (const [index, owner] of owners.entries()) {
      const section = page.locator(`section[aria-labelledby=${owner.heading}]`);
      await expect(page.locator(owner.rows)).toHaveCount(0);
      await expect(page.getByText(owner.absent, { exact: false })).toHaveCount(0);
      retrying[index] = true;
      const before = [...reads];
      await section.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(page.locator(owner.rows)).toHaveCount(owner.count);
      expect(reads.map((count, i) => count - before[i])).toEqual(index === 0 ? [1, 0] : [0, 1]);
      await expect(page.locator(`#${owner.heading}`)).toBeFocused();
    }
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath(`version-${empty}-recovered.png`),
      fullPage: true,
    });
  });
}

test("test-case context stops using stale names and versions and retries without clearing the task draft", async ({
  page,
}) => {
  await stubPlatform(page);
  const owners = [
    { pattern: `**/api/skills/${SKILL}`, path: `/api/skills/${SKILL}` },
    { pattern: `**/skills/${SKILL}/versions`, path: `/skills/${SKILL}/versions` },
  ];
  const reads = [0, 0];
  for (const [index, owner] of owners.entries()) {
    await page.route(owner.pattern, (route) => {
      reads[index] += 1;
      return reads[index] === 2
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({ json: platformResponse(owner.path).body });
    });
  }
  await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
  const context = page.locator("[data-role=test-case-context]");
  await expect(context.getByText("由這個網址選定", { exact: true })).toBeVisible();
  await page.locator('a[href="/policy"]').first().click();
  await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
  await page.goBack();
  await expect(context.getByRole("alert")).toHaveCount(2);
  await expect(context.getByText("由這個網址選定", { exact: true })).toHaveCount(0);
  await expect(context.getByRole("link", { name: "PDF Summariser", exact: true })).toHaveCount(0);
  await expect(context).toContainText("名稱讀取失敗");
  await expect(context).toContainText("Version 資訊讀取失敗");
  const prompt = page.locator(".test-case-design textarea").first();
  await prompt.fill("保持這段尚未儲存的任務提示");
  for (let index = 0; index < 2; index += 1) {
    await context.getByRole("button", { name: "再試一次", exact: true }).first().click();
    await expect(context.getByRole("alert")).toHaveCount(1 - index);
    await expect(page.locator("#test-case-context-title")).toBeFocused();
  }
  await expect(prompt).toHaveValue("保持這段尚未儲存的任務提示");
  await expect(context.getByText("由這個網址選定", { exact: true })).toBeVisible();
  expect(reads).toEqual([3, 3]);
  expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
});
