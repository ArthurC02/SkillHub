import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { FEEDBACK_MAX_MESSAGE, feedbackPagePath, feedbackRunID } from "./api/feedback";
import { FeedbackEntry } from "./components/FeedbackEntry";

// 03:BETA-003/004 — the POST /feedback entry point. Same hand-rolled DOM
// plumbing as packaging.test.tsx: @testing-library is not a dependency and these
// assertions do not justify adding one.

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

/** Records every call so a test can assert nothing was sent as well as what was. */
function stubPlatform(status = 204) {
  const calls: Array<{ url: string; body: unknown }> = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    // The session read is not what this file measures. `FeedbackEntry` reads
    // `/me` so it can say 「回報問題需要登入。」 before somebody writes a paragraph
    // (資訊架構 §5 IA-6); `expect(calls).toHaveLength(0)` below means 「沒有送出
    // 任何一份回報」, which is a claim about POST /feedback and not about every
    // request the component makes.
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
    return Promise.resolve(
      status === 204
        ? new Response(null, { status: 204 })
        : new Response(JSON.stringify({ error: "message must not be blank" }), {
            status,
            headers: { "Content-Type": "application/json" },
          }),
    );
  });
  return calls;
}

// --- the two path helpers ---------------------------------------------------

test("BETA-004 the page path travels without its query string, and a run id only when the address names one", () => {
  // beta-design §4.2 界線 2: a search query can carry personal data, and this
  // channel is not where it belongs.
  expect(feedbackPagePath("/?q=我的客戶名單")).toBe("/");
  expect(feedbackPagePath("/skills/abc#risk")).toBe("/skills/abc");
  expect(feedbackPagePath("/lab/run")).toBe("/lab/run");

  expect(feedbackRunID(`/runs/${RUN}`)).toBe(RUN);
  expect(feedbackRunID(`/runs/${RUN}/compare`)).toBe(RUN);
  expect(feedbackRunID("/workspace/downloads")).toBeUndefined();
  // Not a uuid-shaped segment: dropped rather than sent as one.
  expect(feedbackRunID("/runs/latest")).toBeUndefined();
});

// --- the form ---------------------------------------------------------------

test("BETA-003 a report carries only what the reporter can see on screen", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname={`/runs/${RUN}?tab=advanced`} />);

  // The context is stated before anything is sent: the path without its query
  // string, the run being looked at, and the promise that nothing else is taken.
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
  });
  await waitFor(() => text().includes("已收到"));
  // 204 carries no id, so the confirmation must not imply a ticket to look up.
  expect(text()).toContain("沒有回覆機制");
});

test("NFR-007 a blank report is refused with a sentence, not with a dead button", async () => {
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname="/" />);

  // The submit button is never disabled for a validation reason: a disabled
  // control with no stated cause reads as a bug (the DISC-003 filter-bar ruling).
  const send = container.querySelector("button[type=submit]") as HTMLButtonElement;
  expect(send.disabled).toBe(false);

  await type("   ");
  await submit();

  expect(calls).toHaveLength(0);
  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("內容不能空白");
  // What was typed is still there to fix rather than cleared under the reader.
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
  expect(text()).not.toContain("已收到");
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
    // No run in the address, so no run id — not an empty string.
    run_id: undefined,
  });
});

test("BETA-003 the counter counts what the server counts, so an emoji report is not refused at half length", async () => {
  // The server checks `len([]rune(message))`; `String.length` counts an emoji as
  // two UTF-16 units. 1500 emoji is 1500 runes and 3000 units, so the old counter
  // said 3000／2000 and the pre-check refused a report the server would have
  // taken. GenerateSkill.tsx already refuses `maxLength` on this same reasoning.
  const calls = stubPlatform();
  await render(<FeedbackEntry pathname="/" />);

  const emoji = "🙂".repeat(1500);
  await type(emoji);
  expect(text()).toContain(`1500／${FEEDBACK_MAX_MESSAGE} 字`);

  await submit();
  await waitFor(() => calls.length > 0);
  expect(calls[0].body).toMatchObject({ message: emoji });
});
