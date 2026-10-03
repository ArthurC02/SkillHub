import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { FeatureAvailabilityProvider } from "../../shared/FeatureAvailabilityProvider";
import { Activity } from "./Activity.page";

const flags = vi.hoisted(() => ({ creation: false, generate: false }));

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    className,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string>;
    className?: string;
    children: unknown;
  }) => {
    const path = Object.entries(params ?? {}).reduce(
      (value, [key, parameter]) => value.replace(`$${key}`, parameter),
      to,
    );
    const query = new URLSearchParams(search).toString();
    return (
      <a className={className} href={query ? `${path}?${query}` : path}>
        {children as never}
      </a>
    );
  },
}));

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  flags.creation = false;
  flags.generate = false;
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
        <QueryClientProvider client={queryClient}>
          <FeatureAvailabilityProvider creation={flags.creation && flags.generate}>
            {node}
          </FeatureAvailabilityProvider>
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(settled);
}

const item = (
  kind: string,
  sourceId: string,
  classification: string,
  continuation: Record<string, string>,
) => ({
  kind,
  source_id: sourceId,
  summary: `${kind} ${sourceId}`,
  classification,
  status: { value: "owner_status", label: `狀態 ${sourceId}` },
  activity_at: "2026-09-30T08:00:00Z",
  continuation,
});

test("activity groups owner facts and keeps every continuation in its product context", async () => {
  flags.creation = true;
  flags.generate = true;
  vi.stubGlobal("fetch", () =>
    json({
      complete: true,
      sources: ["run", "evaluation", "creation", "packaging", "publishing"],
      next_cursor: "next-page",
      items: [
        {
          ...item("run", "run-1", "needs_attention", { kind: "run", run_id: "run-1" }),
          context: {
            skill_id: "skill-1",
            skill_name: "PDF Summariser",
            skill_version_id: "version-2",
            test_case_id: "case-1",
          },
        },
        item("creation_session", "session-1", "in_progress", {
          kind: "creation_session",
          session_id: "session-1",
        }),
        item("packaging_artifact", "artifact-1", "recent", {
          kind: "packaging_artifact",
          artifact_id: "artifact-1",
        }),
        item("skill_publication", "publication-1", "recent", {
          kind: "skill_publication",
          publisher: "arthur",
          publication_name: "summariser",
        }),
      ],
    }),
  );

  await render(<Activity />, () => (container.textContent ?? "").includes("publication-1"));

  expect(container.textContent).toContain("需要你處理");
  expect(container.textContent).toContain("平台處理中");
  expect(container.textContent).toContain("最近完成");
  expect(container.textContent).toContain("完整來源5 / 5");
  expect(container.textContent).toContain("本頁需要處理1");
  expect(container.textContent).toContain("本頁進行中1");
  expect(container.textContent).toContain("尚有更多活動");
  expect(container.querySelector('a[href="/skills/skill-1"]')?.textContent).toBe("PDF Summariser");
  expect(container.querySelector('a[href="/skills/skill-1/versions/version-2"]')?.textContent).toBe(
    "精確版本",
  );
  expect(
    container.querySelector('a[href="/lab/test-cases/case-1?version=version-2"]')?.textContent,
  ).toBe("測試題");
  expect(container.querySelector('a[href="/runs/run-1"]')?.textContent).toBe("查看試跑紀錄");
  expect(
    container.querySelector('a[href="/workspace/creations?session=session-1"]')?.textContent,
  ).toBe("繼續創作");
  expect(
    container.querySelector('a[href="/workspace/downloads?artifact=artifact-1"]')?.textContent,
  ).toBe("查看套件");
  expect(
    container.querySelector('a[href="/workspace/downloads?publication=arthur%2Fsummariser"]')
      ?.textContent,
  ).toBe("查看發佈");
  const activityRows = Array.from(container.querySelectorAll(".activity-list > li"));
  expect(activityRows.map((row) => row.getAttribute("data-classification"))).toEqual([
    "needs_attention",
    "in_progress",
    "recent",
    "recent",
  ]);
  expect(
    activityRows.map((row) => row.querySelector(".activity-rail")?.getAttribute("aria-hidden")),
  ).toEqual(["true", "true", "true", "true"]);
});

test("activity does not expose a Studio continuation while either creation flag is off", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      complete: true,
      sources: ["run", "evaluation", "creation", "packaging", "publishing"],
      items: [
        item("creation_session", "session-1", "needs_attention", {
          kind: "creation_session",
          session_id: "session-1",
        }),
      ],
    }),
  );

  await render(<Activity />, () => (container.textContent ?? "").includes("Studio 目前未開放"));

  expect(container.querySelector('a[href^="/workspace/creations"]')).toBeNull();
  expect(container.textContent).not.toContain("繼續創作");
});

test("activity shows a source-specific incomplete state without rendering partial items", async () => {
  vi.stubGlobal("fetch", () =>
    json(
      {
        complete: false,
        unavailable_sources: ["evaluation", "publishing"],
        error: "internal owner details",
      },
      503,
    ),
  );

  await render(<Activity />, () => (container.textContent ?? "").includes("沒有顯示部分結果"));

  expect(container.textContent).toContain("Evaluation、Publishing");
  expect(container.textContent).not.toContain("internal owner details");
  expect(container.querySelector(".activity-list")).toBeNull();
});

test("activity keeps an unknown item visible without inventing a continuation", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      complete: true,
      sources: ["run", "evaluation", "creation", "packaging", "publishing"],
      items: [item("future_kind", "future-1", "neutral", { kind: "run" })],
    }),
  );

  await render(<Activity />, () => (container.textContent ?? "").includes("future-1"));

  expect(container.textContent).toContain("其他活動");
  expect(container.textContent).toContain("目前沒有可用的續作入口");
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
