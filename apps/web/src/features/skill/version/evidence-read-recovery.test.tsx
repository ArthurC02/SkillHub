import { expect, test } from "vitest";
import {
  DOWNLOADS,
  SKILL,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { creationSnapshot } from "../../../testing/fixtures/creation";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

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
  test(`version reread failures hide cached ${empty ? "empty" : "populated"} deliveries and creation history and retry each owner`, async () => {
    const page = app();
    page.route("**/me", ({ url }) => ({
      json: {
        ...(platformResponse(url.pathname).body as object),
        features: { creation_skill: true },
      },
    }));
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
      page.route(owner.pattern, () => {
        reads[index] += 1;
        return fail && !retrying[index]
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: retrying[index] ? owner.recovered : owner.initial };
      });
    }
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    await page.eventually(() => {
      expect(reads).toEqual([1, 1]);
      for (const owner of owners) {
        expect(page.locator(owner.rows).count()).toBe(empty ? 0 : owner.count);
        if (empty) expect(page.getByText(owner.absent).count()).toBeGreaterThan(0);
      }
    });
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
    fail = true;
    await page.goBack();
    await expect(page.getByRole("alert")).toHaveCount(2);
    for (const [index, owner] of owners.entries()) {
      const section = page.locator(`section[aria-labelledby=${owner.heading}]`);
      await expect(page.locator(owner.rows)).toHaveCount(0);
      await expect(page.getByText(owner.absent)).toHaveCount(0);
      retrying[index] = true;
      const before = [...reads];
      await section.locator("button").filter({ hasText: "再試一次" }).focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(page.locator(owner.rows).count()).toBe(owner.count);
        expect(page.locator(`#${owner.heading}`).isFocused()).toBe(true);
      });
      expect(reads.map((count, i) => count - before[i])).toEqual(index === 0 ? [1, 0] : [0, 1]);
    }
    expect(await page.axeViolations("main")).toEqual([]);
  });
}

test("test-case context stops using stale names and versions and retries without clearing the task draft", async () => {
  const page = app();
  const owners = [
    { pattern: `**/api/skills/${SKILL}`, path: `/api/skills/${SKILL}` },
    { pattern: `**/skills/${SKILL}/versions`, path: `/skills/${SKILL}/versions` },
  ];
  const reads = [0, 0];
  for (const [index, owner] of owners.entries()) {
    page.route(owner.pattern, () => {
      reads[index] += 1;
      return reads[index] === 2
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: platformResponse(owner.path).body };
    });
  }
  await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
  const context = page.locator("[data-role=test-case-context]");
  const alerts = page.locator("[data-role=test-case-context] [role=alert]");
  await expect(page.getByText("由這個網址選定", { exact: true })).toBeVisible();
  await page.locator('a[href="/policy"]').first().click();
  await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
  await page.goBack();
  await expect(alerts).toHaveCount(2);
  await expect(page.getByText("由這個網址選定", { exact: true })).toHaveCount(0);
  await expect(context.locator("a").filter({ hasText: "PDF Summariser" })).toHaveCount(0);
  await expect(context).toContainText("名稱讀取失敗");
  await expect(context).toContainText("Version 資訊讀取失敗");
  const prompt = page.locator(".test-case-design textarea").first();
  await prompt.fill("保持這段尚未儲存的任務提示");
  for (let index = 0; index < 2; index += 1) {
    await context.locator("button").filter({ hasText: "再試一次" }).first().click();
    await page.eventually(() => {
      expect(alerts.count()).toBe(1 - index);
      expect(page.locator("#test-case-context-title").isFocused()).toBe(true);
    });
  }
  await expect(prompt).toHaveValue("保持這段尚未儲存的任務提示");
  await expect(page.getByText("由這個網址選定", { exact: true })).toBeVisible();
  expect(reads).toEqual([3, 3]);
  expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
});
