import { expect, test, vi } from "vitest";
import { TEST_CASE, TEST_CASE_DRAFT } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

vi.mock("./components/CheckFlow", () => ({ CheckFlow: () => null }));

const app = renderedApp();

async function open(page: App, kind: string) {
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
  page.route(`**/test-cases/${TEST_CASE}`, () => {
    reads++;
    return { json: draft };
  });
  page.route(`**/test-cases/${TEST_CASE}/criteria`, async ({ method, body }) => {
    writes.push({ method, body });
    started();
    await held;
    draft.acceptance_criteria.push({
      ...draft.acceptance_criteria[0],
      id: "c-validated",
      text: "新的驗收條件",
      source: "user",
      confirmed_at: null,
    });
    return { status: 201, json: draft };
  });
  await page.goto(`/lab/test-cases/${TEST_CASE}`);
  await page.locator("#new-criterion").fill("新的驗收條件");
  await page.locator("#check-editor-root").select(kind);
  await page.getByRole("button", { name: "開始設計", exact: true }).click();
  return { writes, received, release, readCount: () => reads };
}

async function submit(page: App, state: Awaited<ReturnType<typeof open>>, check: unknown) {
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
  await page.eventually(() => expect(state.readCount()).toBe(2));
  await expect(page.locator("#new-criterion")).toHaveValue("");
  await expect(page.locator("#check-node-0-value-0")).toHaveCount(0);
}

for (const scenario of [
  { name: "Unicode limit", character: "😀", count: 200, spaces: false },
  { name: "Unicode overflow", character: "😀", count: 201, spaces: false },
  { name: "ASCII overflow", character: "a", count: 201, spaces: false },
  { name: "trimmed Unicode limit", character: "😀", count: 200, spaces: true },
]) {
  test(`check values retain ${scenario.name}`, async () => {
    const page = app();
    const state = await open(page, "contains_values");
    try {
      const input = page.locator("#check-node-0-value-0");
      const content = scenario.character.repeat(scenario.count);
      const value = scenario.spaces ? ` ${content} ` : content;
      await input.fill(value);
      await expect(input).toHaveValue(value);
      const over = scenario.count === 201;
      await expect(input).toHaveAttribute("aria-invalid", String(over));
      await expect(input).toHaveAccessibleDescription(/最多 200 個字/);
      const add = page.getByRole("button", { name: "新增", exact: true });
      if (over) {
        await expect(input).toHaveAccessibleDescription(/目前 201 個字/);
        await expect(add).toBeDisabled();
        expect(state.writes).toEqual([]);
      } else await expect(add).toBeEnabled();
      await input.click();
      await expect(input).toBeFocused();
      expect(await page.axeViolations(".check-editor-panel")).toEqual([]);
      let normalized = content;
      if (over) {
        normalized = scenario.character.repeat(200);
        await input.fill(normalized);
        await expect(input).toHaveValue(normalized);
      }
      await expect(input).toHaveAttribute("aria-invalid", "false");
      await submit(page, state, { kind: "contains_values", values: [normalized] });
    } finally {
      state.release();
    }
  });
}

for (const threshold of ["0", "1", "0.5"]) {
  test(`fan-out validates ${threshold} passes`, async () => {
    const page = app();
    const state = await open(page, "each");
    try {
      await page.locator("#check-node-0-value-0").fill("資料");
      await page.locator("#check-node-0-add").select("judge");
      await page.getByRole("button", { name: "加入", exact: true }).click();
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
        await expect(input).toHaveAccessibleDescription(/至少通過數必須是整數/);
        expect(state.writes).toEqual([]);
      } else await expect(add).toBeEnabled();
      await input.click();
      await expect(input).toBeFocused();
      expect(await page.axeViolations(".check-editor-panel")).toEqual([]);
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
