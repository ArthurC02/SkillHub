import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  PUBLICATION,
  PUBLISHER,
  SKILL,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const pages = [
  {
    name: "account",
    url: "/workspace/account",
    pattern: "**/me",
    heading: "帳號",
    ready: ".account-profile",
  },
  {
    name: "skill detail",
    url: `/skills/${SKILL}`,
    pattern: `**/api/skills/${SKILL}`,
    heading: "小工具總覽",
    ready: ".skill-detail",
  },
  {
    name: "version context",
    url: `/skills/${SKILL}/versions/${VERSION}`,
    pattern: `**/skills/${SKILL}/versions`,
    heading: "版本與發佈",
    ready: ".version-facts",
  },
  {
    name: "test case",
    url: `/lab/test-cases/${TEST_CASE}?version=${VERSION}`,
    pattern: `**/test-cases/${TEST_CASE}`,
    heading: "測試題",
    ready: ".test-case-detail-page",
  },
  {
    name: "packaging",
    url: `/skills/${SKILL}/package?version=${VERSION}`,
    pattern: `**/api/skills/${SKILL}?view=embedded`,
    heading: "小工具套件",
    ready: ".packaging-targets",
  },
  {
    name: "public publication",
    url: `/p/${PUBLISHER}/${PUBLICATION}`,
    pattern: `**/publications/${PUBLISHER}/${PUBLICATION}`,
    heading: "公開發佈物",
    ready: ".public-publication-page",
  },
  {
    name: "workspace",
    url: "/workspace",
    pattern: "**/me",
    heading: "工作台",
    ready: ".workspace-home",
  },
  {
    name: "admin",
    url: "/admin/dispatch",
    pattern: "**/me",
    heading: "派送煞車",
    ready: ".admin-page-heading",
  },
];

for (const entry of pages) {
  test(`${entry.name} read failure keeps page identity and supports an accessible in-place retry`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    if (entry.name === "packaging") await page.setViewportSize({ width: 375, height: 900 });
    let failing = true;
    let reads = 0;
    await page.route(entry.pattern, (route) => {
      if (route.request().resourceType() === "document") return route.fallback();
      reads += 1;
      const body = platformResponse(route.request().url()).body as object;
      return failing
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({ json: entry.name === "admin" ? { ...body, operator: true } : body });
    });
    await page.goto(entry.url);
    const content = page.getByRole("main");
    await expect(
      page.getByRole("heading", { name: entry.heading, level: 1, exact: true }),
    ).toBeVisible();
    await expect(content.getByRole("alert")).toContainText("暫時無法讀取");
    await expect(content.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
    if (entry.pattern === "**/me")
      await expect(page.getByRole("banner").getByRole("alert")).toHaveText(
        "登入狀態暫時無法確認。",
      );
    if (entry.name === "packaging")
      await page.screenshot({
        path: testInfo.outputPath("page-read-failed-phone.png"),
        fullPage: true,
      });
    const before = reads;
    failing = false;
    await content.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator(entry.ready)).toBeVisible();
    await expect(content.getByRole("alert").filter({ hasText: "暫時無法讀取" })).toHaveCount(0);
    if (entry.name === "admin") await expect(page.locator(".admin-page-heading")).toBeFocused();
    else await expect(page.getByRole("main")).toBeFocused();
    expect(reads).toBeGreaterThan(before);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(new URL(page.url()).pathname).toBe(entry.url.split("?")[0]);
    if (entry.url.includes("version="))
      expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
  });
}

test("a missing test case keeps its page identity and distinct absence response", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.route(`**/test-cases/${TEST_CASE}`, (route) =>
    route.request().resourceType() === "document"
      ? route.fallback()
      : route.fulfill({ status: 404, json: { error: "not found" } }),
  );
  await page.goto(`/lab/test-cases/${TEST_CASE}`);
  await expect(page.getByRole("heading", { name: "測試題", level: 1, exact: true })).toBeVisible();
  await expect(page.getByRole("alert")).toHaveText("找不到這個測試題。");
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
});

test("an expired workspace session retains its page heading and uses the login recovery", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.route("**/me", (route) =>
    route.fulfill({ status: 401, json: { error: "unauthorized" } }),
  );
  await page.goto("/workspace");
  await expect(page.getByRole("heading", { name: "工作台", level: 1, exact: true })).toBeVisible();
  await expect(
    page.getByRole("main").getByText("工作區首頁需要登入。", { exact: false }),
  ).toHaveCount(1);
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
});
