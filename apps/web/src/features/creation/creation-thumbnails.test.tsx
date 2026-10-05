import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { CreationSession } from "./create/components/CreationSession";
import type { CreationSession as Session } from "./creation.service";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}));
vi.mock("./generate/GenerateSkill", () => ({
  GenerateSkill: () => null,
  ReferencePicker: () => null,
}));

let box: HTMLDivElement, root: Root, q: QueryClient;
let revoke: ReturnType<typeof vi.fn<(url: string) => void>>;

const LIMITS = {
  min_budget_credits: 130,
  max_budget_credits: 6500,
  max_steps: 20,
  max_tool_calls: 10,
  call_timeout_seconds: 120,
  session_timeout_seconds: 3600,
  retention_seconds: 604800,
};

const sample = (patch: Partial<Session> = {}): Session => ({
  id: "s1",
  revision: 1,
  state: "waiting_input",
  snapshot: {
    messages: [],
    brief: "",
    brief_confirmed: false,
    acceptance_criteria: [],
    diagram_understanding: "",
    diagram_confirmed: false,
    references: [],
    pending_action: "",
    budget_credits: 500,
    reserved_credits: 50,
    usage_unknown: true,
    steps: 1,
    tool_calls: 0,
  },
  created_at: "2026-09-05T00:00:00Z",
  updated_at: "2026-09-05T00:00:00Z",
  expires_at: "2026-09-06T00:00:00Z",
  deadline: "2026-09-05T01:00:00Z",
  ...patch,
});

const response = (v: unknown, status = 200) =>
  Promise.resolve(
    new Response(JSON.stringify(v), { status, headers: { "Content-Type": "application/json" } }),
  );

beforeEach(() => {
  box = document.createElement("div");
  document.body.appendChild(box);
  let minted = 0;
  URL.createObjectURL = vi.fn(() => `blob:${++minted}`);
  revoke = vi.fn<(url: string) => void>();
  URL.revokeObjectURL = revoke;
  q = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
});

afterEach(async () => {
  await act(async () => root?.unmount());
  q.clear();
  box.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function waitFor(fn: () => boolean) {
  const until = Date.now() + 2500;
  while (!fn()) {
    if (Date.now() > until) throw Error(box.textContent ?? "timeout");
    await act(async () => new Promise((r) => setTimeout(r, 5)));
  }
}

async function sendDiagram() {
  const sent = sample({ revision: 2 });
  sent.snapshot.messages = [{ role: "user", content: "這是我的流程。" }];
  sent.snapshot.attachments = [
    { message_index: 0, media_type: "image/png", bytes: 7, sha256: "digest-1" },
  ];
  let posts = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        posts += 1;
        return response(posts === 1 ? sample() : sent);
      }
      if (url.endsWith("/creation-sessions/limits")) return response(LIMITS);
      if (url.endsWith("/me/credits")) return response({ error: "not found" }, 404);
      return response(url.endsWith("/creation-sessions") ? [] : sent);
    }),
  );

  await act(async () => {
    root = createRoot(box);
    root.render(
      <QueryClientProvider client={q}>
        <CreationSession />
      </QueryClientProvider>,
    );
  });
  const budget = 'input[name="creation-budget"][value="500"]';
  await waitFor(() => !!box.querySelector(budget));
  await act(async () => box.querySelector<HTMLInputElement>(budget)!.click());

  const file = box.querySelector('input[type="file"]') as HTMLInputElement;
  await act(async () => {
    Object.defineProperty(file, "files", {
      configurable: true,
      value: [new File(["diagram"], "flow.png", { type: "image/png" })],
    });
    file.dispatchEvent(new Event("change", { bubbles: true }));
  });
  const start = box.querySelector<HTMLButtonElement>('button[aria-label="開始創作"]')!;
  await act(async () => start.click());
  await waitFor(() => !!box.querySelector('[role="log"] li[data-role="user"] img'));
}

test("the sent picture's thumbnail stays alive while the session is on screen", async () => {
  await sendDiagram();

  expect(
    box.querySelector('[role="log"] li[data-role="user"] img')!.getAttribute("src"),
    "the log shows the thumbnail minted when the picture was sent, not the composer preview",
  ).toBe("blob:2");
  expect(revoke.mock.calls.map(([url]) => url)).toEqual(["blob:1"]);
});

test("leaving the session releases the sent picture's thumbnail exactly once", async () => {
  await sendDiagram();
  revoke.mockClear();

  await act(async () => root.unmount());

  expect(revoke.mock.calls.map(([url]) => url)).toEqual(["blob:2"]);
});
