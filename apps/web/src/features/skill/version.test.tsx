import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import {
  OWN_PUBLICATION,
  OWN_PUBLICATIONS,
  OWN_PUBLISHER,
  SKILL,
  SKILL_VERSIONS,
  VERSION,
  VERSION_DIFF,
  skillDetail,
} from "../../testing/fixtures/platform";
import { SkillVersion } from "./version/SkillVersion.page";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";

let container: HTMLDivElement;
let root: Root;
let routeVersion = VERSION;

const RUN_ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const TEST_CASE_ID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    children,
    className,
    ...rest
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    children?: unknown;
    className?: string;
    [key: string]: unknown;
  }) => {
    const path = Object.entries(params ?? {}).reduce(
      (current, [key, value]) => current.replace(`$${key}`, value),
      to,
    );
    const query = new URLSearchParams(
      Object.entries(search ?? {}).filter((entry): entry is [string, string] => Boolean(entry[1])),
    );
    return (
      <a className={className} href={`${path}${query.size ? `?${query}` : ""}`} {...rest}>
        {children as never}
      </a>
    );
  },
  useParams: () => ({ skillId: SKILL, versionId: routeVersion }),
}));

beforeEach(() => {
  routeVersion = VERSION;
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

function runItem(overrides: Record<string, unknown> = {}) {
  return {
    run_id: RUN_ID,
    status: "succeeded",
    evaluation: { value: "not_evaluated", label: "未評估", note: "尚未產生判定。" },
    skill_id: SKILL,
    skill_name: "PDF Summariser",
    skill_version_id: VERSION,
    test_case_id: TEST_CASE_ID,
    provider: "sandbox",
    cleanup_status: { value: "cleaned", label: "已清理", note: "隔離環境已清理。" },
    created_at: "2026-09-28T10:00:00Z",
    finished_at: "2026-09-28T10:01:00Z",
    ...overrides,
  };
}

function downloadArtifact(overrides: Record<string, unknown> = {}) {
  return {
    artifact_id: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
    skill_id: SKILL,
    skill_version_id: VERSION,
    target: "standard",
    file_name: "pdf-summariser-v2.zip",
    size_bytes: 2048,
    content_hash: "content-hash",
    manifest_hash: "manifest-hash",
    status: "available",
    servable: true,
    serve_state: { value: "available", label: "可下載", note: "" },
    version_number: 2,
    latest_version_number: 2,
    version_state: { value: "current", label: "目前版本", note: "" },
    expires_at: "2026-10-28T10:00:00Z",
    created_at: "2026-09-28T10:00:00Z",
    download_count: 0,
    includes_test_cases: true,
    ...overrides,
  };
}

function stubVersions(
  versions = SKILL_VERSIONS,
  runs: { body?: unknown; status?: number } = { body: { runs: [] } },
  creation?: { enabled?: boolean; body?: unknown; status?: number },
  downloads: { body?: unknown; status?: number } = { body: { downloads: [] } },
) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    calls.push(url);
    const path = url.split("?")[0];
    if (path === "/me") {
      return json({ features: { creation_skill: creation?.enabled === true } });
    }
    if (path === "/creation-sessions") {
      return json(creation?.body ?? [], creation?.status ?? 200);
    }
    if (path === `/api/skills/${SKILL}`) return json(skillDetail(SKILL, "PDF Summariser"));
    if (path === `/skills/${SKILL}/versions`) return json(versions);
    if (path === "/me/publisher") return json(OWN_PUBLISHER);
    if (path === `/skills/${SKILL}/publication`) return json(OWN_PUBLICATION);
    if (path === "/me/publications") return json(OWN_PUBLICATIONS);
    if (path === `/skills/${SKILL}/diff`) return json(VERSION_DIFF);
    if (path === "/runs") return json(runs.body ?? { error: "unavailable" }, runs.status ?? 200);
    if (path === "/downloads") {
      return json(downloads.body ?? { error: "unavailable" }, downloads.status ?? 200);
    }
    return json({ error: "not found" }, 404);
  });
  return calls;
}

function creationSession() {
  return {
    id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
    revision: 7,
    state: "saved",
    snapshot: {
      messages: [],
      brief: "整理採購文件並產生摘要",
      brief_confirmed: true,
      acceptance_criteria: [],
      diagram_understanding: "",
      diagram_confirmed: false,
      references: [],
      pending_action: "",
      budget_credits: 650,
      reserved_credits: 0,
      usage_unknown: false,
      steps: 4,
      tool_calls: 1,
      candidate: { skill_id: SKILL, version_id: VERSION },
    },
    created_at: "2026-09-27T09:00:00Z",
    updated_at: "2026-09-28T09:30:00Z",
    expires_at: "2026-12-27T09:00:00Z",
    deadline: "2026-09-27T10:00:00Z",
  };
}

