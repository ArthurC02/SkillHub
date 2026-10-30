import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function open(page: Page, width: number, kind: string) {
  await stubPlatform(page);
  await page.setViewportSize({ width, height: 900 });
  await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
  await page.goto(`/lab/test-cases/${TEST_CASE}`);
  await page.locator("#new-criterion").fill("新的驗收條件");
  await page.locator("#check-editor-root").selectOption(kind);
  const start = page.getByRole("button", { name: "開始設計", exact: true });
  await start.focus();
  await page.keyboard.press("Enter");
  await expect(page.locator(".check-editor-node")).toBeVisible();
  await expect(page.getByRole("group", { name: "檢查方式與設計", exact: true })).toBeFocused();
}

async function flowState(page: Page) {
  return page.locator(".react-flow").evaluate((flow) => {
    const frame = flow.getBoundingClientRect();
    const nodes = Array.from(flow.querySelectorAll(".react-flow__node")).map((node) => {
      const box = node.getBoundingClientRect();
      return {
        within:
          box.x >= frame.x &&
          box.right <= frame.right &&
          box.y >= frame.y &&
          box.bottom <= frame.bottom,
        centered:
          Math.abs(box.x + box.width / 2 - frame.x - frame.width / 2) < 2 &&
          Math.abs(box.y + box.height / 2 - frame.y - frame.height / 2) < 2,
      };
    });
    return {
      within: nodes.length > 0 && nodes.every((node) => node.within),
      centered: nodes.length === 1 && nodes[0].centered,
      transform: flow.querySelector<HTMLElement>(".react-flow__viewport")!.style.transform,
    };
  });
}

async function pan(page: Page, distance: number) {
  const canvas = page.locator(".react-flow");
  await canvas.scrollIntoViewIfNeeded();
  const box = await canvas.boundingBox();
  if (!box) throw new Error("Missing canvas bounds");
  await page.mouse.move(box.x + 10, box.y + box.height - 10);
  await page.mouse.down();
  await page.mouse.move(box.x + 10 + distance, box.y + box.height - 10, { steps: 4 });
  await page.mouse.up();
}

