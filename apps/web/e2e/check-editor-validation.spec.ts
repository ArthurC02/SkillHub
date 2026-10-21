import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE, TEST_CASE_DRAFT } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function open(page: Page, width: number, kind: string) {
  await stubPlatform(page);
  await page.setViewportSize({ width, height: 900 });
  await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
  const draft = structuredClone(TEST_CASE_DRAFT);
  const writes: unknown[] = [];
  let reads = 0;
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  const received = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route(`**/test-cases/${TEST_CASE}`, (route) => {
    if (route.request().isNavigationRequest()) return route.fallback();
    reads++;
    return route.fulfill({ json: draft });
  });
  await page.route(`**/test-cases/${TEST_CASE}/criteria`, async (route) => {
    writes.push({ method: route.request().method(), body: route.request().postDataJSON() });
    started();
    await held;
    draft.acceptance_criteria.push({
      ...draft.acceptance_criteria[0],
      id: "c-validated",
      text: "新的驗收條件",
      source: "user",
      confirmed_at: null,
    });
    return route.fulfill({ status: 201, json: draft });
  });
  await page.goto(`/lab/test-cases/${TEST_CASE}`);
  await page.locator("#new-criterion").fill("新的驗收條件");
  await page.locator("#check-editor-root").selectOption(kind);
  await page.getByRole("button", { name: "開始設計", exact: true }).click();
  return { writes, received, release, readCount: () => reads };
}

async function inspect(page: Page, info: TestInfo) {
  const report = await new AxeBuilder({ page }).include(".check-editor-panel").analyze();
  await writeFile(info.outputPath("check-editor-panel-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("check-editor-validation.png") });
}

async function submit(page: Page, state: Awaited<ReturnType<typeof open>>, check: unknown) {
  const add = page.getByRole("button", { name: "新增", exact: true });
  await expect(add).toBeEnabled();
  await add.click();
  await state.received;
  await expect(page.getByRole("button", { name: "新增中…", exact: true })).toBeDisabled();
  expect(state.writes).toEqual([
    { method: "POST", body: { source: "user", text: "新的驗收條件", check } },
  ]);
  state.release();
  await expect(page.locator("#criterion-c-validated")).toHaveValue("新的驗收條件");
  await expect(page.getByRole("status").filter({ hasText: "已新增驗收條件。" })).toHaveText(
    "已新增驗收條件。",
  );
  await expect.poll(state.readCount).toBe(2);
  await expect(page.locator("#new-criterion")).toHaveValue("");
  await expect(page.locator("#check-node-0-value-0")).toHaveCount(0);
}

for (const scenario of [
  { name: "Unicode limit", character: "😀", count: 200, spaces: false },
  { name: "Unicode overflow", character: "😀", count: 201, spaces: false },
  { name: "ASCII overflow", character: "a", count: 201, spaces: false },
  { name: "trimmed Unicode limit", character: "😀", count: 200, spaces: true },
]) {
  for (const width of [375, 1280]) {
    test(`check values retain ${scenario.name} at ${width}px`, async ({ page }, info) => {
      const state = await open(page, width, "contains_values");
      try {
        const input = page.locator("#check-node-0-value-0");
        const content = scenario.character.repeat(scenario.count);
        const value = scenario.spaces ? ` ${content} ` : content;
        await input.click();
        await page.keyboard.press("Control+A");
        await page.keyboard.insertText(value);
        await expect(input).toHaveValue(value);
        const over = scenario.count === 201;
        await expect(input).toHaveAttribute("aria-invalid", String(over));
        const description = await input.getAttribute("aria-describedby");
        expect(description).toBeTruthy();
        const hint = page.locator(`#${description}`);
        await expect(hint).toContainText("最多 200 個字");
        const add = page.getByRole("button", { name: "新增", exact: true });
        if (over) {
          await expect(hint).toContainText("目前 201 個字");
          await expect(add).toBeDisabled();
          expect(state.writes).toEqual([]);
        } else await expect(add).toBeEnabled();
        await input.click();
        await expect(input).toBeFocused();
        await expect(input).toBeInViewport({ ratio: 1 });
        await inspect(page, info);
        let normalized = content;
        if (over) {
          await input.click();
          await page.keyboard.press("Backspace");
          normalized = scenario.character.repeat(200);
          await expect(input).toHaveValue(normalized);
        }
        await expect(input).toHaveAttribute("aria-invalid", "false");
        await submit(page, state, { kind: "contains_values", values: [normalized] });
      } finally {
        state.release();
      }
    });
  }
}

for (const threshold of ["0", "1", "0.5"]) {
  for (const width of [375, 1280]) {
    test(`fan-out validates ${threshold} passes at ${width}px`, async ({ page }, info) => {
      const state = await open(page, width, "each");
      try {
        await page.locator("#check-node-0-value-0").fill("資料");
        const picker = page.locator("#check-node-0-add");
        await picker.selectOption("judge");
        await picker.locator("..").getByRole("button", { name: "加入", exact: true }).click();
        await page
          .locator(".check-editor-outline")
          .getByRole("button", { name: /^Fan-Out：資料(?:（要修正）)?$/ })
          .click();
        const input = page.locator("#check-node-0-at-least");
        await input.fill(threshold);
        await expect(input).toHaveValue(threshold);
        await expect(input).toHaveAttribute("aria-invalid", String(threshold === "0.5"));
        const add = page.getByRole("button", { name: "新增", exact: true });
        if (threshold === "0.5") {
          await expect(add).toBeDisabled();
          const description = await input.getAttribute("aria-describedby");
          expect(description).toBeTruthy();
          await expect(page.locator(`#${description}`)).toContainText("至少通過數必須是整數");
          expect(state.writes).toEqual([]);
        } else await expect(add).toBeEnabled();
        await input.click();
        await expect(input).toBeFocused();
        await expect(input).toBeInViewport({ ratio: 1 });
        await inspect(page, info);
        if (threshold === "0.5") await input.fill("0");
        await expect(input).toHaveAttribute("aria-invalid", "false");
        await submit(page, state, {
          kind: "each",
          values: ["資料"],
          children: [{ kind: "judge" }],
          ...(threshold === "1" ? { at_least: 1 } : {}),
        });
      } finally {
        state.release();
      }
    });
  }
}
