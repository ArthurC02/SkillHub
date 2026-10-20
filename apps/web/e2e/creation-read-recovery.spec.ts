import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import type { CreationSession } from "../src/features/creation/creation.service";
import { ME } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

const ID = "44444444-4444-4444-8444-444444444444";
const SESSION: CreationSession = {
  id: ID,
  revision: 7,
  state: "waiting_input",
  snapshot: {
    messages: Array.from({ length: 12 }, (_, index) => ({
      role: "assistant",
      content: `第 ${index + 1} 段需求：請保留這場創作的需求。\n\n整理採購文件時，請分別列出品項、數量、交付日期與待確認的條件。`,
    })),
    brief: "整理採購文件",
    brief_confirmed: false,
    acceptance_criteria: [],
    diagram_understanding: "",
    diagram_confirmed: false,
    references: [],
    pending_action: "",
    budget_credits: 500,
    reserved_credits: 0,
    spent_credits: 10,
    usage_unknown: false,
    steps: 1,
    tool_calls: 0,
  },
  created_at: "2026-09-29T07:00:00Z",
  updated_at: "2026-09-29T08:00:00Z",
  expires_at: "2099-09-30T08:00:00Z",
  deadline: "2099-09-29T09:00:00Z",
};

async function stubCreation(page: Page, status = 200) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.addInitScript(() =>
    Object.defineProperty(window, "EventSource", { value: undefined, configurable: true }),
  );
  await page.route("**/me", (route) =>
    route.fulfill({ json: { ...ME, features: { generate_skill: true, creation_skill: true } } }),
  );
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let held = status === 200;
  let reads = 0;
  let reply: unknown = SESSION;
  const writes: string[] = [];
  await page.route("**/creation-sessions**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "POST") {
      writes.push(path);
      return route.fulfill({ json: SESSION });
    }
    if (path.endsWith("/limits"))
      return route.fulfill({
        json: {
          min_budget_credits: 130,
          max_budget_credits: 6500,
          max_steps: 20,
          max_tool_calls: 10,
          call_timeout_seconds: 120,
          session_timeout_seconds: 3600,
          retention_seconds: 604800,
        },
      });
    if (!path.endsWith(ID)) return route.fulfill({ json: [SESSION] });
    reads++;
    if (held) await gate;
    return status === 200
      ? route.fulfill({ json: reply })
      : route.fulfill({ status, json: { error: "unavailable" } });
  });
  return {
    writes,
    get reads() {
      return reads;
    },
    release,
    fail: () => {
      status = 503;
      held = false;
    },
    recover: () => {
      status = 200;
      reply = SESSION;
      held = false;
      release();
    },
    invalid: (value: unknown) => {
      status = 200;
      held = false;
      reply = value;
    },
  };
}

for (const [name, reply] of [
  ["empty reply", null],
  ["another session", { ...SESSION, id: "55555555-5555-4555-8555-555555555555" }],
  ["missing snapshot", { ...SESSION, snapshot: null }],
  ["missing messages", { ...SESSION, snapshot: { ...SESSION.snapshot, messages: null } }],
] as const) {
  test(`an invalid creation ${name} offers keyboard GET recovery without a new paid session`, async ({
    page,
  }) => {
    const state = await stubCreation(page);
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    state.invalid(reply);
    await page.goto(`/workspace/creations?session=${ID}`);
    const current = page.locator("#creation-workspace");
    await expect(current.getByRole("alert")).toContainText("暫時無法讀取創作紀錄");
    await expect(current.locator(".composer, .starter-cards, .budget-picker")).toHaveCount(0);
    await captureReadState(page);
    state.recover();
    await current.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(current.locator("#creation-message")).toBeEnabled();
    await expect(current).toContainText("請保留這場創作的需求");
    await expect(current).toBeFocused();
    await expect(page).toHaveURL(new RegExp(`session=${ID}$`));
    expect(state.reads).toBe(2);
    expect(state.writes).toEqual([]);
    expect(errors).toEqual([]);
  });
}

async function captureReadState(page: Page) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath("creation-read-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  for (const review of report.incomplete) {
    expect(review.id).toBe("color-contrast");
    for (const node of review.nodes) {
      expect(node.target).toHaveLength(1);
      expect(node.target[0]).toMatch(
        /^li\[data-index="\d+"\] > \.agent-avatar\[aria-hidden="true"\]$/,
      );
      expect(node.any.length).toBeGreaterThan(0);
      expect(node.any.every((check) => check.data?.messageKey === "nonBmp")).toBe(true);
      const avatar = page.locator(node.target[0] as string);
      await expect(avatar).toHaveAttribute("aria-hidden", "true");
      await expect(avatar.locator("..").locator(".creation-who")).toHaveText("Agent");
    }
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("creation-read-phone.png") });
}

