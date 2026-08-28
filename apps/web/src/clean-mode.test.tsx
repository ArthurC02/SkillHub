import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";

// PORT-003. Same shape as generate.test.tsx:169-183 (GEN-002/GEN-004) —
// dynamic import, one component rendered without the router.
//
// One difference from that harness: GeneratedNotice takes its state as a
// prop, so it needs nothing else in the tree. CleanModeNotice reads
// `GET /me` through `useCleanMode`, so this harness wraps it in the app's
// own `queryClient` and stubs `fetch` the way generate.test.tsx's
// `stubSession` does, trimmed to the one endpoint this component calls.

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

/** A logged-in `/me`, with `features` present only when asked for. */
function stubMe(features?: Record<string, boolean>) {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
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
    return Promise.resolve(new Response(JSON.stringify({}), { status: 200 }));
  });
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

async function renderNotice() {
  const { CleanModeNotice } = await import("./components/CleanModeNotice");
  await act(async () => {
    root = createRoot(container);
    root.render(
      <QueryClientProvider client={queryClient}>
        <CleanModeNotice />
      </QueryClientProvider>,
    );
  });
}

test("PORT-003: with clean_mode on, the notice states its three absences", async () => {
  stubMe({ clean_mode: true });
  await renderNotice();
  await waitFor(() => (container.textContent?.length ?? 0) > 0);

  const text = container.textContent ?? "";
  expect(text).toContain("沙箱沒有隔離");
  expect(text).toContain("不驗證 presigned URL");
  expect(text).toContain("只有一條連線");
  expect(text).not.toContain("完整");
  expect(text).not.toContain("等同");
  expect(text).not.toContain("與正式環境一致");
});

// ADR-052's mechanism reused: /me without features.clean_mode is what every
// deployment returns until SKILLHUB_CLEAN_MODE is set, and the notice must not
// be drawable from that default.
test("PORT-003: without the flag, the notice renders nothing", async () => {
  stubMe();
  await renderNotice();
  // Give the /me query a chance to resolve; the assertion must hold either
  // way, but this rules out the test passing only because nothing has loaded.
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
  });

  expect(container.textContent).toBe("");
});
