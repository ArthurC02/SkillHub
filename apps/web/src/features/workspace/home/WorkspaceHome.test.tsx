import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../../core/api/queryClient";
import { WorkspaceHome } from "./WorkspaceHome.page";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    className,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    className?: string;
    children: unknown;
  }) => (
    <a
      className={className}
      href={Object.entries(params ?? {}).reduce((path, [key, value]) => {
        return path.replace(`$${key}`, value);
      }, to)}
    >
      {children as never}
    </a>
  ),
}));

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  await act(async () => queryClient.cancelQueries());
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
  await waitFor(settled);
}

function section(title: string) {
  return Array.from(container.querySelectorAll("section")).find(
    (candidate) => candidate.firstElementChild?.querySelector("h2")?.textContent === title,
  )!;
}

const run = (id: string, name: string, status: string, verdict: string) => ({
  run_id: id,
  status,
  skill_id: "skill-1",
  skill_name: name,
  skill_version_id: "version-1",
  provider: "self-hosted",
  cleanup_status: { value: "pending", label: "待清理", note: "完成後由平台清理。" },
  evaluation: { value: verdict, label: verdict, note: "依驗收條件判定。" },
  created_at: "2026-09-28T12:00:00Z",
});

test("workspace home separates decisions, active work, and owned assets from server facts", async () => {
  const fetchSpy = vi.fn((input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
      });
    }
    if (url.includes("/runs?")) {
      return json({
        runs: [
          run("run-review", "Needs review", "succeeded", "not_met"),
          run("run-failed", "Needs failure review", "failed", "not_evaluated"),
          run("run-active", "Still running", "running", "undetermined"),
          run("run-cancelled", "Cancelled", "cancelled", "not_evaluated"),
          run("run-done", "Already good", "succeeded", "met"),
        ],
      });
    }
    if (url.endsWith("/skills")) {
      return json({
        skills: [
          {
            skill_id: "skill-1",
            name: "PDF Summariser",
            summary: "整理長篇 PDF",
            redistribution: "unknown",
            risk: { scan_status: "scanned", level: "none", warnings: 0, disclosures: [] },
            verification: { value: "scanned", label: "已掃描", note: "靜態掃描完成。" },
          },
        ],
        total: 1,
        limit: 100,
        truncated: false,
      });
    }
    return json({ error: "not found" }, 404);
  });
  vi.stubGlobal("fetch", fetchSpy);

  await render(<WorkspaceHome />, () => (container.textContent ?? "").includes("PDF Summariser"));

  expect(section("需要留意").textContent).toContain("Needs review");
  expect(section("需要留意").textContent).toContain("Needs failure review");
  expect(section("需要留意").textContent).toContain("檢視證據");
  expect(section("需要留意").textContent).toContain("查看原因");
  expect(section("需要留意").textContent).not.toContain("Still running");
  expect(section("需要留意").textContent).not.toContain("Cancelled");
  expect(section("執行中").textContent).toContain("Still running");
  expect(section("執行中").textContent).not.toContain("Needs review");
  expect(container.textContent).not.toContain("Already good");
  expect(section("你的資產").textContent).toContain("PDF Summariser");

  const refresh = Array.from(container.querySelectorAll("button")).find((candidate) =>
    candidate.textContent?.includes("重新整理"),
  );
  expect(refresh).toBeDefined();
  expect(refresh?.closest("p")?.querySelector("time")).not.toBeNull();

  const before = fetchSpy.mock.calls.filter(([input]) => String(input).includes("/runs?")).length;
  await act(async () => refresh?.click());
  await waitFor(
    () => fetchSpy.mock.calls.filter(([input]) => String(input).includes("/runs?")).length > before,
  );
});

test("workspace home without active work does not show a stale refresh control", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/me")) {
      return json({
        user_id: "u-1",
        email: "tester@example.com",
        display_name: "tester",
        workspace_id: "ws-1",
        operator: false,
      });
    }
    if (url.includes("/runs?")) {
      return json({ runs: [run("run-done", "Already good", "succeeded", "met")] });
    }
    if (url.endsWith("/skills")) {
      return json({ skills: [], total: 0, limit: 100, truncated: false });
    }
    return json({ error: "not found" }, 404);
  });

  await render(<WorkspaceHome />, () =>
    (container.textContent ?? "").includes("目前沒有正在執行的試跑"),
  );

  expect(container.textContent).not.toContain("上次取得於");
  expect(
    Array.from(container.querySelectorAll("button")).find((candidate) =>
      candidate.textContent?.includes("重新整理"),
    ),
  ).toBeUndefined();
});

async function waitFor(done: () => boolean) {
  const deadline = Date.now() + 2000;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`wait timed out: ${container.textContent}`);
}