async function inspect(page: Page, info: TestInfo, selector: string) {
  const report = await new AxeBuilder({ page }).include(selector).analyze();
  await writeFile(info.outputPath("editor-layout-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("editor-layout.png") });
  return report;
}

async function addJudge(page: Page, count: number) {
  await page.locator(".check-editor-outline > li > button").focus();
  await page.keyboard.press("Enter");
  const picker = page.locator("#check-node-0-add");
  await picker.selectOption("judge");
  await picker.locator("..").getByRole("button", { name: "加入", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(page.locator(".react-flow__node")).toHaveCount(count);
  await expect(page.locator(".check-editor-node")).toBeFocused();
}

async function focusFirstValue(page: Page) {
  const input = page.locator("#check-node-0-value-0");
  await page.locator(".check-editor-node").focus();
  await page.keyboard.press("Tab");
  await expect(input).toBeFocused();
  await expect(input).toBeInViewport({ ratio: 1 });
  return input;
}

for (const width of [375, 1280]) {
  test(`check tree stays visible and keyboard focus follows edits at ${width}px`, async ({
    page,
  }, info) => {
    await open(page, width, "sequence");
    await addJudge(page, 2);
    await addJudge(page, 3);
    await expect.poll(async () => (await flowState(page)).within).toBe(true);
    const pausedAt = new Date();
    await page.clock.install({ time: new Date(pausedAt.getTime() - info.timeout) });
    await page.clock.pauseAt(pausedAt);
    for (const key of ["Enter", "Space"]) {
      await pan(page, 220);
      await expect.poll(async () => (await flowState(page)).within).toBe(false);
      await page.getByRole("button", { name: "顯示全部節點", exact: true }).focus();
      await page.keyboard.down(key);
      if (key === "Space") expect((await flowState(page)).within).toBe(false);
      await page.keyboard.up(key);
      await expect.poll(async () => (await flowState(page)).within).toBe(true);
    }
    await page.clock.resume();
    const firstPanel = await page.locator(".check-editor-panel").boundingBox();
    await page.setViewportSize({ width: width === 375 ? 1280 : 375, height: 900 });
    await expect.poll(async () => (await flowState(page)).within).toBe(true);
    const secondPanel = await page.locator(".check-editor-panel").boundingBox();
    expect(firstPanel).not.toBeNull();
    expect(secondPanel).not.toBeNull();
    const phone = width === 375 ? firstPanel! : secondPanel!;
    const desktop = width === 1280 ? firstPanel! : secondPanel!;
    expect(desktop.width).toBeGreaterThanOrEqual(phone.width);
    const child = page.locator(".check-editor-outline > li > ol > li > button").first();
    await child.focus();
    await page.keyboard.press("Enter");
    await expect(child).toBeFocused();
    await expect(page.locator(".check-editor-node h4")).toHaveText("選取的節點：評審判定");
    await inspect(page, info, '[aria-label="檢查方式與設計"]');
    await page.getByRole("button", { name: "刪除這個節點與它底下的節點", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator(".react-flow__node")).toHaveCount(2);
    await expect(page.locator(".check-editor-node")).toBeFocused();
    await page.getByRole("button", { name: "拿掉整個檢查", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("#check-editor-root")).toBeVisible();
    await expect(page.getByRole("group", { name: "檢查方式與設計", exact: true })).toBeFocused();
    await expect(page.locator("#new-criterion")).toHaveValue("新的驗收條件");
  });

  test(`check labels preserve full drafts and manual view at ${width}px`, async ({
    page,
  }, info) => {
    await open(page, width, "contains_values");
    await expect.poll(async () => (await flowState(page)).centered).toBe(true);
    await pan(page, 20);
    await expect.poll(async () => (await flowState(page)).centered).toBe(false);
    const viewport = (await flowState(page)).transform;
    const input = await focusFirstValue(page);
    const value = "資料".repeat(40);
    await page.keyboard.press("Control+A");
    await page.keyboard.insertText(value);
    await expect(input).toHaveValue(value);
    await expect(input).toBeInViewport({ ratio: 1 });
    await expect(page.locator(".check-editor-outline-node")).toContainText(value);
    await expect(page.locator(".check-flow-label")).toContainText(value);
    await expect(page.locator(".check-editor-node h4")).toHaveText("選取的節點：回答提到指定的值");
    expect((await flowState(page)).transform).toBe(viewport);
    expect((await flowState(page)).within).toBe(true);
    expect(
      await page
        .locator(".check-flow-label")
        .evaluate((label) => label.scrollWidth > label.clientWidth),
    ).toBe(true);
    await page.locator(".check-flow").scrollIntoViewIfNeeded();
    await page.screenshot({ path: info.outputPath("editor-label-preview.png") });
  });

  test(`check length feedback preserves a visible keyboard draft at ${width}px`, async ({
    page,
  }, info) => {
    await open(page, width, "contains_values");
    const input = await focusFirstValue(page);
    await page.keyboard.press("Control+A");
    await page.keyboard.insertText("a".repeat(201));
    await expect(input).toHaveValue("a".repeat(201));
    await expect(input).toHaveAttribute("aria-invalid", "true");
    await expect(page.getByText("值最多 200 個字，目前 201 個字。", { exact: true })).toHaveCount(
      1,
    );
    await expect(page.locator(".check-editor-outline-node")).toContainText("（要修正）");
    await expect(page.getByRole("button", { name: "新增", exact: true })).toBeDisabled();
    await expect(input).toBeFocused();
    await expect(input).toBeInViewport({ ratio: 1 });
    await inspect(page, info, '[aria-label="檢查方式與設計"]');
    await page.keyboard.press("Backspace");
    await expect(input).toHaveValue("a".repeat(200));
    await expect(input).toHaveAttribute("aria-invalid", "false");
    await expect(page.getByRole("button", { name: "新增", exact: true })).toBeEnabled();
  });

  test(`criterion length reason stays below its actions at ${width}px`, async ({ page }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
    await page.goto(`/lab/test-cases/${TEST_CASE}`);
    const input = page.locator("#new-criterion");
    await input.click();
    await page.keyboard.insertText("a".repeat(2001));
    await expect(input).toHaveValue("a".repeat(2001));
    const add = page.getByRole("button", { name: "新增", exact: true });
    await expect(add).toBeDisabled();
    const id = await add.getAttribute("aria-describedby");
    expect(id).toBeTruthy();
    await expect(input).toHaveAttribute("aria-describedby", id!);
    const reason = page.locator(`#${id}`);
    await expect(reason).toHaveText("驗收條件最多 2000 個字，目前 2001 個字。");
    const bottom = await page
      .locator("p:has(#new-criterion)")
      .getByRole("button")
      .evaluateAll((buttons) => buttons.map((button) => button.getBoundingClientRect().bottom));
    expect(bottom).toHaveLength(2);
    const box = await reason.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.y).toBeGreaterThanOrEqual(Math.max(...bottom));
    const report = await inspect(page, info, `p:has(#new-criterion), #${id}`);
    expect(report.incomplete).toEqual([]);
  });
}
