import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { SkillVersionSummary } from "../../../../core/api/types";
import type { RunListItem } from "../../runs.service";
import { CompareCandidatesPicker } from "./CompareCandidatesPicker";

let container: HTMLDivElement;
let root: Root;

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    children,
    ...rest
  }: {
    to: string;
    params: Record<string, string>;
    children: unknown;
    [key: string]: unknown;
  }) => {
    const href = Object.entries(params).reduce(
      (path, [key, value]) => path.replace(`$${key}`, value),
      to,
    );
    return (
      <a href={href} {...rest}>
        {children as never}
      </a>
    );
  },
}));

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
});

const BASE = {
  selfPending: false,
  selfError: null,
  testCaseId: "tc-1",
  siblingsPending: false,
  siblingsError: null,
  candidates: [] as RunListItem[],
  hasMoreCandidates: false,
  loadingMoreCandidates: false,
  onLoadMoreCandidates: () => {},
  versionsPending: false,
  versionsError: null as Error | null,
  versions: [] as SkillVersionSummary[],
  draft: "",
  onDraftChange: () => {},
  onPick: () => {},
};

async function mount(props: Partial<typeof BASE>) {
  root = createRoot(container);
  await act(async () => {
    root.render(
      <StrictMode>
        <CompareCandidatesPicker {...BASE} {...props} />
      </StrictMode>,
    );
  });
}

const text = () => container.textContent ?? "";

function candidate(runId = "run-2", versionId = "version-1"): RunListItem {
  return {
    run_id: runId,
    status: "succeeded",
    evaluation: { value: "met", label: "符合", note: "" },
    skill_id: "skill-1",
    skill_name: "小工具",
    skill_version_id: versionId,
    provider: "sandbox",
    cleanup_status: { value: "done", label: "已清理", note: "" },
    created_at: "2026-01-01T00:00:00Z",
  };
}

test("shows loading for the current run before its own load settles", async () => {
  await mount({ selfPending: true });
  expect(text()).toContain("載入目前這次試跑");
});

test("says the test case can no longer be resolved when the current run has none", async () => {
  await mount({ testCaseId: undefined });
  expect(text()).toContain("這次試跑的測試題已無法解析");
});

test("says there is nothing to compare when the test case has no other runs", async () => {
  await mount({ candidates: [] });
  expect(text()).toContain("這個測試題目前只有這一次試跑紀錄");
});

test("distinguishes candidate runs by immutable Version before choosing one", async () => {
  const onPick = vi.fn();
  await mount({
    candidates: [candidate("run-2", "version-1"), candidate("run-3", "version-2")],
    versions: [
      {
        version_id: "version-1",
        version_number: 1,
        content_hash: "sha256:one",
        created_at: "2025-12-01T00:00:00Z",
      },
      {
        version_id: "version-2",
        version_number: 2,
        content_hash: "sha256:two",
        created_at: "2026-01-01T00:00:00Z",
      },
    ],
    onPick,
  });

  const links = Array.from(container.querySelectorAll("a"));
  expect(links.map((link) => [link.textContent, link.getAttribute("href")])).toEqual([
    ["v1", "/skills/skill-1/versions/version-1"],
    ["v2", "/skills/skill-1/versions/version-2"],
  ]);
  const buttons = Array.from(container.querySelectorAll("button"));
  expect(buttons[0].getAttribute("aria-label")).toContain("v1");
  expect(buttons[1].getAttribute("aria-label")).toContain("v2");
  const button = buttons[0];
  await act(async () => button.click());
  expect(onPick).toHaveBeenCalledWith("run-2");
});

test("gives runs from the same Version distinct accessible comparison names", async () => {
  await mount({
    candidates: [
      candidate("run-2", "version-1"),
      {
        ...candidate("run-3", "version-1"),
        created_at: "2026-01-02T00:00:00Z",
      },
    ],
    versions: [
      {
        version_id: "version-1",
        version_number: 1,
        content_hash: "sha256:one",
        created_at: "2025-12-01T00:00:00Z",
      },
    ],
  });

  const labels = Array.from(container.querySelectorAll("button[aria-label]"), (button) =>
    button.getAttribute("aria-label"),
  );
  expect(labels).toHaveLength(2);
  expect(new Set(labels)).toHaveProperty("size", 2);
  expect(labels[0]).toContain("試跑紀錄 ID run-2");
  expect(labels[1]).toContain("試跑紀錄 ID run-3");
});

test("keeps an unresolved owner Version ID without guessing a version number", async () => {
  await mount({ candidates: [candidate("run-2", "unknown-version")], versions: [] });

  expect(text()).toContain("Version：編號未知");
  expect(text()).toContain("unknown-version");
  expect(text()).not.toContain("v1");
  expect(container.querySelector("a")?.getAttribute("href")).toBe(
    "/skills/skill-1/versions/unknown-version",
  );
});

test("keeps candidate Runs visible while Version numbers are loading", async () => {
  await mount({ candidates: [candidate()], versionsPending: true });

  expect(text()).toContain("載入候選試跑紀錄的 Version 編號");
  expect(text()).toContain("Version：編號載入中");
  expect(text()).not.toContain("Version IDversion-1");
});

test("offers older Runs when the current 測試題 has another page", async () => {
  const onLoadMoreCandidates = vi.fn();
  await mount({
    candidates: [candidate()],
    hasMoreCandidates: true,
    onLoadMoreCandidates,
  });

  const loadMore = Array.from(container.querySelectorAll("button")).find(
    (item) => item.textContent === "載入更早的試跑紀錄",
  )!;
  await act(async () => loadMore.click());
  expect(onLoadMoreCandidates).toHaveBeenCalledOnce();
});

test("keeps the older-試跑紀錄 control disabled while its next page is loading", async () => {
  await mount({
    candidates: [candidate()],
    hasMoreCandidates: true,
    loadingMoreCandidates: true,
  });

  const loadMore = Array.from(container.querySelectorAll("button")).find(
    (item) => item.textContent === "載入中…",
  );
  expect(loadMore?.disabled).toBe(true);
});

test("a Version read failure is not presented as a resolved or absent Version", async () => {
  await mount({
    candidates: [candidate()],
    versionsError: new Error("service unavailable"),
  });

  expect(container.querySelector('[role="alert"]')?.textContent).toContain(
    "候選試跑紀錄的 Version 編號",
  );
  expect(text()).toContain("Version：編號未知");
  expect(text()).toContain("version-1");
  expect(text()).not.toContain("service unavailable");
});
