import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { AuthControls } from "./components/AuthControls";
import { ImportSkill } from "./pages/ImportSkill";
import { CancelRunControl } from "./pages/RunTrace";

let container: HTMLDivElement;
let root: Root;

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, params, children }: { to: string; params?: Record<string, string>; children: unknown }) => (
    <a href={Object.entries(params ?? {}).reduce((path, [key, value]) => path.replace(`$${key}`, value), to)}>
      {children as never}
    </a>
  ),
  useParams: () => ({ runId: "run-1" }),
}));

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
  const deadline = Date.now() + 2000;
  while (!settled()) {
    if (Date.now() > deadline) throw new Error(`render timeout: ${container.textContent}`);
    await act(async () => new Promise((resolve) => setTimeout(resolve, 5)));
  }
}

test("anonymous visitors get a working GitHub login entry", async () => {
  vi.stubGlobal("fetch", vi.fn(() => json({ error: "no session" }, 401)));
  await render(<AuthControls />, () => container.querySelector("a") !== null);

  expect(container.querySelector("a")?.getAttribute("href")).toBe(
    "http://localhost:8080/auth/github/login",
  );
});

test("signed-in users can end the session and clear cached workspace data", async () => {
  let signedIn = true;
  const fetchMock = vi.fn((_url: string | URL | Request, init?: RequestInit) => {
    if (init?.method === "POST") {
      signedIn = false;
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    return signedIn
      ? json({
          user_id: "user-1",
          workspace_id: "workspace-1",
          display_name: "Ada",
          email: "ada@example.com",
        })
      : json({ error: "no session" }, 401);
  });
  vi.stubGlobal("fetch", fetchMock);
  queryClient.setQueryData(["private", "sentinel"], { secret: true });
  await render(<AuthControls />, () => container.textContent?.includes("登出") ?? false);

  await act(async () => container.querySelector<HTMLButtonElement>("button")!.click());
  await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));

  expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(true);
  expect(queryClient.getQueryData(["private", "sentinel"])).toBeUndefined();
});

test("URL import sends the source and links the imported skill", async () => {
  const fetchMock = vi.fn((_url: string | URL | Request, init?: RequestInit) =>
    json({
      skill_id: "skill-1",
      version_id: "version-1",
      version_number: 2,
      content_hash: "abc",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    }, init?.method === "POST" ? 201 : 200),
  );
  vi.stubGlobal("fetch", fetchMock);
  await render(<ImportSkill />, () => container.querySelector("form") !== null);

  const input = container.querySelector<HTMLInputElement>("#skill-import-url")!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(
      input,
      "https://github.com/example/skill",
    );
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));

  const [, init] = fetchMock.mock.calls.at(-1)!;
  expect(init?.method).toBe("POST");
  expect(init?.body).toBe(JSON.stringify({ url: "https://github.com/example/skill" }));
  expect(container.querySelector('a[href="/skills/skill-1"]')).not.toBeNull();
});

test("a running Run requires confirmation before cancellation", async () => {
  const fetchMock = vi.fn((_url: string | URL | Request, _init?: RequestInit) =>
    json({
      run_id: "run-1",
      skill_id: "skill-1",
      skill_version_id: "version-1",
      test_case_snapshot_id: "snapshot-1",
      note: "cancellation requested",
    }, 202),
  );
  vi.stubGlobal("fetch", fetchMock);
  await render(<CancelRunControl runId="run-1" status="running" />, () => Boolean(container.querySelector("button")));

  await act(async () => container.querySelector<HTMLButtonElement>("button")!.click());
  expect(container.textContent).toContain("確定要取消");
  await act(async () => container.querySelector<HTMLButtonElement>("button")!.click());
  await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));

  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(String(fetchMock.mock.calls[0][0])).toContain("/runs/run-1/cancel");
  expect(fetchMock.mock.calls[0][1]?.method).toBe("POST");
});
