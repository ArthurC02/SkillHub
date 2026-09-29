import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { WorkspaceSkills } from "./skills/WorkspaceSkills.page";
import type { OwnSkill } from "../../core/api/types";

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

const SECOND: OwnSkill = { ...SKILL, skill_id: "s-2", name: "第二個 Skill" };

const FIRST_PAGE = Array.from({ length: 24 }, (_, index) => ({
  ...SKILL,
  skill_id: `page-skill-${index + 1}`,
  name: `分頁 Skill ${index + 1}`,
}));

function pointAt(target: Element, type: string, pointerType: string) {
  target.dispatchEvent(
    new PointerEvent(type, { bubbles: true, clientX: 30, clientY: 40, pointerType }),
  );
}

test("the card grid lights up where a mouse points, stays dark for touch, and goes dark on leave", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    return json({ skills: [SKILL, SECOND], limit: 100, truncated: false, total: 2 });
  });
  await render(<WorkspaceSkills />, () => text().includes("第二個 Skill"));
  const grid = container.querySelector(".skill-grid")!;
  const glow = () =>
    Array.from(
      grid.querySelectorAll<HTMLElement>(".skill-card"),
      (card) => `${card.style.getPropertyValue("--x")} ${card.style.getPropertyValue("--y")}`,
    );

  await act(async () => pointAt(grid.firstElementChild!, "pointermove", "touch"));
  expect(glow()).toEqual([" ", " "]);

  await act(async () => pointAt(grid.firstElementChild!, "pointermove", "mouse"));
  expect(glow()).toEqual(["30px 40px", "30px 40px"]);

  await act(async () =>
    grid.dispatchEvent(
      new PointerEvent("pointerout", { bubbles: true, relatedTarget: document.body }),
    ),
  );
  expect(glow()).toEqual([" ", " "]);
});

test("the library loads every owner skill page without duplicating the existing cards", async () => {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    if (url.endsWith("/me")) return json(ME);
    calls.push(url);
    if (url.includes("offset=24")) {
      return json({ skills: [FIRST_PAGE[23], SECOND], limit: 24, truncated: false, total: 25 });
    }
    return json({ skills: FIRST_PAGE, limit: 24, truncated: true, total: 25 });
  });

  await render(<WorkspaceSkills />, () => text().includes("已顯示 24 / 25 個 Skill"));
  expect(calls).toContain("/skills?limit=24&offset=0");

  await act(async () => button("載入更多")?.click());
  await waitFor(() => text().includes("已顯示 25 / 25 個 Skill"));

  expect(calls).toContain("/skills?limit=24&offset=24");
  expect(container.querySelectorAll(".skill-card")).toHaveLength(25);
  expect(text()).toContain("分頁 Skill 1");
  expect(text()).toContain("第二個 Skill");
  expect(button("載入更多")).toBeUndefined();
});

test("a later library page failure keeps the loaded skills and offers the same action again", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    if (url.endsWith("/me")) return json(ME);
    if (url.includes("offset=24")) return json({ error: "temporarily unavailable" }, 503);
    return json({ skills: FIRST_PAGE, limit: 24, truncated: true, total: 25 });
  });

  await render(<WorkspaceSkills />, () => text().includes("已顯示 24 / 25 個 Skill"));
  await act(async () => button("載入更多")?.click());
  await waitFor(() => text().includes("沒有載入更多"));

  expect(container.querySelectorAll(".skill-card")).toHaveLength(24);
  expect(text()).toContain("已顯示的 24 個仍可使用");
  expect(text()).not.toContain("temporarily unavailable");
  expect(button("載入更多")).toBeDefined();
});

test("a manage menu stays open for a click inside it, and closes on a click outside or Escape", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    return json({ skills: [SKILL, SECOND], limit: 100, truncated: false, total: 2 });
  });
  await render(<WorkspaceSkills />, () => text().includes("第二個 Skill"));
  const [first, second] = Array.from(container.querySelectorAll<HTMLDetailsElement>(".skill-menu"));
  const press = (target: EventTarget) =>
    act(async () => {
      target.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    });

  await act(async () => {
    first.open = true;
  });
  await press(first.querySelector("a")!);
  expect(first.open, "a click on a menu item closed its own menu").toBe(true);

  await press(document.body);
  expect(first.open).toBe(false);

  await act(async () => {
    second.open = true;
  });
  await act(async () => {
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
  });
  expect(second.open).toBe(false);
  expect(document.activeElement).toBe(second.querySelector("summary"));
});

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
  const status = container.querySelector('[role="status"]');
  expect(status?.textContent ?? "").not.toContain("這個 Skill 已經不在了。");
});