async function render(settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <SkillVersion />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(settled);
}

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const text = () => container.textContent ?? "";

async function settleReads() {
  await waitFor(
    () =>
      queryClient.getQueryState(queryKeys.me)?.status === "success" && !queryClient.isFetching(),
  );
  await act(async () => {});
}

test("an owned immutable version becomes one shareable context for validation, package and release", async () => {
  stubVersions();
  await render(() => text().includes("已列入 Catalog"));

  expect(text()).toContain("v2，最新版本");
  expect(text()).toContain("已有 Release");
  expect(text()).toContain("已列入 Catalog");
  expect(container.querySelector("#skill-version-file")).not.toBeNull();
  expect(
    container.querySelector(`a[href="/lab/test-cases?skill=${SKILL}&version=${VERSION}"]`),
  ).not.toBeNull();
  expect(
    container.querySelector(`a[href="/skills/${SKILL}/package?version=${VERSION}"]`),
  ).not.toBeNull();
  expect(
    container.querySelector(`a[href="/workspace/downloads?bundleVersion=${VERSION}"]`),
  ).not.toBeNull();
  expect(
    container.querySelector(`a[href="/skills/${SKILL}/versions/${VERSION}"][aria-current="page"]`),
  ).not.toBeNull();
  const facts = container.querySelector("#version-facts-title")!.closest("section")!;
  const continuation = container.querySelector("#version-continuation-title")!.closest("section")!;
  const evidence = container.querySelector("#version-evidence-title")!.closest("section")!;
  expect(facts.compareDocumentPosition(continuation) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(
    0,
  );
  expect(
    continuation.compareDocumentPosition(evidence) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0);
  await settleReads();
  expect(text()).not.toContain("Activity");
  expect(text()).not.toContain("Studio 歷程");
});

test("a historical version labels the current 小工具 summary without presenting it as version content", async () => {
  routeVersion = SKILL_VERSIONS.versions[1].version_id;
  stubVersions();
  await render(() => text().includes("v1，歷史版本"));

  const summary = container.querySelector('[data-role="skill-current-summary"]');
  expect(summary?.textContent).toContain("小工具目前說明");
  expect(summary?.textContent).toContain("把 PDF 整理成摘要");
});

test("a version from another 小工具 cannot expose publish, upload or package actions", async () => {
  routeVersion = "99999999-9999-4999-8999-999999999999";
  const calls = stubVersions();
  await render(() => text().includes("無法開啟這個版本"));

  expect(text()).toContain("這個版本不屬於目前的小工具");
  expect(container.querySelector("#skill-version-file")).toBeNull();
  expect(container.querySelector('a[href*="/package"]')).toBeNull();
  expect(calls).not.toContain("/me/publisher");
  expect(calls).not.toContain(`/skills/${SKILL}/publication`);
  expect(calls.some((call) => call.startsWith("/runs?"))).toBe(false);
  expect(calls).not.toContain("/downloads");
});

test("a version keeps only its exact 小工具 delivery artifacts and preserves owner state", async () => {
  const artifact = downloadArtifact({
    servable: false,
    serve_state: { value: "expired", label: "已過期", note: "檔案已刪除。" },
  });
  stubVersions(SKILL_VERSIONS, { body: { runs: [] } }, undefined, {
    body: {
      downloads: [
        artifact,
        downloadArtifact({
          artifact_id: "ffffffff-ffff-4fff-8fff-ffffffffffff",
          skill_id: "11111111-1111-4111-8111-111111111111",
        }),
        downloadArtifact({
          artifact_id: "22222222-2222-4222-8222-222222222222",
          skill_version_id: "33333333-3333-4333-8333-333333333333",
        }),
        downloadArtifact({
          artifact_id: "44444444-4444-4444-8444-444444444444",
          skill_id: undefined,
          skill_version_id: undefined,
          plugin: { name: "review-kit", version: "1.0.0", members: [] },
          file_name: "review-kit.zip",
        }),
      ],
    },
  });
  await render(() => text().includes("pdf-summariser-v2.zip"));

  expect(text()).toContain("這一版的交付套件");
  expect(text()).toContain("已過期");
  expect(text()).not.toContain("review-kit.zip");
  expect(container.querySelectorAll("[data-version-deliverable]")).toHaveLength(1);
  expect(
    container.querySelector(`a[href="/workspace/downloads?artifact=${artifact.artifact_id}"]`),
  ).not.toBeNull();
});

test("a delivery history read failure is not presented as no package for the version", async () => {
  stubVersions(SKILL_VERSIONS, { body: { runs: [] } }, undefined, { status: 503 });
  await render(() => Boolean(container.querySelector('[role="alert"]')));

  expect(text()).toContain("暫時無法讀取這一版的交付套件");
  expect(text()).not.toContain("這一版還沒有交付套件");
});

test("a version with no delivery artifacts states bounded absence", async () => {
  stubVersions();
  await render(() => text().includes("這一版還沒有交付套件"));

  expect(text()).toContain("完成打包後，套件與保留狀態會留在這裡");
});

test("an immutable version shows exact run evidence without collapsing execution and verdict", async () => {
  const calls = stubVersions(SKILL_VERSIONS, {
    body: {
      runs: [
        runItem(),
        runItem({
          run_id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
          status: "failed",
          status_reason: "Sandbox stopped before completion.",
          evaluation: { value: "partially_met", label: "部分符合", note: "部分條件有證據。" },
          failure_class: { value: "sandbox", label: "隔離環境", note: "執行環境已停止。" },
        }),
      ],
    },
  });
  await render(() => text().includes("驗證證據"));

  expect(calls.some((call) => call.includes(`skill_version_id=${VERSION}`))).toBe(true);
  expect(text()).toContain("執行狀態：執行完成");
  expect(text()).toContain("任務判定：未評估");
  expect(text()).not.toContain("任務判定：符合");
  expect(text()).toContain("執行狀態：執行失敗");
  expect(text()).toContain("任務判定：部分符合");
  expect(text()).toContain("Sandbox stopped before completion.");
  expect(container.querySelector(`a[href="/runs/${RUN_ID}"]`)).not.toBeNull();
  expect(
    container.querySelector(`a[href="/lab/test-cases/${TEST_CASE_ID}?version=${VERSION}"]`),
  ).not.toBeNull();
});

test("a version with no runs states exact absence and keeps the validation exit", async () => {
  stubVersions();
  await render(() => text().includes("這個版本還沒有試跑紀錄"));

  expect(text()).toContain("這個版本還沒有試跑紀錄");
  expect(
    container.querySelector(`a[href="/lab/test-cases?skill=${SKILL}&version=${VERSION}"]`),
  ).not.toBeNull();
});

test("a run evidence read failure is not presented as an empty history", async () => {
  stubVersions(SKILL_VERSIONS, { status: 503 });
  await render(() => text().includes("暫時無法讀取這個版本的試跑證據"));

  expect(text()).not.toContain("這個版本還沒有試跑紀錄");
});

test("an in-flight version run exposes freshness and refresh", async () => {
  stubVersions(SKILL_VERSIONS, {
    body: { runs: [runItem({ status: "running", finished_at: undefined })] },
  });
  await render(() => text().includes("有試跑還在進行中"));

  expect(container.querySelector("button")?.textContent).toContain("重新整理");
});

test("an empty owner-scoped version list is absence of access, not absence of history", async () => {
  stubVersions({ versions: [] });
  await render(() => text().includes("無法開啟這個版本"));

  expect(text()).toContain("無權檢視");
  expect(text()).toContain("這不代表它沒有版本");
});

test("a retained creation session gives the immutable version a Studio continuation", async () => {
  const session = creationSession();
  const calls = stubVersions(
    SKILL_VERSIONS,
    { body: { runs: [] } },
    {
      enabled: true,
      body: [session],
    },
  );
  await render(() => text().includes("整理採購文件並產生摘要"));

  expect(calls).toContain(`/creation-sessions?version_id=${VERSION}`);
  expect(text()).toContain("Studio 歷程");
  expect(
    container.querySelector(`a[href="/workspace/creations?session=${session.id}"]`),
  ).not.toBeNull();
  expect(container.querySelector(".version-creation-list time")?.getAttribute("datetime")).toBe(
    session.updated_at,
  );
});

test("the immutable version exposes no Studio context while creation is disabled", async () => {
  const calls = stubVersions();
  await render(() => text().includes("PDF Summariser v2"));
  await settleReads();

  expect(text()).not.toContain("Studio 歷程");
  expect(calls.some((call) => call.startsWith("/creation-sessions"))).toBe(false);
});

test("a version without a retained creation session explains the bounded absence", async () => {
  stubVersions(SKILL_VERSIONS, { body: { runs: [] } }, { enabled: true, body: [] });
  await render(() => text().includes("沒有仍可開啟的 Studio 會話"));

  expect(text()).toContain("其他方式建立");
  expect(text()).toContain("超過保存期限");
});

test("a creation-context read failure is not presented as no retained session", async () => {
  stubVersions(SKILL_VERSIONS, { body: { runs: [] } }, { enabled: true, status: 503 });
  await render(() => Boolean(container.querySelector('[role="alert"]')));

  expect(text()).toContain("這個版本的 Studio 歷程");
  expect(text()).not.toContain("沒有仍可開啟的 Studio 會話");
});
