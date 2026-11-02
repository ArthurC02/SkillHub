import { expect, test } from "vitest";
import { renderedApp, type App, type Request } from "../../testing/app";
import { ME } from "../../testing/fixtures/platform";
import type { CreationSession } from "./creation.service";

const app = renderedApp();

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
const CREDITS = {
  balance_credits: 1000,
  debt_floor_credits: -50,
  estimated_session: { low_credits: 30, high_credits: 65, sample_size: 40, estimated: false },
  can_start: true,
};
const FEATURES = { generate_skill: true, creation_skill: true };

function stubFeatures(page: App) {
  Object.defineProperty(window, "EventSource", { value: undefined, configurable: true });
  page.route("**/me", () => ({ json: { ...ME, features: FEATURES } }));
}

function stubCreation(page: App, status = 200) {
  stubFeatures(page);
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let held = status === 200;
  let reads = 0;
  let reply: unknown = SESSION;
  const writes: string[] = [];
  page.route("**/creation-sessions**", async (request) => {
    const path = request.url.pathname;
    if (request.method === "POST") {
      writes.push(path);
      return { json: SESSION };
    }
    if (path.endsWith("/limits")) return { json: LIMITS };
    if (!path.endsWith(ID)) return { json: [SESSION] };
    reads++;
    if (held) await gate;
    return status === 200 ? { json: reply } : { status, json: { error: "unavailable" } };
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

function stubWriteReceipt(page: App, mode: string, damage: string) {
  stubFeatures(page);
  const posts: Record<string, unknown>[] = [];
  const paths: string[] = [];
  let current = SESSION;
  page.route("**/me/credits", () => ({ json: CREDITS }));
  page.route("**/creation-sessions**", (request: Request) => {
    const path = request.url.pathname;
    paths.push(path);
    if (path.endsWith("/events")) return { status: 204 };
    if (path.endsWith("/limits")) return { json: LIMITS };
    if (request.method === "POST") {
      const body = request.body as Record<string, unknown>;
      posts.push(body);
      current = {
        ...SESSION,
        id: mode === "start" ? String(body.id) : ID,
        revision: 8,
        snapshot: {
          ...SESSION.snapshot,
          messages: [...SESSION.snapshot.messages, { role: "user", content: "保留這份送出的補充" }],
        },
      };
      const value =
        posts.length > 1
          ? current
          : damage === "empty"
            ? null
            : damage === "foreign"
              ? { ...current, id: "55555555-5555-4555-8555-555555555555" }
              : { ...current, snapshot: null };
      return { json: value };
    }
    if (path === "/creation-sessions")
      return { json: current.id === ID ? [current] : [current, SESSION] };
    if (path === "/creation-sessions/" + current.id) return { json: current };
    return { status: 404, json: { error: "找不到可讀取的創作或參考內容。" } };
  });
  return {
    posts,
    paths,
    get requestedID() {
      return current.id;
    },
  };
}

function sessionParam(page: App) {
  return new URLSearchParams(page.search()).get("session");
}

for (const [mode, damage] of [
  ["start", "empty"],
  ["start", "foreign"],
  ["start", "snapshot"],
  ["action", "empty"],
  ["action", "foreign"],
  ["action", "snapshot"],
]) {
  test(`an uncertain ${mode} receipt with ${damage} data keeps the editor and retries the same write`, async () => {
    const page = app();
    const state = stubWriteReceipt(page, mode, damage);
    await page.goto(
      mode === "start" ? "/workspace/creations" : `/workspace/creations?session=${ID}`,
    );
    if (mode === "start") {
      await page.getByRole("button", { name: "＋ 開始新的創作", exact: true }).click();
      await page
        .locator(".budget-picker label")
        .filter({ hasText: /^500 點$/ })
        .click();
      await expect(page.getByRole("radio", { name: "500 點", exact: true })).toBeChecked();
    }
    const current = page.locator("#creation-workspace");
    await current.locator("#creation-message").fill("保留這份送出的補充");
    await current
      .getByRole("button", { name: mode === "start" ? "開始創作" : "送出", exact: true })
      .click();
    await expect(current.getByRole("alert")).toContainText("創作結果尚未確認");
    await expect(current.getByRole("alert")).toBeFocused();
    await expect(current.locator("#creation-message")).toHaveValue("保留這份送出的補充");
    expect(page.pathname()).toBe("/workspace/creations");
    expect(sessionParam(page)).toBe(mode === "start" ? null : ID);
    const recovery = current.getByRole("link", { name: "另開創作紀錄核對", exact: true });
    await expect(recovery).toHaveAttribute(
      "href",
      `/workspace/creations?session=${state.requestedID}`,
    );
    await expect(recovery).toHaveAttribute("target", "_blank");
    if (mode === "start")
      await expect(page.getByRole("radio", { name: "500 點", exact: true })).toBeChecked();
    expect(await page.axeViolations("main")).toEqual([]);
    expect(state.posts).toHaveLength(1);
    expect(state.paths).not.toContain("/creation-sessions/55555555-5555-4555-8555-555555555555");
    await current.getByRole("button", { name: "重試", exact: true }).focus();
    await page.press("Enter");
    await page.eventually(() => expect(state.posts).toHaveLength(2));
    expect(state.posts[1]).toEqual(state.posts[0]);
    await expect(current.locator("#creation-message")).toHaveValue("");
    await page.eventually(() => expect(sessionParam(page)).toBe(state.requestedID));
  });
}

test("an explicitly new creation discards an uncertain start identity", async () => {
  const page = app();
  const state = stubWriteReceipt(page, "start", "empty");
  await page.goto("/workspace/creations");
  await page.getByRole("button", { name: "＋ 開始新的創作", exact: true }).click();
  await page
    .locator(".budget-picker label")
    .filter({ hasText: /^500 點$/ })
    .click();
  const input = page.locator("#creation-message");
  await input.fill("保留這份送出的補充");
  await page.getByRole("button", { name: "開始創作", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("創作結果尚未確認");
  expect(state.posts).toHaveLength(1);
  expect(await page.axeViolations("main")).toEqual([]);
  await page.getByRole("button", { name: /創作清單 ·/ }).click();
  await page.getByRole("button", { name: "＋ 開始新的創作", exact: true }).click();
  await expect(input).toHaveValue("");
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.getByRole("radio", { name: "500 點", exact: true })).toBeChecked();
  expect(state.posts).toHaveLength(1);
  await input.fill("保留這份送出的補充");
  await page.getByRole("button", { name: "開始創作", exact: true }).click();
  await page.eventually(() => expect(state.posts).toHaveLength(2));
  await expect(input).toHaveValue("");
  expect(state.posts[1].id).not.toBe(state.posts[0].id);
  expect({ ...state.posts[1], id: state.posts[0].id }).toEqual(state.posts[0]);
  await page.eventually(() => expect(sessionParam(page)).toBe(state.posts[1].id));
});

for (const [name, reply] of [
  ["empty reply", null],
  ["another session", { ...SESSION, id: "55555555-5555-4555-8555-555555555555" }],
  ["missing snapshot", { ...SESSION, snapshot: null }],
  ["missing messages", { ...SESSION, snapshot: { ...SESSION.snapshot, messages: null } }],
] as const) {
  test(`an invalid creation ${name} offers keyboard GET recovery without a new paid session`, async () => {
    const page = app();
    const state = stubCreation(page);
    const errors: string[] = [];
    const onError = (event: ErrorEvent) => errors.push(event.message);
    window.addEventListener("error", onError);
    try {
      state.invalid(reply);
      await page.goto(`/workspace/creations?session=${ID}`);
      const current = page.locator("#creation-workspace");
      await expect(current.getByRole("alert")).toContainText("暫時無法讀取創作紀錄");
      await expect(current.locator(".composer, .starter-cards, .budget-picker")).toHaveCount(0);
      expect(await page.axeViolations("main")).toEqual([]);
      state.recover();
      await current.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      await expect(current.locator("#creation-message")).toBeEnabled();
      await expect(current).toContainText("請保留這場創作的需求");
      await expect(current).toBeFocused();
      expect(sessionParam(page)).toBe(ID);
      expect(state.reads).toBe(2);
      expect(state.writes).toEqual([]);
      expect(errors).toEqual([]);
    } finally {
      window.removeEventListener("error", onError);
    }
  });
}

test("a selected creation waits for its first reply without offering a new paid session", async () => {
  const page = app();
  const state = stubCreation(page);
  try {
    await page.goto(`/workspace/creations?session=${ID}`);
    await page.eventually(() => expect(state.reads).toBe(1));
    const current = page.locator("#creation-workspace");
    await expect(current.getByRole("status")).toContainText("載入創作紀錄中");
    await expect(current.locator(".composer, .starter-cards, .budget-picker")).toHaveCount(0);
    expect(await page.axeViolations("main")).toEqual([]);
    state.recover();
    await expect(current).toContainText("請保留這場創作的需求");
    await expect(current.locator("#creation-message")).toBeEnabled();
    await expect(current.locator(".budget-picker")).toHaveCount(0);
    expect(state.reads).toBe(1);
    expect(state.writes).toEqual([]);
  } finally {
    state.release();
  }
});

for (const status of [404, 503]) {
  test(`a selected creation refused with ${status} resumes its own read after connectivity returns`, async () => {
    const page = app();
    const state = stubCreation(page, status);
    try {
      await page.goto(`/workspace/creations?session=${ID}`);
      const current = page.locator("#creation-workspace");
      await expect(current.getByRole("alert")).toContainText(
        status === 404
          ? "找不到這次創作：它可能已超過保存期限或已被刪除。"
          : "暫時無法讀取創作紀錄",
      );
      await expect(current.locator(".composer, .starter-cards, .budget-picker")).toHaveCount(0);
      const retry = current.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await page.setOffline(true);
      await page.press("Enter");
      await expect(current.getByRole("status")).toHaveText(
        "等待連線以讀取創作紀錄；恢復連線後會繼續讀取。",
      );
      await expect(current.getByRole("alert")).toHaveCount(0);
      await expect(current.locator(".composer, .starter-cards, .loading-skeleton")).toHaveCount(0);
      await expect(current).toBeFocused();
      expect(await page.axeViolations("main")).toEqual([]);
      expect(state.reads).toBe(1);
      expect(state.writes).toEqual([]);
      state.recover();
      await page.setOffline(false);
      await expect(current).toContainText("請保留這場創作的需求");
      await expect(current.locator("#creation-message")).toBeEnabled();
      await expect(current.locator(".budget-picker")).toHaveCount(0);
      expect(sessionParam(page)).toBe(ID);
      expect(state.reads).toBe(2);
      expect(state.writes).toEqual([]);
    } finally {
      state.release();
      await page.setOffline(false);
    }
  });
}

test("choosing an unread creation offline keeps the selection instead of opening a new session", async () => {
  const page = app();
  const state = stubCreation(page);
  try {
    await page.goto("/workspace/creations");
    const selected = page.locator(`.creation-session-item[data-session="${ID}"]`);
    await expect(selected).toBeVisible();
    await page.setOffline(true);
    await selected.click();
    const current = page.locator("#creation-workspace");
    await expect(current.getByRole("status")).toContainText("等待連線以讀取創作紀錄");
    await expect(current.locator(".composer, .starter-cards, .loading-skeleton")).toHaveCount(0);
    expect(await page.axeViolations("main")).toEqual([]);
    expect(state.reads).toBe(0);
    expect(state.writes).toEqual([]);
    state.recover();
    await page.setOffline(false);
    await expect(current).toContainText("請保留這場創作的需求");
    expect(state.reads).toBe(1);
    expect(state.writes).toEqual([]);
  } finally {
    state.release();
    await page.setOffline(false);
  }
});

test("a cached creation hides its old read failure during an offline retry and keeps its draft", async () => {
  const page = app();
  const state = stubCreation(page);
  state.recover();
  try {
    await page.goto(`/workspace/creations?session=${ID}`);
    await expect(page.locator("#creation-message")).toBeEnabled();
    state.fail();
    await page.getByRole("link", { name: "資產庫", exact: true }).click();
    await page.eventually(() => expect(page.pathname()).toBe("/library"));
    await expect(page.locator("h1")).toHaveText("資產庫");
    await page.goBack();
    const current = page.locator("#creation-workspace");
    await expect(current.getByRole("alert")).toContainText("暫時無法讀取創作紀錄");
    await current.locator("#creation-message").fill("尚未送出的補充");
    await page.setOffline(true);
    const retry = current.getByRole("button", { name: "再試一次", exact: true });
    await retry.focus();
    await page.press("Enter");
    await expect(current.getByRole("status")).toHaveText("等待連線以重新讀取創作紀錄…");
    await expect(current.getByRole("alert")).toHaveCount(0);
    await expect(current.locator("#creation-message")).toHaveValue("尚未送出的補充");
    await expect(current).toBeFocused();
    expect(state.reads).toBe(2);
    expect(state.writes).toEqual([]);
    const transcript = current.getByRole("region", { name: "創作對話與決策", exact: true });
    await transcript.focus();
    await expect(transcript).toBeFocused();
    expect(await page.axeViolations("main")).toEqual([]);
    state.recover();
    await page.setOffline(false);
    await expect(current.getByRole("status")).toHaveCount(0);
    await expect(current.locator("#creation-message")).toHaveValue("尚未送出的補充");
    expect(state.reads).toBe(3);
    expect(state.writes).toEqual([]);
  } finally {
    state.release();
    await page.setOffline(false);
  }
});
