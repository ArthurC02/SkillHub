import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { ImportSkill } from "./pages/ImportSkill";

// 04 丙-150/152 — ImportSkill's own writer/mutation-state coverage, split out
// of workspace.test.tsx (which is another writer's this round). Scaffolding
// copied from workspace.test.tsx.

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

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    children?: unknown;
  }) => (
    <a href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}>
      {children as never}
    </a>
  ),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

async function render(node: ReactNode, settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(settled);
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

const text = () => container.textContent ?? "";

const ME = { user_id: "u-1", email: "a@b.c", display_name: "a", workspace_id: "ws-1" };

async function submitURL(url = "https://github.com/example/skill") {
  await render(<ImportSkill />, () => text().includes("匯入 Skill"));
  const input = container.querySelector<HTMLInputElement>('input[type="url"]')!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.bind(
      input,
    );
    setter(url);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

// --- (a) mutation.onError by status ----------------------------------------

test("04 丙-150(a): a session that expired mid-import says 需要登入, not the raw server string", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST") return json({ error: "not authenticated" }, 401);
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("需要登入"));

  expect(text()).not.toContain("not authenticated");
});

test("04 丙-150(a): a 400 (bad zip / unreachable URL) gets the page's own sentence", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST") return json({ error: "bad request" }, 400);
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("這個檔案不是可用的 zip 套件，或網址抓不到內容。"));
});

// --- (c) a non-empty findings array, rendered ------------------------------

/**
 * 04 丙-152 — the first test in this repo to render a non-empty findings
 * array. Messages and codes are the real Go strings
 * (apps/platform/internal/shared/skillpkg/skillpkg.go:308,443), copied
 * verbatim per 04 丙-143's fixture rule.
 */
test("04 丙-152: a categorised 422 renders both real Chinese finding messages and their codes", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST")
      return json(
        {
          errors: [
            {
              severity: "error",
              code: "skill-md-missing",
              path: "SKILL.md",
              message: "套件根目錄找不到 SKILL.md",
            },
            {
              severity: "error",
              code: "name-invalid",
              path: "SKILL.md",
              message: "name 只能使用小寫英文字母、數字與單一連字號",
            },
          ],
          warnings: [],
          infos: [],
        },
        422,
      );
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("套件根目錄找不到 SKILL.md"));

  expect(text()).toContain("套件根目錄找不到 SKILL.md");
  expect(text()).toContain("skill-md-missing");
  expect(text()).toContain("name 只能使用小寫英文字母、數字與單一連字號");
  expect(text()).toContain("name-invalid");
});