test("a selected creation waits for its first reply without offering a new paid session", async ({
  page,
}) => {
  const state = await stubCreation(page);
  try {
    await page.goto(`/workspace/creations?session=${ID}`);
    await expect.poll(() => state.reads).toBe(1);
    const current = page.locator("#creation-workspace");
    await expect(current.getByRole("status")).toContainText("載入創作紀錄中");
    await expect(current.locator(".composer, .starter-cards, .budget-picker")).toHaveCount(0);
    await captureReadState(page);
    state.recover();
    await expect(current).toContainText("請保留這場創作的需求");
    await expect(current.locator("#creation-message")).toBeEnabled();
    await expect(current.locator(".budget-picker")).toHaveCount(0);
    const stream = current.locator(".creation-stream");
    await expect
      .poll(() => stream.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    await expect(current.locator('.creation-log > li[data-index="11"]')).toBeInViewport();
    const previousScrollTop = await stream.evaluate((element) => element.scrollTop);
    await stream.hover();
    await page.mouse.wheel(0, -2000);
    await expect
      .poll(() => stream.evaluate((element) => element.scrollTop))
      .toBeLessThan(previousScrollTop);
    await stream.focus();
    await page.keyboard.press("Home");
    await expect.poll(() => stream.evaluate((element) => element.scrollTop)).toBe(0);
    const latest = current.getByRole("button", { name: "↓ 回到最新", exact: true });
    await expect(latest).toBeVisible();
    await latest.focus();
    await page.keyboard.press("Enter");
    await expect(latest).toHaveCount(0);
    await expect(current.locator('.creation-log > li[data-index="11"]')).toBeInViewport();
    expect(state.reads).toBe(1);
    expect(state.writes).toEqual([]);
  } finally {
    state.release();
  }
});

for (const status of [404, 503]) {
  test(`a selected creation refused with ${status} resumes its own read after connectivity returns`, async ({
    page,
  }) => {
    const state = await stubCreation(page, status);
    await page.emulateMedia({ colorScheme: "dark" });
    try {
      await page.goto(`/workspace/creations?session=${ID}`);
      const current = page.locator("#creation-workspace");
      await expect(current.getByRole("alert")).toContainText("暫時無法讀取創作紀錄");
      await expect(current.locator(".composer, .starter-cards, .budget-picker")).toHaveCount(0);
      const retry = current.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await setOffline(page, true);
      await page.keyboard.press("Enter");
      await expect(current.getByRole("status")).toHaveText(
        "等待連線以讀取創作紀錄；恢復連線後會繼續讀取。",
      );
      await expect(current.getByRole("alert")).toHaveCount(0);
      await expect(current.locator(".composer, .starter-cards, .loading-skeleton")).toHaveCount(0);
      await expect(current).toBeFocused();
      await captureReadState(page);
      expect(state.reads).toBe(1);
      expect(state.writes).toEqual([]);
      state.recover();
      await setOffline(page, false);
      await expect(current).toContainText("請保留這場創作的需求");
      await expect(current.locator("#creation-message")).toBeEnabled();
      await expect(current.locator(".budget-picker")).toHaveCount(0);
      await expect(page).toHaveURL(new RegExp(`session=${ID}$`));
      expect(state.reads).toBe(2);
      expect(state.writes).toEqual([]);
    } finally {
      state.release();
      await setOffline(page, false);
    }
  });
}

test("choosing an unread creation offline keeps the selection instead of opening a new session", async ({
  page,
}) => {
  const state = await stubCreation(page);
  try {
    await page.goto("/workspace/creations");
    const selected = page.locator(`.creation-session-item[data-session="${ID}"]`);
    await expect(selected).toBeVisible();
    await setOffline(page, true);
    await selected.click();
    const current = page.locator("#creation-workspace");
    await expect(current.getByRole("status")).toContainText("等待連線以讀取創作紀錄");
    await expect(current.locator(".composer, .starter-cards, .loading-skeleton")).toHaveCount(0);
    await captureReadState(page);
    expect(state.reads).toBe(0);
    expect(state.writes).toEqual([]);
    state.recover();
    await setOffline(page, false);
    await expect(current).toContainText("請保留這場創作的需求");
    expect(state.reads).toBe(1);
    expect(state.writes).toEqual([]);
  } finally {
    state.release();
    await setOffline(page, false);
  }
});

test("a cached creation hides its old read failure during an offline retry and keeps its draft", async ({
  page,
}) => {
  const state = await stubCreation(page);
  state.recover();
  try {
    await page.goto(`/workspace/creations?session=${ID}`);
    await expect(page.locator("#creation-message")).toBeEnabled();
    state.fail();
    await page.getByRole("link", { name: "資產庫", exact: true }).click();
    await expect(page).toHaveURL(/\/library$/);
    await expect(
      page.getByRole("heading", { level: 1, name: "資產庫", exact: true }),
    ).toBeVisible();
    await page.goBack();
    const current = page.locator("#creation-workspace");
    await expect(current.getByRole("alert")).toContainText("暫時無法讀取創作紀錄");
    await current.locator("#creation-message").fill("尚未送出的補充");
    await setOffline(page, true);
    const retry = current.getByRole("button", { name: "再試一次", exact: true });
    await retry.focus();
    await page.keyboard.press("Enter");
    await expect(current.getByRole("status")).toHaveText("等待連線以重新讀取創作紀錄…");
    await expect(current.getByRole("alert")).toHaveCount(0);
    await expect(current.locator("#creation-message")).toHaveValue("尚未送出的補充");
    await expect(current).toBeFocused();
    expect(state.reads).toBe(2);
    expect(state.writes).toEqual([]);
    const transcript = current.getByRole("region", { name: "創作對話與決策", exact: true });
    await transcript.focus();
    await expect(transcript).toBeFocused();
    await page.keyboard.press("Home");
    await expect.poll(() => transcript.evaluate((element) => element.scrollTop)).toBe(0);
    await captureReadState(page);
    state.recover();
    await setOffline(page, false);
    await expect(current.getByRole("status")).toHaveCount(0);
    await expect(current.locator("#creation-message")).toHaveValue("尚未送出的補充");
    expect(state.reads).toBe(3);
    expect(state.writes).toEqual([]);
  } finally {
    state.release();
    await setOffline(page, false);
  }
});
