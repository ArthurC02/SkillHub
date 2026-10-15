import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { SKILL, SKILL_VERSIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const CURRENT = {
  versions: [
    {
      version_id: "99999999-1111-4111-8111-111111111111",
      version_number: 3,
      content_hash: "hash-new",
      created_at: "2026-10-15T12:00:00Z",
    },
    ...SKILL_VERSIONS.versions,
  ],
};
async function verifyVisibleFocus(page: Page) {
  const heading = page.getByRole("heading", { name: "版本", exact: true });
  await expect(heading).toBeFocused();
  const position = await heading.evaluate((element) => ({
    top: element.getBoundingClientRect().top,
    headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
  }));
  expect(position.top).toBeGreaterThanOrEqual(position.headerBottom);
}

for (const [cached, empty] of [
  [false, false],
  [true, false],
  [true, true],
] as const) {
  test(`version history failure with cached=${cached} empty=${empty} does not claim latest or no access and recovers in place`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let checking = !cached;
    let reads = 0;
    const writes: string[] = [];
    page.on("request", (request) => {
      if (request.method() !== "GET") writes.push(request.url());
    });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/skills/${SKILL}/versions`, async (route) => {
      reads += 1;
      if (!checking) return route.fulfill({ json: empty ? { versions: [] } : SKILL_VERSIONS });
      if (reads === (cached ? 2 : 1))
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      await gate;
      return route.fulfill({ json: CURRENT });
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      const history = page.getByRole("region", { name: "版本", exact: true });
      if (cached) {
        await expect(history.getByRole("button", { name: "重新整理", exact: true })).toBeVisible();
        await expect(
          history.getByText(empty ? "無權檢視" : "共 2 版，最新 v2", { exact: false }),
        ).toBeVisible();
        checking = true;
        await history.getByRole("button", { name: "重新整理", exact: true }).click();
      }
      await expect(history.getByRole("alert")).toContainText("暫時無法讀取版本歷史");
      await expect(history.getByText("最新 v2", { exact: false })).toHaveCount(0);
      await expect(history.getByText("無權檢視", { exact: false })).toHaveCount(0);
      await expect(history.getByRole("group")).toHaveCount(0);
      const retry = history.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => reads).toBe(cached ? 3 : 2);
      await expect(history.getByRole("status")).toContainText("載入版本歷史中…");
      await verifyVisibleFocus(page);
      await expect(history.getByText("最新 v2", { exact: false })).toHaveCount(0);
      release();
      await expect(history.getByText("共 3 版，最新 v3", { exact: false })).toBeVisible();
      await expect(history.getByRole("alert")).toHaveCount(0);
      await verifyVisibleFocus(page);
      await history.locator("summary").click();
      await expect(history.getByRole("link", { name: "v3", exact: true })).toHaveAttribute(
        "href",
        `/skills/${SKILL}/versions/99999999-1111-4111-8111-111111111111`,
      );
      expect(writes).toEqual([]);
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await history.scrollIntoViewIfNeeded();
      await page.screenshot({ path: test.info().outputPath("version-history-recovered.png") });
    } finally {
      release();
    }
  });
}

for (const mode of ["pending", "offline"] as const) {
  test(`a cached version history in ${mode} waits for a current snapshot without declaring latest`, async ({
    page,
    context,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let checking = false;
    let reads = 0;
    const writes: string[] = [];
    page.on("request", (request) => {
      if (request.method() !== "GET") writes.push(request.url());
    });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/skills/${SKILL}/versions`, async (route) => {
      reads += 1;
      if (checking) await gate;
      return route.fulfill({ json: checking ? CURRENT : SKILL_VERSIONS });
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      const history = page.getByRole("region", { name: "版本", exact: true });
      await expect(history.getByText("共 2 版，最新 v2", { exact: false })).toBeVisible();
      checking = true;
      if (mode === "offline") await context.setOffline(true);
      await history.getByRole("button", { name: "重新整理", exact: true }).click();
      await expect(history.getByRole("status")).toHaveText(
        mode === "offline" ? "等待連線以確認版本歷史；目前無法確認最新版本。" : "載入版本歷史中…",
      );
      await expect(history.getByText("共 2 版，最新 v2", { exact: false })).toHaveCount(0);
      await expect(history.locator("details")).toHaveCount(0);
      await expect(history.getByText("無權檢視", { exact: false })).toHaveCount(0);
      await verifyVisibleFocus(page);
      expect(reads).toBe(mode === "offline" ? 1 : 2);
      if (mode === "offline") await context.setOffline(false);
      await expect.poll(() => reads).toBe(2);
      release();
      await expect(history.getByText("共 3 版，最新 v3", { exact: false })).toBeVisible();
      await expect(history.getByText("上次取得於", { exact: false })).toBeVisible();
      await verifyVisibleFocus(page);
      expect(writes).toEqual([]);
    } finally {
      release();
      await context.setOffline(false);
    }
  });
}
