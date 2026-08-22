import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { Downloads } from "./pages/Downloads";
import { PackagingBlockedReason as PackagingBlockedReasonEnum } from "@skillhub/api-client-ts";
import { PACKAGING_BLOCKED_LABEL, Packaging, packagingGate } from "./pages/Packaging";
import type { DownloadArtifact, PackagingBlockedReason } from "./api/packaging";
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
const OLDER_VERSION = "44444444-4444-4444-4444-444444444444";
const ARTIFACT = "33333333-3333-3333-3333-333333333333";

// GET /skills/{id}/versions, newest first (04 丙-14).
const VERSIONS = {
  versions: [
    {
      version_id: VERSION,
      version_number: 2,
      content_hash: "sha256:bb",
      created_at: "2026-08-02T00:00:00Z",
    },
    {
      version_id: OLDER_VERSION,
      version_number: 1,
      content_hash: "sha256:aa",
      created_at: "2026-08-01T00:00:00Z",
    },
  ],
};

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
    spec_validation: { value: "passed", label: "通過", note: "" },
    capability: { value: "unverified", label: "未驗證", note: "" },
    runtime: { value: "unverified", label: "未驗證", note: "" },
    note: "",
  },
};

// `env_vars` is required by the contract, so both of its answers are here: empty
// (this target needs none) and populated.
const targets = {
  targets: [
    {
      id: "standard",
      kind: "standard_package",
      version: "1.0.0",
      display_name: "標準 Agent Skill 套件",
      support_status: "unverified",
      // No prompt: this target names no agent to run one against.
      verification_steps: ["Unzip the package. SKILL.md must be at the root of the archive."],
      notes: ["Any spec-compliant agent may try it; Skill Hub has not tried it on yours."],
      env_vars: [],
    },
    {
      id: "claude-code",
      kind: "profile",
      version: "1.0.0",
      display_name: "Claude Code",
      install_location: "~/.claude/skills/<name>/ (user)",
      support_status: "unverified",
      verification_prompt: "/skills",
      notes: [],
      env_vars: [],
    },
    {
      id: "claude-agent-sdk",
      kind: "profile",
      version: "1.0.0",
      display_name: "Claude Agent SDK",
      install_location: ".claude/skills/<name>/ (working directory)",
      support_status: "verified",
      verification_prompt: "List the skills you can use.",
      verification_steps: ["Set cwd to the directory holding .claude/skills/."],
      notes: [],
      env_vars: [
        {
          name: "ANTHROPIC_API_KEY",
          required: true,
          description: "The SDK reads the key from your own environment.",
          // A placeholder, never a credential shape: the same string is rendered
          // verbatim into INSTALL.md, which ships inside packages (iron rule 11).
          example: "<your own key>",
        },
      ],
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
  servable: true,
  serve_state: { value: "available", label: "可下載", note: "" },
  version_number: 2,
  latest_version_number: 2,
  version_state: { value: "current", label: "v2（最新）", note: "" },
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
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    calls.push(url);
    if (url.endsWith("/versions")) return json(VERSIONS);
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
              // A gate closed before any bytes were read, so there is nothing to
              // have dependencies — not a package that has none.
              dependencies: [],
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
              // The lines the produced INSTALL.md carries, verbatim.
              dependencies: [
                "SKILL.md: package evidences 2 third-party dependencies: pandas, openpyxl",
                "pandas",
                "openpyxl",
              ],
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
  return calls;
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
  // A response missing the field the contract requires is a platform that failed
  // to answer, not a permission. It refuses, like every other non-`allowed` case.
  expect(packagingGate(detail({ redistribution: undefined }))).toBe("license_unknown");
});

test("every refusal the contract can send has a sentence on this page", () => {
  // The label table's own comment says two copies of it would drift. One did:
  // `file_removed_by_packager` was added to the contract and to the generated
  // client, and this hand-written union kept four values — so a real refusal
  // rendered 「不能打包：」 followed by nothing, which is the blank §2.1 forbids,
  // in the one place a reader most needs a sentence.
  //
  // Asserted against the generated enum rather than a list written here, because
  // a list written here is the same drift one file over.
  for (const value of Object.values(PackagingBlockedReasonEnum)) {
    const label = PACKAGING_BLOCKED_LABEL[value as PackagingBlockedReason];
    expect(label, `no sentence for blocked_reason ${value}`).toBeTruthy();
  }
});

test("PACK-002 the post-install check is on the page, not only inside the package", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));

  // Both shapes: the standard package's steps, and the profile's prompt. The
  // INSTALL.md sentence stays as a supplement rather than as the whole answer.
  expect(text()).toContain("SKILL.md must be at the root of the archive");
  expect(text()).toContain("List the skills you can use.");
  expect(text()).toContain("裝好之後怎麼確認");
  expect(text()).toContain("隨套件內的 INSTALL.md 一起下載");
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

