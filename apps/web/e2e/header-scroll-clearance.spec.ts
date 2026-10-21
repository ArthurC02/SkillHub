import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, TRACE_GENERAL } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function openRun(page: Page) {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await expect(page.locator("#feedback-comment")).toBeVisible();
}

async function capture(page: Page) {
  await page.screenshot({ path: test.info().outputPath("header-clearance.png") });
  await page.evaluate(() => scrollTo(0, 0));
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath("header-clearance-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
}

test("keyboard navigation keeps the active first text line below the sticky header", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.route(`**/runs/${RUN}/trace`, (route) =>
    route.fulfill({
      json: {
        ...TRACE_GENERAL,
        status: "succeeded",
        questions: [
          { question: "要處理哪個工作表？", why: "確認來源", options: ["Sheet1", "Sheet2"] },
        ],
      },
    }),
  );
  await openRun(page);
  const comment = page.locator("#feedback-comment");
  await expect(comment).toBeVisible();
  const answer = page
    .getByRole("region", { name: "這次試跑需要你回答", exact: true })
    .getByRole("textbox", { name: "你的回答", exact: true });
  await answer.click();
  for (let step = 0; step < 40; step++) {
    await page.keyboard.press("Shift+Tab");
    if ((await page.evaluate(() => document.activeElement?.id)) === "feedback-comment") break;
  }
  await expect(comment).toBeFocused();
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.type("keyboard text");
  await expect(comment).toHaveValue("keyboard text");
  const position = await comment.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const style = getComputedStyle(element);
    return {
      firstLineTop: rect.top + parseFloat(style.borderTopWidth) + parseFloat(style.paddingTop),
      firstLineBottom:
        rect.top +
        parseFloat(style.borderTopWidth) +
        parseFloat(style.paddingTop) +
        parseFloat(style.lineHeight),
      headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
      viewportBottom: innerHeight,
    };
  });
  expect(position.firstLineTop).toBeGreaterThanOrEqual(position.headerBottom);
  expect(position.firstLineBottom).toBeLessThanOrEqual(position.viewportBottom);
  await capture(page);
});

for (const width of [375, 1280]) {
  test(`section navigation keeps its heading below the header after resizing to ${width}`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await openRun(page);
    await page.setViewportSize({ width, height: 900 });
    const nav = page.getByRole("navigation", { name: "試跑結果導覽", exact: true });
    await nav.getByRole("link", { name: "任務判定", exact: true }).click();
    const position = await page.locator("#evaluation-heading").evaluate((element) => ({
      headingTop: element.getBoundingClientRect().top,
      headingBottom: element.getBoundingClientRect().bottom,
      headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
      viewportBottom: innerHeight,
    }));
    expect(position.headingTop).toBeGreaterThanOrEqual(position.headerBottom);
    expect(position.headingBottom).toBeLessThanOrEqual(position.viewportBottom);
    expect(new URL(page.url()).hash).toBe("#run-decision");
    await capture(page);
  });

  test(`header typing preserves the underlying reading position at ${width}`, async ({ page }) => {
    await stubPlatform(page);
    await openRun(page);
    await page.setViewportSize({ width, height: 900 });
    await page.locator("#feedback-comment").click();
    await page.keyboard.press("ControlOrMeta+A");
    await page.keyboard.type("reading context");
    const readingPosition = await page.evaluate(() => scrollY);
    const search = page.getByPlaceholder("搜尋小工具或描述任務");
    const bounds = await search.boundingBox();
    expect(bounds).not.toBeNull();
    await page.mouse.click(bounds!.x + bounds!.width / 2, bounds!.y + bounds!.height / 2);
    expect(await page.evaluate(() => scrollY)).toBe(readingPosition);
    await page.keyboard.type("PDF tools");
    await expect(search).toBeFocused();
    await expect(search).toHaveValue("PDF tools");
    expect(await page.evaluate(() => scrollY)).toBe(readingPosition);
    expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
    await capture(page);
  });
}
