import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { SKILL, TEST_CASE, VERSION, RUN, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const [owner, endpoint, sentence] of [
  ["skill", `/api/skills/${SKILL}`, "暫時無法讀取這個小工具"],
  ["test case", `/test-cases/${TEST_CASE}`, "暫時無法讀取測試題"],
  ["summary", `/skills/${SKILL}/runs/preflight`, "暫時無法讀取權限摘要"],
] as const) {
  test(`preflight recovers its unreadable ${owner} without starting a Run or losing context`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    let documents = 0;
    const writes: string[] = [];
    page.on("request", (request) => {
      if (request.resourceType() === "document") documents += 1;
      if (request.method() === "POST") writes.push(request.url());
    });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**${endpoint}*`, async (route) => {
      const request = route.request();
      if (new URL(request.url()).pathname !== endpoint || request.method() !== "GET")
        return route.fallback();
      reads += 1;
      if (reads === 1)
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      await gate;
      return route.fulfill({ json: platformResponse(request.url()).body });
    });
    try {
      await page.goto(
        `/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}&against=${RUN}`,
      );
      const main = page.getByRole("main");
      await expect(main.getByRole("alert")).toContainText(sentence);
      expect(reads).toBe(1);
      const retry = main.getByRole("button", { name: "再試一次", exact: true });
      await expect(retry).toBeVisible();
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => reads).toBe(2);
      const heading = page.getByRole("heading", { name: "執行前權限確認", exact: true });
      await expect(heading).toBeFocused();
      await expect(main.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
      expect(writes).toEqual([]);
      release();
      await expect(main.getByRole("alert")).toHaveCount(0);
      await expect(main.locator(".preflight-context")).toContainText("PDF Summariser");
      await expect(main.locator(".preflight-context")).toContainText("去重複列");
      await expect(page.getByLabel("小工具 Version", { exact: true })).toHaveValue(VERSION);
      await expect(
        page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }),
      ).toBeEnabled();
      await expect(heading).toBeFocused();
      expect(reads).toBe(2);
      expect(documents).toBe(1);
      expect(writes).toEqual([]);
      const url = new URL(page.url());
      expect(url.searchParams.get("version")).toBe(VERSION);
      expect(url.searchParams.get("against")).toBe(RUN);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      await page.screenshot({
        path: test.info().outputPath("preflight-recovered.png"),
        fullPage: true,
      });
    } finally {
      release();
    }
  });
}
