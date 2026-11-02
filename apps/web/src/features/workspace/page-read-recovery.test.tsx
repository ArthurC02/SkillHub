import { expect, test } from "vitest";
import {
  PUBLICATION,
  PUBLISHER,
  SKILL,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

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
    ready: ".detail-answer",
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
  test(`${entry.name} read failure keeps page identity and supports an accessible in-place retry`, async () => {
    const page = app();
    let failing = true;
    let reads = 0;
    page.route(entry.pattern, ({ url }) => {
      reads += 1;
      const body = platformResponse(url.pathname + url.search).body as object;
      return failing
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: entry.name === "admin" ? { ...body, operator: true } : body };
    });
    await page.goto(entry.url);
    const content = page.locator("main");
    const alerts = content.locator("[role=alert]");
    const retry = content.locator("button").filter({ hasText: "再試一次" });
    await page.eventually(() => {
      expect(page.locator("h1").filter({ hasText: entry.heading }).count()).toBe(1);
      expect(alerts.text()).toContain("暫時無法讀取");
      expect(retry.count()).toBe(1);
      if (entry.pattern === "**/me")
        expect(page.locator("header [role=alert]").text()).toBe("登入狀態暫時無法確認。");
    });
    const before = reads;
    failing = false;
    await retry.focus();
    await page.press("Enter");
    await page.eventually(() => {
      expect(page.locator(entry.ready).count()).toBe(1);
      expect(alerts.filter({ hasText: "暫時無法讀取" }).count()).toBe(0);
      expect(
        (entry.name === "admin" ? page.locator(".admin-page-heading") : content).isFocused(),
      ).toBe(true);
    });
    expect(reads).toBeGreaterThan(before);
    expect(await page.axeViolations("main")).toEqual([]);
    expect(page.pathname()).toBe(entry.url.split("?")[0]);
    if (entry.url.includes("version="))
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
  });
}

test("a missing test case keeps its page identity and distinct absence response", async () => {
  const page = app();
  page.route(`**/test-cases/${TEST_CASE}`, () => ({ status: 404, json: { error: "not found" } }));
  await page.goto(`/lab/test-cases/${TEST_CASE}`);
  await page.eventually(() => {
    expect(page.locator("h1").filter({ hasText: "測試題" }).count()).toBe(1);
    expect(page.getByRole("alert").text()).toBe("找不到這個測試題。");
    expect(page.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(0);
  });
});

test("an expired workspace session retains its page heading and uses the login recovery", async () => {
  const page = app();
  page.route("**/me", () => ({ status: 401, json: { error: "unauthorized" } }));
  await page.goto("/workspace");
  await page.eventually(() => {
    expect(page.locator("h1").filter({ hasText: "工作台" }).count()).toBe(1);
    expect(page.locator("main").text()).toContain("工作區首頁需要登入。");
    expect(page.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(0);
  });
});
