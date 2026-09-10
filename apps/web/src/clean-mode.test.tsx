import { readFileSync } from "node:fs";
import { join } from "node:path";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";

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
  expect(text).toContain("物件儲存只在記憶體裡，行程結束即消失。");
  expect(text).toContain("試跑前那份「可連往哪裡」的清單，在這個模式下不被強制");
  expect(text).not.toContain("完整");
  expect(text).not.toContain("等同");
  expect(text).not.toContain("與正式環境一致");
});

test("PORT-003: without the flag, the notice renders nothing", async () => {
  stubMe();
  await renderNotice();
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
  });

  expect(container.textContent).toBe("");
});

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

test("PORT-003: without the injected flag, a signed-out visitor (401 from /me) sees nothing", async () => {
  stubAnonymous();
  await renderNotice();
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
  });

  expect(container.textContent).toBe("");
});

test("PORT-003: index.html still carries the exact placeholder cmd/api rewrites", () => {
  const html = readFileSync(join(import.meta.dirname, "..", "index.html"), "utf8");
  expect(html).toContain("<!--SKILLHUB_CLEAN_MODE_FLAG-->");
});
