import { expect, test, type Page, type TestInfo } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { RUN, SKILL, VERSION, TEST_CASE } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function capture(page: Page, info: TestInfo, state: string) {
  const audit = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(`${state}-identity-a11y.json`), JSON.stringify(audit, null, 2));
  expect(audit.violations).toEqual([]);
  expect(audit.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}-identity.png`) });
}

for (const width of [375, 1280]) {
  test(`Run identifier disclosure preserves the decision and keyboard continuation at ${width}`, async ({
    page,
  }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    const writes: string[] = [];
    page.on("request", (request) => {
      if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
    });
    await page.goto(`/runs/${RUN}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await expect(page.getByRole("combobox", { name: "評估版本", exact: true })).toBeVisible();
    await page.evaluate(() => document.fonts.ready.then(() => undefined));
    const identity = page.locator(".run-identity");
    const disclosure = page.locator(".run-workspace-header > details:has(.run-identity)");
    const summary = disclosure.locator("summary");
    await expect(summary).toHaveText("試跑識別碼");
    await expect(disclosure).toHaveJSProperty("open", false);
    await expect(identity.locator("code")).toHaveText(RUN);
    await expect(identity).toBeHidden();
    const verdict = page.locator("#run-decision .verdict");
    await expect(verdict).toHaveText("任務判定：未符合");
    expect(await verdict.evaluate((element) => element.closest("details") === null)).toBe(true);
    const position = await verdict.evaluate((element) => ({
      top: element.getBoundingClientRect().top,
      bottom: element.getBoundingClientRect().bottom,
      headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
    }));
    expect(position.top).toBeGreaterThanOrEqual(position.headerBottom);
    expect(position.bottom).toBeLessThanOrEqual(900);
    const source = page.getByRole("navigation", { name: "這次試跑的來源", exact: true });
    await expect(source).toBeVisible();
    expect(await source.evaluate((element) => element.closest("details") === null)).toBe(true);
    await expect(source.getByRole("link", { name: "來源小工具", exact: true })).toHaveAttribute(
      "href",
      `/skills/${SKILL}`,
    );
    await expect(source.getByRole("link", { name: "來源 Version", exact: true })).toHaveAttribute(
      "href",
      `/skills/${SKILL}/versions/${VERSION}`,
    );
    await expect(source.getByRole("link", { name: "來源測試題", exact: true })).toHaveAttribute(
      "href",
      `/lab/test-cases/${TEST_CASE}?version=${VERSION}`,
    );
    await page.keyboard.press("Tab");
    await expect(page.getByRole("link", { name: "跳到主要內容", exact: true })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page.locator("#main-content")).toBeFocused();
    const heading = await page.locator("#run-heading").evaluate((element) => ({
      top: element.getBoundingClientRect().top,
      headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
    }));
    expect(heading.top).toBeGreaterThanOrEqual(heading.headerBottom);
    await page.keyboard.press("Tab");
    await expect(summary).toBeFocused();
    const focused = await summary.evaluate((element) => ({
      top: element.getBoundingClientRect().top,
      bottom: element.getBoundingClientRect().bottom,
      headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
    }));
    expect(focused.top).toBeGreaterThanOrEqual(focused.headerBottom);
    expect(focused.bottom).toBeLessThanOrEqual(900);
    await capture(page, info, "closed");
    await page.keyboard.press("Enter");
    await expect(disclosure).toHaveJSProperty("open", true);
    await expect(identity).toBeVisible();
    await expect(identity.locator("code")).toHaveText(RUN);
    await expect(summary).toBeFocused();
    await capture(page, info, "open");
    await page.keyboard.press("Space");
    await expect(disclosure).toHaveJSProperty("open", false);
    await expect(identity).toBeHidden();
    await expect(summary).toBeFocused();
    await expect(verdict).toBeVisible();
    await expect(source).toBeVisible();
    expect(writes).toEqual([]);
    expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
    expect(new URL(page.url()).hash).toBe("#main-content");
  });
}