test("DESIGN-012 the three compatibility axes are on the packaging page and stay apart", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("這個版本的相容性"));

  // The same component the skill page uses, so the two surfaces cannot describe
  // one measurement differently. An axis with no answer says 未驗證 rather than
  // being hidden — a missing row reads as "fine" and 未驗證 does not.
  expect(text()).toContain("規格驗證：通過");
  expect(text()).toContain("能力相容：未驗證");
  expect(text()).toContain("實測相容：未驗證");
  // And the page refuses to let one axis be read as another.
  expect(text()).toContain("「規格驗證通過」不等於「裝得起來」");
});

test("PACK-002 環境變數需求 is on the target, and 「不需要」 is stated rather than left blank", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));

  // Empty on two targets: they genuinely need none, and the page says so.
  expect(text()).toContain("這個目標不需要任何環境變數");
  // Populated on the SDK target, with required/optional stated per variable.
  expect(text()).toContain("ANTHROPIC_API_KEY");
  expect(text()).toContain("（必要）");
  expect(text()).toContain("套件裡不會有任何金鑰");
});

test("PACK-002 依賴需求 shows the same lines the package's INSTALL.md will carry", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("依賴需求"));

  expect(text()).toContain("pandas");
  expect(text()).toContain("openpyxl");
  // Served, not derived here: the page and the packaged document read one list.
  expect(text()).toContain("同一份清單會寫進套件內的 INSTALL.md");
  expect(text()).toContain("Skill Hub 不會替你安裝這些");
});

test("PACK-002 an empty dependency list means two different things and is never printed as one", async () => {
  stubPlatform({ blocked: true }); // a gate closed before any bytes were read
  await render(<Packaging />, () => text().includes("依賴需求"));

  expect(text()).toContain("還沒有讀到套件內容");
  expect(text()).not.toContain("沒有宣告依賴檔");
});

// --- the download history ---------------------------------------------------

test("WS-004 an expired package stays in the list, says it expired, and offers no bytes", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      downloads: [
        {
          ...artifact,
          artifact_id: "expired-1",
          // Servability is the server's answer now (04 丙-29 ⑤) — it checks a
          // purge this shape cannot see — so an expired fixture states it the
          // way the API would rather than leaving the date to imply it.
          servable: false,
          serve_state: {
            value: "expired",
            label: "已過期,不再提供下載",
            note: "檔案已刪除,這筆紀錄保留。",
          },
          expires_at: "2026-01-01T00:00:00Z",
        },
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

// --- 04 丙-14: the version picker -------------------------------------------

test("04 丙-14 the packaging page picks the version from a list, and ?version= is the default", async () => {
  const calls = stubPlatform();
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  const select = container.querySelector<HTMLSelectElement>("select")!;
  // The URL named a version and that is what is selected — not the first row of
  // the list, and not the skill's latest by accident: the page previously had
  // no control here at all and only ever read the search param.
  expect(select.value).toBe(VERSION);
  expect(text()).toContain("v2（最新）");
  expect(text()).toContain("v1");

  // Picking another version re-previews that version: PACK-001 packages one
  // immutable version, so a preview belonging to a different one must not stand.
  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(select, OLDER_VERSION);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() =>
    calls.some((u) => u.includes(`/versions/${OLDER_VERSION}/packaging/preview`)),
  );
  expect(text()).toContain(OLDER_VERSION);
  // And the "最新版本" label goes with it, rather than labelling an older version
  // as the latest.
  expect(text()).not.toContain("最新版本）");
});
