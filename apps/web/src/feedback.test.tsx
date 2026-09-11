import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { FEEDBACK_MAX_MESSAGE, feedbackPagePath, feedbackRunID } from "./api/feedback";
import { FeedbackEntry } from "./components/FeedbackEntry";

const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function render(node: ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
}

async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

function setValue(input: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
  setter.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

async function type(value: string) {
  await act(async () => setValue(container.querySelector("textarea")!, value));
}

async function submit() {
  await act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

const text = () => container.textContent ?? "";

const FEEDBACK_400_BODY = "message 不能空白，且最多 2000 字";
const FEEDBACK_500_BODY = "回報沒有記錄成功，可以再送一次";
const NOT_AUTHENTICATED_BODY = "not authenticated";

function stubPlatform(status = 204) {
  const calls: Array<{ url: string; body: unknown }> = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/me")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            user_id: "u-1",
            email: "tester@example.com",
            display_name: "tester",
            workspace_id: "ws-1",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    }
    calls.push({ url: String(input), body: JSON.parse(String(init?.body ?? "null")) });
    if (status === 204) return Promise.resolve(new Response(null, { status: 204 }));
    const message =
      status === 401
        ? NOT_AUTHENTICATED_BODY
        : status === 500
          ? FEEDBACK_500_BODY
          : FEEDBACK_400_BODY;
    return Promise.resolve(
      new Response(JSON.stringify({ error: message }), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  return calls;
}

test("BETA-004 the page path travels without its query string, and a run id only when the address names one", () => {
  expect(feedbackPagePath("/?q=我的客戶名單")).toBe("/");
  expect(feedbackPagePath("/skills/abc#risk")).toBe("/skills/abc");
  expect(feedbackPagePath("/lab/run")).toBe("/lab/run");

  expect(feedbackRunID(`/runs/${RUN}`)).toBe(RUN);
  expect(feedbackRunID(`/runs/${RUN}/compare`)).toBe(RUN);
  expect(feedbackRunID("/workspace/downloads")).toBeUndefined();
  expect(feedbackRunID("/runs/latest")).toBeUndefined();
});

test("BETA-003 a report carries only what the reporter can see on screen", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname={`/runs/${RUN}?tab=advanced`} />);

  expect(text()).toContain(`/runs/${RUN}`);
  expect(text()).not.toContain("tab=advanced");
  expect(text()).toContain("沒有截圖");

  await type("按了建立下載套件之後畫面沒有任何反應。");
  await submit();
  await waitFor(() => calls.length > 0);

  expect(calls[0].url).toContain("/feedback");
  expect(calls[0].body).toEqual({
    kind: "blocking_issue",
    message: "按了建立下載套件之後畫面沒有任何反應。",
    page_path: `/runs/${RUN}`,
    run_id: RUN,
    build_id: import.meta.env.VITE_BUILD_ID,
  });
  expect(calls[0].body).toHaveProperty("build_id", expect.any(String));
  await waitFor(() => text().includes("已收到"));
  expect(text()).toContain("沒有回覆機制");
});

test("NFR-007 a blank report is refused with a sentence, not with a dead button", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname="/" />);

  const send = container.querySelector("button[type=submit]") as HTMLButtonElement;
  expect(send.disabled).toBe(false);

  await type("   ");
  await submit();

  expect(calls).toHaveLength(0);
  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("內容不能空白");
  expect((container.querySelector("textarea") as HTMLTextAreaElement).value).toBe("   ");
});

test("BETA-003 an over-long report says how long it is instead of being cut in half", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname="/" />);

  await type("字".repeat(FEEDBACK_MAX_MESSAGE + 5));
  await submit();

  expect(calls).toHaveLength(0);
  expect(container.querySelector('[role="alert"]')?.textContent).toContain(
    String(FEEDBACK_MAX_MESSAGE + 5),
  );
});

test("a report at exactly the length ceiling is accepted, not refused", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname="/" />);

  await type("字".repeat(FEEDBACK_MAX_MESSAGE));
  await submit();
  await waitFor(() => calls.length > 0);

  expect(container.querySelector('[role="alert"]')).toBeNull();
});

test("BETA-004 a failed submit keeps the words and says what to do next", async () => {
  stubPlatform(400);
  await render(<FeedbackEntry pathname="/" />);

  await type("Fork 之後找不到我 Fork 出來的東西。");
  await submit();
  await waitFor(() => text().includes("送不出去"));

  expect((container.querySelector("textarea") as HTMLTextAreaElement).value).toBe(
    "Fork 之後找不到我 Fork 出來的東西。",
  );
  expect(text()).toContain("可以稍後再按一次");
  expect(text()).toContain("目前沒有第二條回報管道");
  expect(text()).not.toContain("寫信");
  expect(text()).not.toContain("已收到");
});

test("丙-150 a session that expires mid-typing shows 需要登入, not the server's raw body", async () => {
  stubPlatform(401);
  await render(<FeedbackEntry pathname="/" />);

  await type("Fork 之後找不到我 Fork 出來的東西。");
  await submit();
  await waitFor(() => text().includes("需要登入"));

  expect(text()).not.toContain(NOT_AUTHENTICATED_BODY);
});

test("BETA-005 the two kinds are the reporter's choice and the need signal is one of them", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname="/workspace/downloads" />);

  const needSignal = Array.from(container.querySelectorAll("input[type=radio]")).find(
    (r) => (r as HTMLInputElement).value === "need_signal",
  ) as HTMLInputElement;
  await act(async () => needSignal.click());
  await type("想把套件直接推到 GitHub，不用自己下載再上傳。");
  await submit();
  await waitFor(() => calls.length > 0);

  expect(calls[0].body).toEqual({
    kind: "need_signal",
    message: "想把套件直接推到 GitHub，不用自己下載再上傳。",
    page_path: "/workspace/downloads",
    run_id: undefined,
    build_id: import.meta.env.VITE_BUILD_ID,
  });
});

test("BETA-003 the counter counts what the server counts, so an emoji report is not refused at half length", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname="/" />);

  const emoji = "🙂".repeat(1500);
  await type(emoji);
  expect(text()).toContain(`1500／${FEEDBACK_MAX_MESSAGE} 字`);

  await submit();
  await waitFor(() => calls.length > 0);
  expect(calls[0].body).toMatchObject({ message: emoji });
});
