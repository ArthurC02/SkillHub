import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { SKILL } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });
test.afterEach(async ({ page }) => {
  if (test.info().status === test.info().expectedStatus || page.isClosed()) return;
  const state = await page.locator(".app-nav").evaluate((nav) => ({
    width: nav.clientWidth,
    scrollWidth: nav.scrollWidth,
    scrollLeft: nav.scrollLeft,
    inset: getComputedStyle(nav).scrollPaddingInlineEnd,
    focus: { tag: document.activeElement?.tagName, id: document.activeElement?.id },
    visibility: document.visibilityState,
    bounds: nav.getBoundingClientRect().toJSON(),
    links: Array.from(nav.querySelectorAll("a"), (link) => ({
      text: link.textContent,
      current: link.getAttribute("aria-current"),
      bounds: link.getBoundingClientRect().toJSON(),
    })),
    cue: nav.querySelector(".nav-scroll-cue")?.outerHTML,
    cueBounds: nav.querySelector(".nav-scroll-cue")?.getBoundingClientRect().toJSON(),
  }));
  const path = test.info().outputPath("navigation-state.json");
  await writeFile(path, JSON.stringify(state));
  await test.info().attach("navigation-state", { path, contentType: "application/json" });
});

function linkIsUncovered(nav: Locator, selector: string) {
  return nav.evaluate((element, selector) => {
    const link = element.querySelector(selector);
    const cue = element.querySelector<HTMLElement>(".nav-scroll-cue");
    if (!link || !cue) return false;
    const bounds = element.getBoundingClientRect();
    const item = link.getBoundingClientRect();
    const end = cue.hidden ? bounds.right : cue.getBoundingClientRect().left;
    return item.left >= bounds.left && item.right <= end;
  }, selector);
}

function afterLayout(page: Page) {
  return page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
}

for (const colorScheme of ["light", "dark"] as const) {
  test(`the current main navigation item stays uncovered in ${colorScheme} mode`, async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme });
    await page.setViewportSize({ width: 320, height: 812 });
    await stubPlatform(page);
    await page.goto("/activity");
    const nav = page.locator(".app-nav");
    await expect(page.getByRole("heading", { level: 1, name: "活動", exact: true })).toBeVisible();
    const currentIsUncovered = () => linkIsUncovered(nav, '[aria-current="page"]');
    await expect.poll(currentIsUncovered).toBe(true);
    const focus = await page.evaluate(() => document.activeElement?.id);
    const scrollY = await page.evaluate(() => window.scrollY);
    await page.screenshot({ path: test.info().outputPath("current-navigation-viewport.png") });
    await afterLayout(page);
    await nav.evaluate((element) => {
      element.scrollLeft = 0;
      window.dispatchEvent(new Event("resize"));
    });
    await expect.poll(currentIsUncovered).toBe(true);
    await page.setViewportSize({ width: 640, height: 812 });
    await expect(nav.locator(".nav-scroll-cue")).toBeHidden();
    const report = await new AxeBuilder({ page }).include(".app-nav").analyze();
    await writeFile(test.info().outputPath("current-navigation-a11y.json"), JSON.stringify(report));
    expect(report.violations).toEqual([]);
    expect(report.incomplete).toEqual([]);
    await page.setViewportSize({ width: 320, height: 812 });
    await afterLayout(page);
    await expect.poll(currentIsUncovered).toBe(true);
    expect(await page.evaluate(() => document.activeElement?.id)).toBe(focus);
    expect(await page.evaluate(() => window.scrollY)).toBe(scrollY);
    await nav.evaluate((element) => {
      element.scrollLeft = 0;
    });
    await expect(nav.locator(".nav-scroll-cue")).toBeVisible();
    await expect.poll(() => nav.evaluate((element) => element.scrollLeft)).toBe(0);
    await nav.getByRole("link", { name: "發佈", exact: true }).click();
    await expect(nav.getByRole("link", { name: "發佈", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await expect.poll(currentIsUncovered).toBe(true);
    await nav.getByRole("link", { name: "工作台", exact: true }).click();
    await expect(nav.getByRole("link", { name: "工作台", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await expect.poll(currentIsUncovered).toBe(true);
    await page.goBack();
    await expect(nav.getByRole("link", { name: "發佈", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await expect.poll(currentIsUncovered).toBe(true);
  });
}

for (const colorScheme of ["light", "dark"] as const) {
  for (const [name, path, selector] of [
    ["main", "/activity", ".app-nav"],
    ["skill", `/skills/${SKILL}`, ".category-nav"],
  ]) {
    test(`${name} navigation keeps its overflow cue readable in ${colorScheme} mode`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme });
      await page.setViewportSize({ width: 320, height: 812 });
      await stubPlatform(page);
      await page.goto(path);
      const nav = page.locator(selector);
      const cue = nav.locator(":scope > .nav-scroll-cue");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);
      await nav.evaluate((element) => {
        element.scrollLeft = 0;
      });
      await expect(cue).toBeVisible();
      const report = await new AxeBuilder({ page }).include(selector).analyze();
      await writeFile(test.info().outputPath("navigation-a11y.json"), JSON.stringify(report));
      await page.screenshot({ path: test.info().outputPath("navigation-viewport.png") });
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await nav.locator("a").first().focus();
      const links = await nav.locator("a").count();
      for (let index = 1; index < links; index++) {
        await page.keyboard.press("Tab");
      }
      await expect(nav.locator("a").last()).toBeFocused();
      await expect(cue).toBeHidden();
      await page.screenshot({ path: test.info().outputPath("focused-navigation-viewport.png") });
      for (let index = 1; index < links; index++) {
        await page.keyboard.press("Shift+Tab");
      }
      await expect(nav.locator("a").first()).toBeFocused();
      await expect(cue).toBeVisible();
      await page.setViewportSize({ width: 640, height: 812 });
      await expect(cue).toBeHidden();
      await page.setViewportSize({ width: 320, height: 812 });
      await afterLayout(page);
      await expect.poll(() => linkIsUncovered(nav, ":focus")).toBe(true);
      await page.screenshot({
        path: test.info().outputPath("resized-focus-navigation-viewport.png"),
      });
      await nav.evaluate((element) => {
        element.scrollLeft = 0;
      });
      await expect(cue).toBeVisible();
      await nav.evaluate((element) => {
        element.scrollLeft = element.scrollWidth;
      });
      await expect(cue).toBeHidden();
    });
  }
}
