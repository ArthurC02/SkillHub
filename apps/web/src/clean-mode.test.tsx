import { readFileSync } from "node:fs";
import { join } from "node:path";
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
  delete window.__SKILLHUB_CLEAN_MODE__;
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

/**
 * A signed-out visitor: `GET /me` answers 401, the way it does for anyone
 * without a session — `/` and `/skills/$id` are both reachable in this state,
 * which is exactly what the old `/me`-only flag could never disclose to.
 */
function stubAnonymous() {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify({ error: "unauthenticated" }), { status: 401 })),
  );
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

test("PORT-003: with clean_mode on, the notice states its five absences", async () => {
  stubMe({ clean_mode: true });
  await renderNotice();
  await waitFor(() => (container.textContent?.length ?? 0) > 0);

  const text = container.textContent ?? "";
  expect(text).toContain("沙箱沒有隔離");
  expect(text).toContain("不驗證 presigned URL");
  expect(text).toContain("只有一條連線");
  // The fourth: an in-memory object store. Its failure has no symptom until a
  // restart, so it is the one absence a reader cannot infer from the screen.
  expect(text).toContain("物件儲存只在記憶體裡，行程結束即消失。");
  // The fifth, and the only one the user can be actively misled about rather
  // than merely uninformed of: 試跑前的權限摘要 prints an egress allow list and
  // asks them to agree to it, and in this mode nothing holds the run to it
  // (04 丙-98). 設計 §2.2 ranks 「顯示但不強制」 as the worst of the four states,
  // so the sentence has to name that screen rather than talk about networking.
  expect(text).toContain("試跑前那份「可連往哪裡」的清單，在這個模式下不被強制");
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

// The gap this change closes: cmd/api's clean-mode static handler
// (apps/platform/cmd/api/main.go, cleanModeStaticHandler) injects
// `window.__SKILLHUB_CLEAN_MODE__ = true` into the HTML it serves itself, so
// the notice reaches a visitor `GET /me` (session-gated) never could.
test("PORT-003: the injected flag alone shows the notice to a signed-out visitor, before /me resolves", async () => {
  stubAnonymous();
  window.__SKILLHUB_CLEAN_MODE__ = true;
  await renderNotice();
  await waitFor(() => (container.textContent?.length ?? 0) > 0);

  const text = container.textContent ?? "";
  expect(text).toContain("沙箱沒有隔離");
  expect(text).toContain("不驗證 presigned URL");
  expect(text).toContain("只有一條連線");
});

// The other half: a signed-out visitor whose page was not served by the
// clean-mode static handler (so nothing injected the flag) must not see the
// notice either — the same 401 as above, but with no window flag set.
test("PORT-003: without the injected flag, a signed-out visitor (401 from /me) sees nothing", async () => {
  stubAnonymous();
  await renderNotice();
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
  });

  expect(container.textContent).toBe("");
});

/**
 * 02:PORT-003 的匿名揭露，其實只是一行位元組。
 *
 * `cmd/api`'s `cleanModeStaticHandler` (apps/platform/cmd/api/main.go) swaps
 * this exact comment for a `<script>` that sets `window.__SKILLHUB_CLEAN_MODE__`,
 * and it matches the bytes. Every test above stubs that window flag or `/me`,
 * so all four pass on an `index.html` that no longer carries the placeholder at
 * all — the disclosure would then be dead for exactly the readers it was added
 * for (signed-out visitors on `/` and `/skills/$id`), with nothing red.
 *
 * So this one reads the real file. Not a rendering assertion: the substitution
 * happens in Go, and the only thing this side owns is that the target string is
 * still there, verbatim, for the handler to find. `.prettierignore` carries the
 * other half — a formatter that reflows the comment breaks the same match.
 */
test("PORT-003: index.html still carries the exact placeholder cmd/api rewrites", () => {
  const html = readFileSync(join(import.meta.dirname, "..", "index.html"), "utf8");
  expect(html).toContain("<!--SKILLHUB_CLEAN_MODE_FLAG-->");
});
