import AxeBuilder from "./axe";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import type { CreationSession } from "../src/features/creation/creation.service";
import { ME } from "../src/testing/fixtures/platform";
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
const LIMITS = {
  min_budget_credits: 130,
  max_budget_credits: 6500,
  max_steps: 20,
  max_tool_calls: 10,
  call_timeout_seconds: 120,
  session_timeout_seconds: 3600,
  retention_seconds: 604800,
};
async function stubCreation(page: Page, status = 200, events = false) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  if (!events)
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
    if (path.endsWith("/limits")) return route.fulfill({ json: LIMITS });
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

async function captureReadState(page: Page) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath("creation-read-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  for (const review of report.incomplete) {
    expect(review.id).toBe("color-contrast");
    for (const node of review.nodes) {
      expect(node.target).toHaveLength(1);
      expect(node.target[0]).toMatch(
        /^li\[(?:data-index="\d+"|data-role="assistant")\] > \.agent-avatar\[aria-hidden="true"\]$/,
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

test("a delayed creation GET keeps newer stream progress and unsent input", async ({ page }) => {
  const state = await stubCreation(page, 200, true);
  let frames = 0;
  const newer = {
    ...SESSION,
    revision: 8,
    snapshot: { ...SESSION.snapshot, brief: "串流已確認的最新需求" },
  };
  await page.route("**/events", (route) => {
    frames++;
    return frames === 1
      ? route.fulfill({
          headers: { "Content-Type": "text/event-stream" },
          body: "data: " + JSON.stringify(newer) + "\n\n",
        })
      : route.fulfill({ status: 204 });
  });
  try {
    await page.goto(`/workspace/creations?session=${ID}`);
    const current = page.locator("#creation-workspace");
    await expect.poll(() => state.reads).toBe(1);
    await expect(current).toContainText("串流已確認的最新需求");
    await expect(current.getByRole("status")).toContainText("正在重新讀取創作紀錄");
    await current.locator("#creation-message").fill("請保留尚未送出的補充");
    const finished = page.waitForResponse(
      (reply) => new URL(reply.url()).pathname === "/creation-sessions/" + ID,
    );
    state.recover();
    await (await finished).finished();
    await expect(current.getByRole("status")).toHaveCount(0);
    await expect(current).toContainText("串流已確認的最新需求");
    await expect(current.locator("#creation-message")).toHaveValue("請保留尚未送出的補充");
    await expect(page).toHaveURL(new RegExp(`session=${ID}$`));
    expect(state.reads).toBe(1);
    expect(state.writes).toEqual([]);
    await captureReadState(page);
  } finally {
    state.release();
  }
});

test("a long creation transcript keeps the latest message in view until the reader scrolls away", async ({
  page,
}) => {
  const state = await stubCreation(page);
  state.recover();
  await page.goto(`/workspace/creations?session=${ID}`);
  const current = page.locator("#creation-workspace");
  await expect(current).toContainText("請保留這場創作的需求");
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
});
