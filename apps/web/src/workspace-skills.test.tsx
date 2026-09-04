import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { WorkspaceSkills } from "./pages/WorkspaceSkills";
import type { OwnSkill } from "./api/types";

// 04 丙-150 — WorkspaceSkills' own writer/mutation-state coverage, split out
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

function button(text: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes(text),
  );
}

const text = () => container.textContent ?? "";

const ME = { user_id: "u-1", email: "a@b.c", display_name: "a", workspace_id: "ws-1" };

const SKILL: OwnSkill = {
  skill_id: "s-1",
  name: "範例 Skill",
  summary: "一個測試用的摘要",
  redistribution: "allowed",
  risk: { scan_status: "scanned", level: "none", warnings: 0, disclosures: [], note: "" },
  verification: { value: "not_measured", label: "未測量", note: "" },
};

// The real Go string for DELETE /skills/{id}'s `note` (04 丙-149,
// library/http.go:157) — copied verbatim per 04 丙-143's fixture rule.
const DELETION_NOTE =
  "已從你的工作區、清單與搜尋移除；版本快照維持凍結，這次刪除不會移除它們；Fork 引用的共用套件物件不受影響";

async function openConfirm() {
  await render(<WorkspaceSkills />, () => text().includes("範例 Skill"));
  await act(async () => button("刪除")?.click());
  await act(async () => button("確認刪除")?.click());
}

test("04 丙-150(b): a successful delete shows the server's real Chinese deletionNote", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "DELETE")
      return json({ deleted: true, versions_retained: 2, note: DELETION_NOTE });
    return json({ skills: [SKILL], limit: 100, truncated: false, total: 1 });
  });

  await openConfirm();
  await waitFor(() => text().includes(DELETION_NOTE));

  const status = container.querySelector('[role="status"]');
  expect(status?.textContent).toContain(`已刪除。${DELETION_NOTE}`);
});

test("04 丙-150(b): a session that expired mid-delete says 需要登入, not the raw server string", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "DELETE") return json({ error: "not authenticated" }, 401);
    return json({ skills: [SKILL], limit: 100, truncated: false, total: 1 });
  });

  await openConfirm();
  await waitFor(() => text().includes("需要登入"));

  expect(text()).not.toContain("not authenticated");
});

test("04 丙-150(b): a 404 delete (skill already gone) gets the page's own sentence, in role=alert not role=status", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "DELETE") return json({ error: "not found" }, 404);
    return json({ skills: [SKILL], limit: 100, truncated: false, total: 1 });
  });

  await openConfirm();
  await waitFor(() => text().includes("這個 Skill 已經不在了。"));

  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("這個 Skill 已經不在了。");
  // success and failure never share one element (04 丙-150).
  const status = container.querySelector('[role="status"]');
  expect(status?.textContent ?? "").not.toContain("這個 Skill 已經不在了。");
});
