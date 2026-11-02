import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RUN, TRACE_GENERAL } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const NEXT = "5c2f8e10-0b7a-4d63-9a41-7e3d2b1c9f44";
const ASKING = {
  ...TRACE_GENERAL,
  status: "succeeded",
  questions: [{ question: "要處理哪個工作表？", why: "確認來源", options: ["Sheet1", "Sheet2"] }],
};
const form = (page: App) => page.getByRole("region", { name: "這次試跑需要你回答", exact: true });

async function fillAnswer(page: App) {
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await form(page).getByRole("radio", { name: "Sheet2", exact: true }).check();
}

async function focusedFailure(page: App) {
  const failure = form(page).getByRole("alert", { name: "回答送出結果", exact: true });
  await expect(failure).toContainText("無法確認回答是否已送出並開始下一次試跑");
  await expect(failure).toBeFocused();
  return failure;
}

for (const outcome of ["network", "server", "malformed", "incomplete", "same Run"] as const) {
  test(`${outcome} continuation outcome preserves answers and uncertainty until a deliberate resend`, async () => {
    const page = app();
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: unknown[] = [];
    page.route(`**/runs/${RUN}/trace*`, async ({ url }) => {
      if (url.searchParams.get("mode") === "advanced") return undefined;
      reads += 1;
      if (reads > 1) await gate;
      if (reads === 2 && outcome === "server")
        return { status: 503, json: { error: "service unavailable" } };
      return { json: ASKING };
    });
    page.route(`**/runs/${RUN}/continuations`, ({ body }) => {
      writes.push(body);
      if (writes.length > 1) return { status: 201, json: { run_id: NEXT } };
      if (outcome === "network") return "network-error";
      if (outcome === "server") return { status: 503, json: { error: "service unavailable" } };
      if (outcome === "malformed")
        return { status: 201, contentType: "application/json", body: "{" };
      return { status: 201, json: outcome === "same Run" ? { run_id: RUN } : {} };
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await fillAnswer(page);
      await form(page).getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
      const failure = await focusedFailure(page);
      await page.eventually(() => expect(reads).toBe(2));
      await expect(form(page).getByRole("radio", { name: "Sheet2", exact: true })).toBeChecked();
      const answer = form(page).getByRole("textbox", { name: "你的回答", exact: true });
      await expect(answer).toBeEditable();
      const resend = form(page).getByRole("button", {
        name: "核對後仍要重新送出回答",
        exact: true,
      });
      await expect(resend).toBeDisabled();
      const history = failure.getByRole("link", { name: "另開試跑紀錄", exact: true });
      await expect(history).toHaveAttribute("href", "/workspace/runs");
      await expect(history).toHaveAttribute("target", "_blank");
      await answer.fill("Sheet3");
      await expect(failure).toContainText("原始問題仍然顯示，不代表尚未收到回答");
      await expect(answer).toBeFocused();
      await page.getByRole("button", { name: "進階模式", exact: true }).click();
      await expect(answer).toHaveValue("Sheet3");
      await expect(failure).toBeVisible();
      release();
      if (outcome === "server") {
        await expect(form(page).getByRole("status")).toContainText("原始問題尚未確認");
        await expect(resend).toBeDisabled();
        await expect(answer).toHaveValue("Sheet3");
        await expect(failure).toBeVisible();
        await form(page).getByRole("button", { name: "重新確認問題", exact: true }).click();
      }
      await expect(resend).toBeEnabled();
      expect(page.pathname()).toBe(`/runs/${RUN}`);
      expect(writes).toEqual([{ answers: ["Sheet2"] }]);
      expect(await page.axeViolations("body")).toEqual([]);
      await resend.focus();
      await page.press("Enter");
      await page.eventually(() => expect(page.pathname()).toBe(`/runs/${NEXT}`));
      await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toBeVisible();
      expect(writes).toEqual([{ answers: ["Sheet2"] }, { answers: ["Sheet3"] }]);
    } finally {
      release();
    }
  });
}

test("a confirmed continuation navigates before the original Trace reread finishes", async () => {
  const page = app();
  let reads = 0;
  let posts = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route(`**/runs/${RUN}/trace`, async () => {
    reads += 1;
    if (reads > 1) await gate;
    return { json: ASKING };
  });
  page.route(`**/runs/${RUN}/continuations`, () => {
    posts += 1;
    return { status: 201, json: { run_id: NEXT } };
  });
  try {
    await page.goto(`/runs/${RUN}`);
    await fillAnswer(page);
    await form(page).getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
    await page.eventually(() => expect(page.pathname()).toBe(`/runs/${NEXT}`));
    await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toBeVisible();
    await page.eventually(() => expect(reads).toBe(2));
    expect(posts).toBe(1);
  } finally {
    release();
  }
});
