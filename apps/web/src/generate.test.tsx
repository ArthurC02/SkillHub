import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import { GenerationFailureFailureEnum } from "@skillhub/api-client-ts";

// GEN-004 and GEN-008. Same hand-rolled harness the other suites use.
//
// The load-bearing case is the first one: an entry point that appears when it
// should not is the failure ADR-052 exists to prevent, and it has no symptom —
// the page looks fine, and what breaks is the meaning of a number nobody
// re-measures. Twelve people, one chance.

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  window.history.pushState({}, "", "/");
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function render() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
  if (container.querySelector(".app-shell") && router.state.location.pathname !== "/") {
    await act(async () => {
      await router.navigate({ to: "/", search: {} });
    });
  }
}

const NO_RESULTS = {
  query: "沒有人做過的事",
  results: [],
  limit: 20,
  truncated: false,
  degraded: false,
  partial_index: false,
  filtered_out: false,
  no_results: true,
  query_suggestion: "試著說出你手上的檔案格式。",
};

/** A logged-in session, with `features` present only when asked for. */
function stubSession(features?: Record<string, boolean>, failures?: unknown[]) {
  const posted: { path: string; body: string }[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input).replace(/^https?:\/\/[^/]+/, "").split("?")[0];
    if (init?.method === "POST") {
      posted.push({ path, body: String(init.body ?? "") });
      return Promise.resolve(
        new Response(JSON.stringify({ error: "not implemented in this stub" }), { status: 502 }),
      );
    }
    if (path === "/skills/generate/failures") {
      return Promise.resolve(
        new Response(JSON.stringify({ failures: failures ?? [] }), { status: 200 }),
      );
    }
    if (path.startsWith("/api/skills/search")) {
      return Promise.resolve(new Response(JSON.stringify(NO_RESULTS), { status: 200 }));
    }
    if (path === "/me") {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            user_id: "u-1",
            email: "t@example.com",
            display_name: "tester",
            workspace_id: "ws-1",
            deletion_requested_at: null,
            purge_after: null,
            deletion_scope: null,
            ...(features ? { features } : {}),
          }),
          { status: 200 },
        ),
      );
    }
    return Promise.resolve(new Response(JSON.stringify({ skills: [] }), { status: 200 }));
  });
  return posted;
}

async function submitSearch(text: string) {
  const input = container.querySelector("input")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await waitFor(() => !container.textContent?.includes("搜尋中…"));
}

async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (done()) return;
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

// ADR-052's whole point. `/me` without a `features` object is what every
// deployment returns today, and the entry point must not be drawable from it.
test("GEN-008: the generate entry point is absent until /me says the flag is on", async () => {
  stubSession();
  await render();
  await submitSearch("沒有人做過的事");

  // The no-results state itself is still there — otherwise this passes because
  // the page failed, not because the flag held.
  expect(container.textContent).toContain("沒有夠接近的 Skill");
  expect(container.textContent).not.toContain("讓平台依你的描述做一個");
  expect(container.querySelector("#generate-task")).toBeNull();
});

test("GEN-008: with the flag on, the entry point appears in the no-results state", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  expect(container.textContent).toContain("讓平台依你的描述做一個");
  const box = container.querySelector<HTMLTextAreaElement>("#generate-task");
  expect(box).not.toBeNull();
  // DISC-005's suggestion is still the server's, and the generate box is
  // seeded with what was searched for rather than starting empty.
  expect(container.textContent).toContain("試著說出你手上的檔案格式。");
  expect(box!.value).toBe("沒有人做過的事");
});

// 02:GEN-002 forbids showing a generated package as an unknown source, and
// GEN-004 requires two named absences rather than one neutral word. Both are
// sentences that stay wrong silently.
test("GEN-002/GEN-004: a generated skill's source is stated, and its two absences with it", async () => {
  const { GeneratedNotice } = await import("./components/GeneratedNotice");
  await act(async () => {
    root = createRoot(container);
    root.render(<GeneratedNotice />);
  });

  const text = container.textContent ?? "";
  expect(text).toContain("沒有經過任何人工檢視");
  expect(text).toContain("沒有任何試跑證據");
  // ADR-041 決策 2 / 02:GEN-004: a neutral word describing a package the
  // platform wrote thirty seconds ago as merely recent is the failure named.
  expect(text).not.toContain("新建立");
  expect(text).not.toContain("來源未知");
});

// GEN-003's last clause. The write half shipped first and was briefly counted as
// satisfying it; a row only a database connection can see is not a record left
// in the workspace, and the gap shows no symptom on any screen.
test("GEN-003: past failures are readable, and the task description is not among them", async () => {
  stubSession({ generate_skill: true }, [
    {
      occurred_at: "2026-08-23T10:00:00Z",
      failure: "blocked",
      attempts: 2,
      codes: ["name-invalid"],
    },
    { occurred_at: "2026-08-23T09:00:00Z", failure: "quota", attempts: 0 },
  ]);
  await render();
  await submitSearch("沒有人做過的事");
  await waitFor(() => (container.textContent ?? "").includes("最近沒有成功的生成"));

  const text = container.textContent ?? "";
  expect(text).toContain("最近沒有成功的生成（2 次）");
  expect(text).toContain("name-invalid");
  // 02:GEN-001: a refusal before the model call costs nothing, and the row has
  // to say so — otherwise "額度不足" reads like something the user was billed for.
  expect(text).toContain("沒有呼叫模型，也沒有花錢");
  // NFR-002: the description belongs to the source row, not to 400-day history.
  expect(text).toContain("這裡沒有記下你當時輸入的任務描述");
});

// An empty history is a section with no rows, not a value rendered blank
// (design system §2.9 is about the latter). A user who has never failed must
// not be shown a heading for something that did not happen.
test("GEN-003: a workspace with no failures is shown no history section", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  expect(container.textContent).toContain("讓平台依你的描述做一個");
  expect(container.textContent).not.toContain("最近沒有成功的生成");
});

// GEN-008 / 02:GEN-001 「生成前顯示…」. Two things are asserted: that the three
// enforced ceilings are on screen, and that the money position says 尚未定值
// rather than printing the measured average — the latter is the 04 乙-2 shape
// (a number with nothing behind it) and the former is its correction.
test("GEN-008: the bounds the server enforces are stated before the button, and the cost says it is unset", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const text = container.textContent ?? "";
  expect(text).toContain("這一次最多會用到");
  expect(text).toContain("4,000 字");
  expect(text).toContain("推理加輸出合計 16,000 token");
  expect(text).toContain("最多嘗試 2 次");
  expect(text).toContain("尚未定值");
  // The measured average must not appear as a promise.
  expect(text).not.toMatch(/\$0\.00/);
  // And the textarea carries no maxLength: the browser's unit (UTF-16 code
  // units) is not the server's (runes), so one enforcer, and it is the server.
  expect(container.querySelector<HTMLTextAreaElement>("#generate-task")!.maxLength).toBe(-1);
});

// The sentence table is keyed on the hand-written union; this asserts it
// against the generated enum, so a value added to the contract and not to
// types.ts fails here rather than rendering the "unreadable" fallback for a
// value the server meant (the PACKAGING_BLOCKED_LABEL pattern).
test("GEN-003: every failure value in the contract has a sentence", async () => {
  const { FAILURE_SENTENCE } = await import("./components/GenerateSkill");
  for (const value of Object.values(GenerationFailureFailureEnum)) {
    const sentence = FAILURE_SENTENCE[value as keyof typeof FAILURE_SENTENCE];
    expect(sentence, `no sentence for failure ${JSON.stringify(value)}`).toBeTypeOf("function");
  }
});

