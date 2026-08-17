import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { Downloads } from "./pages/Downloads";
import { Packaging, packagingGate } from "./pages/Packaging";
import type { DownloadArtifact } from "./api/packaging";
import type { SkillDetail } from "./api/types";

// 02:PACK-001 / PACK-002 / WS-002 / WS-004. Same hand-rolled DOM plumbing as
// eval.test.tsx: @testing-library is not a dependency and these assertions do
// not justify adding one.

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

const SKILL = "11111111-1111-1111-1111-111111111111";
const VERSION = "22222222-2222-2222-2222-222222222222";
const ARTIFACT = "33333333-3333-3333-3333-333333333333";

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
  useParams: () => ({ skillId: SKILL }),
  useSearch: () => ({ version: VERSION }),
}));

const skill = {
  skill_id: SKILL,
  name: "CSV 清理",
  summary: "整理 CSV。",
  scope: "private",
  tier: { value: "indexed", label: "已索引", note: "" },
  enrichment: { status: "pending", note: "" },
  limitations: [],
  version: {
    version_id: VERSION,
    version_number: 2,
    content_hash: "sha256:aa",
    created_at: "2026-08-01T00:00:00Z",
  },
  license: { status: { value: "declared", label: "已宣告", note: "" } },
  redistribution: { value: "allowed", label: "可再散布", note: "MIT。" },
  derivation: { is_fork: false, label: "衍生關係", note: "" },
  risk: {
    scan_status: "scanned",
    counts: { errors: 0, warnings: 0, infos: 0 },
    highlights: [],
    info_counts: {},
    note: "",
  },
  compatibility: {
    spec_validation: "passed",
    capability: "unverified",
    runtime: "unverified",
    note: "",
  },
};

const targets = {
  targets: [
    {
      id: "standard",
      kind: "standard_package",
      version: "1.0.0",
      display_name: "標準 Agent Skill 套件",
      support_status: "unverified",
      notes: ["Any spec-compliant agent may try it; Skill Hub has not tried it on yours."],
    },
    {
      id: "claude-agent-sdk",
      kind: "profile",
      version: "1.0.0",
      display_name: "Claude Agent SDK",
      install_location: ".claude/skills/<name>/ (working directory)",
      support_status: "verified",
      notes: [],
    },
  ],
};

const emptyValidation = { blocked: false, errors: [], warnings: [], infos: [] };

const artifact: DownloadArtifact = {
  artifact_id: ARTIFACT,
  skill_id: SKILL,
  skill_version_id: VERSION,
  target: "standard",
  file_name: "csv-cleanup-v2.zip",
  size_bytes: 4096,
  content_hash: "sha256:bbbb",
  manifest_hash: "sha256:cccc",
  status: "available",
  expires_at: "2099-01-01T00:00:00Z",
  created_at: "2026-08-17T00:00:00Z",
  download_count: 1,
  includes_test_cases: false,
  packager_version: "1.0.0",
};

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

/** `blocked` drives the preview; `duplicate` drives what POST .../packaging answers. */
function stubPlatform(options: { blocked?: boolean; duplicate?: boolean } = {}) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/packaging/targets")) return json(targets);
    if (url.includes("/packaging/preview")) {
      return json(
        options.blocked
          ? {
              target: "standard",
              allowed: false,
              blocked_reason: "license_unknown",
              blocked_message: "nobody has established whether this skill may be redistributed",
              validation: emptyValidation,
              included_test_cases: [],
              excluded_test_cases: [
                { test_case_id: "tc1", name: "我上傳的資料", reason: "user-uploaded dataset" },
              ],
            }
          : {
              target: "standard",
              allowed: true,
              validation: {
                blocked: false,
                errors: [],
                warnings: [
                  { code: "frontmatter.long_description", message: "description is long" },
                ],
                infos: [],
              },
              included_test_cases: [],
              excluded_test_cases: [],
            },
      );
    }
    if (url.includes("/packaging") && init?.method === "POST") {
      return json({ ...artifact, duplicate: options.duplicate === true }, 201);
    }
    if (url.includes(`/api/skills/${SKILL}`)) return json(skill);
    return json({ error: "not found" }, 404);
  });
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

// --- the redistribution gate ------------------------------------------------

test("ADR-027 only `allowed` opens the packaging entry, and unknown is refused like blocked", () => {
  const detail = (over: Partial<SkillDetail>) => ({
    ...(skill as unknown as SkillDetail),
    ...over,
  });

  expect(packagingGate(detail({}))).toBeNull();
  expect(packagingGate(detail({ redistribution: { value: "blocked", label: "", note: "" } }))).toBe(
    "not_redistributable",
  );
  expect(packagingGate(detail({ redistribution: { value: "unknown", label: "", note: "" } }))).toBe(
    "license_unknown",
  );
  // Anything unrecognised fails closed rather than releasing.
  expect(packagingGate(detail({ redistribution: { value: "maybe", label: "", note: "" } }))).toBe(
    "license_unknown",
  );
  // The two locks are independent: a hold closes it whatever the licence says.
  expect(
    packagingGate(detail({ access_restriction: { reason: "license-review", note: "" } })),
  ).toBe("license_hold");
  // The field the current detail handler does not send yet is not a third
  // verdict: the server's own gate answers instead.
  expect(packagingGate(detail({ redistribution: undefined }))).toBeNull();
});

// --- the packaging page -----------------------------------------------------

test("PACK-002 an unverified target says so and does not promise the package installs", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));

  expect(text()).toContain("未驗證");
  expect(text()).toContain("沒有把套件裝進這個目標跑過");
  // The measured one keeps its own word, so the two are not shown as one state.
  expect(text()).toContain("已驗證");
});

test("PACK-001 a blocked preview names which lock closed and refuses to offer the build", async () => {
  stubPlatform({ blocked: true });
  await render(<Packaging />, () => text().includes("不能打包"));

  // The reason code, the platform's own sentence, and what it means for the reader.
  expect(text()).toContain("license_unknown");
  expect(text()).toContain("nobody has established whether this skill may be redistributed");
  expect(text()).toContain("授權未知一律當成不可散布處理");
  expect(button("建立下載套件")?.disabled).toBe(true);
  // What will not travel is listed rather than silently missing.
  expect(text()).toContain("我上傳的資料");
});

test("PACK-001 warnings are shown even when packaging is allowed", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("description is long");
  expect(button("建立下載套件")?.disabled).toBe(false);
});

test("PACK-001 an identical package answers 已有相同套件 rather than pretending to build a second", async () => {
  stubPlatform({ duplicate: true });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));
  await act(async () => button("建立下載套件")?.click());
  await waitFor(() => text().includes("已有相同套件"));

  expect(text()).toContain("不是第二份");
  const href = Array.from(container.querySelectorAll("a"))
    .map((a) => a.getAttribute("href") ?? "")
    .find((h) => h.includes("/downloads/"));
  expect(href).toContain(`/downloads/${ARTIFACT}/content`);
});

// --- the download history ---------------------------------------------------

test("WS-004 an expired package stays in the list, says it expired, and offers no bytes", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      downloads: [
        { ...artifact, artifact_id: "expired-1", expires_at: "2026-01-01T00:00:00Z" },
        artifact,
      ],
    }),
  );
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(text()).toContain("已過期");
  expect(text()).toContain("「已過期」與「沒有這一筆」不是同一件事");
  // One row is servable and one is not, so exactly one download link exists.
  const links = Array.from(container.querySelectorAll("a")).filter((a) =>
    (a.getAttribute("href") ?? "").includes("/content"),
  );
  expect(links).toHaveLength(1);
});

test("WS-002 an empty history says nothing was ever downloaded, not that records were cleared", async () => {
  vi.stubGlobal("fetch", () => json({ downloads: [] }));
  await render(<Downloads />, () => text().includes("還沒有打包過任何套件"));

  expect(text()).toContain("不是紀錄被清掉了");
  // No row at all, and therefore nothing marked expired: the two answers do not
  // stand in for one another. (The page's own intro sentence explains the
  // distinction, so this asserts on the badge rather than on the word.)
  expect(container.querySelector(".badge-expired")).toBeNull();
});

test("SEC-006 deleting states its scope first and then deletes", async () => {
  const calls: Array<{ url: string; method?: string }> = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    calls.push({ url: String(input), method: init?.method });
    if (init?.method === "DELETE") return Promise.resolve(new Response(null, { status: 204 }));
    return json({ downloads: calls.some((c) => c.method === "DELETE") ? [] : [artifact] });
  });
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  await act(async () => button("刪除")?.click());
  // The scope is on screen before anything is destroyed (02:WS-002 第 3 條).
  expect(text()).toContain("刪除的是這個套件的檔案本身");
  expect(text()).toContain("紀錄會保留");
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除")?.click());
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.url).toContain(`/downloads/${ARTIFACT}`);
});
