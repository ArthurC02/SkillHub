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

const searchListeners = new Set<() => void>();
let search: Record<string, string | undefined> = { version: VERSION };

function setSearch(next: Record<string, string | undefined>) {
  search = next;
  for (const listener of searchListeners) listener();
}

beforeEach(() => setSearch({ version: VERSION }));

vi.mock("@tanstack/react-router", async () => {
  const { useSyncExternalStore } = await import("react");
  return {
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
    useSearch: () =>
      useSyncExternalStore(
        (listener: () => void) => {
          searchListeners.add(listener);
          return () => searchListeners.delete(listener);
        },
        () => search,
      ),
    useNavigate: () => (options: { search?: unknown }) => {
      const next =
        typeof options.search === "function"
          ? (options.search as (prev: typeof search) => typeof search)(search)
          : (options.search as typeof search);
      setSearch({ ...next });
      return Promise.resolve();
    },
  };
});

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
    disclosures: [],
    note: "",
  },
  compatibility: {
    spec_validation: { value: "passed", label: "通過", note: "" },
    capability: { value: "unverified", label: "未驗證", note: "" },
    runtime: { value: "unverified", label: "未驗證", note: "" },
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
      verification_steps: [
        "解壓縮套件。SKILL.md 必須位於壓縮檔的根層。（原文：Unzip the package. SKILL.md must be at the root of the archive.）",
      ],
      notes: [
        "任何符合規格的 Agent 都可以試，Skill Hub 沒有在你的 Agent 上試過。（原文：Any spec-compliant agent may try it; Skill Hub has not tried it on yours.）",
      ],
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
      verification_steps: [
        "把 cwd 設成放著 .claude/skills/ 的那個目錄。（原文：Set cwd to the directory holding .claude/skills/.）",
      ],
      notes: [],
      env_vars: [
        {
          name: "ANTHROPIC_API_KEY",
          required: true,
          description:
            "SDK 從你自己的環境讀取這個金鑰。（原文：The SDK reads the key from your own environment.）",
          example: "<your own key>",
        },
      ],
    },
  ],
};

const emptyValidation = { blocked: false, errors: [], warnings: [], infos: [] };

const EXCLUDED_TEST_CASE = {
  test_case_id: "tc1",
  name: "我上傳的資料",
  reason: "user_uploaded_dataset",
  label: "含你上傳的資料集",
  note: "你上傳的資料不能隨套件散布（授權未定），這個 Test Case 因此不打包。",
};

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

function stubPlatform(
  options: {
    blocked?: boolean;
    duplicate?: boolean;
    retentionDays?: number | "absent";
    excludedFiles?: {
      path: string;
      reason: string;
      label: string;
      note: string;
    }[];
    excludedTestCases?: {
      test_case_id: string;
      name: string;
      reason: string;
      label: string;
      note: string;
    }[];
    skill?: typeof skill;
  } = {},
) {
  const retention = options.retentionDays === "absent" ? undefined : (options.retentionDays ?? 23);
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
              blocked_message: "沒有人確認過這個 Skill 可不可以再散布，未確認的授權視同不允許",
              validation: emptyValidation,
              dependencies: [],
              included_test_cases: [],
              excluded_test_cases: options.excludedTestCases ?? [EXCLUDED_TEST_CASE],
              excluded_files: options.excludedFiles ?? [],
              retention_days: retention,
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
              dependencies: [
                "SKILL.md: package evidences 2 third-party dependencies: pandas, openpyxl",
                "pandas",
                "openpyxl",
              ],
              included_test_cases: [],
              excluded_test_cases: [],
              excluded_files: options.excludedFiles ?? [],
              retention_days: retention,
            },
      );
    }
    if (url.includes("/packaging") && init?.method === "POST") {
      return json({ ...artifact, duplicate: options.duplicate === true }, 201);
    }
    if (url.includes(`/api/skills/${SKILL}`)) return json(options.skill ?? skill);
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

function occurrences(needle: string): number {
  return text().split(needle).length - 1;
}

test("ADR-027 only `allowed` opens the packaging entry, and unknown is refused like blocked", () => {
  const detail = (over: Partial<SkillDetail>) => ({
    ...(skill as unknown as SkillDetail),
    ...over,
  });

  expect(packagingGate(detail({}))).toBeNull();
  expect(
    packagingGate(detail({ redistribution: { value: "self_supplied", label: "", note: "" } })),
  ).toBeNull();
  expect(
    packagingGate(detail({ redistribution: { value: "generated", label: "", note: "" } })),
  ).toBeNull();
  expect(packagingGate(detail({ redistribution: { value: "blocked", label: "", note: "" } }))).toBe(
    "not_redistributable",
  );
  expect(packagingGate(detail({ redistribution: { value: "unknown", label: "", note: "" } }))).toBe(
    "license_unknown",
  );
  expect(packagingGate(detail({ redistribution: { value: "maybe", label: "", note: "" } }))).toBe(
    "license_unknown",
  );
  expect(
    packagingGate(detail({ access_restriction: { reason: "license-review", note: "" } })),
  ).toBe("license_hold");
  expect(packagingGate(detail({ redistribution: undefined }))).toBe("license_unknown");
});

test("every refusal the contract can send has a sentence on this page", () => {
  for (const value of Object.values(PackagingBlockedReasonEnum)) {
    const label = PACKAGING_BLOCKED_LABEL[value as PackagingBlockedReason];
    expect(label, `no sentence for blocked_reason ${value}`).toBeTruthy();
  }
});

test("PACK-002 the post-install check is on the page, not only inside the package", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));

  expect(text()).toContain("SKILL.md must be at the root of the archive");
  expect(text()).toContain("List the skills you can use.");
  expect(text()).toContain("裝好之後怎麼確認");
  expect(text()).toContain("隨套件內的 INSTALL.md 一起下載");
});

test("PACK-002 an unverified target says so and does not promise the package installs", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));

  expect(text()).toContain("未驗證");
  expect(text()).toContain("沒有把套件裝進這個目標跑過");
  expect(text()).toContain("已驗證");
});

test("PACK-001 a blocked preview names which lock closed and refuses to offer the build", async () => {
  stubPlatform({ blocked: true });
  await render(<Packaging />, () => text().includes("不能打包"));

  expect(text()).toContain("license_unknown");
  expect(text()).toContain("沒有人確認過這個 Skill 可不可以再散布，未確認的授權視同不允許");
  expect(text()).toContain("授權未知一律當成不可散布處理");
  expect(button("建立下載套件")?.disabled).toBe(true);
  expect(text()).toContain("我上傳的資料");
});

test("丙-154① 不會進包的 Test Case 印 label/note，不印機器碼 reason", async () => {
  stubPlatform({
    blocked: true,
    excludedTestCases: [
      {
        test_case_id: "tc2",
        name: "我的 CSV 清理測試",
        reason: "not_curated",
        label: "未經策展",
        note: "只有平台策展的 Test Case 會隨套件散布，你自己的 Test Case 留在工作區。",
      },
    ],
  });
  await render(<Packaging />, () => text().includes("不能打包"));

  expect(text()).toContain("未經策展");
  expect(text()).not.toContain("not_curated");
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

  expect(text()).toContain("規格驗證：通過");
  expect(text()).toContain("能力相容：未驗證");
  expect(text()).toContain("執行環境相容：未驗證");
  expect(text()).not.toContain("實測相容");
  expect(text()).toContain("「規格驗證通過」不等於「裝得起來」");
});

function elementSaying(needle: string): Element {
  const found = Array.from(container.querySelectorAll("h1,h2,h3,p,li,span,code,strong,a")).find(
    (el) => (el.textContent ?? "").includes(needle) && el.children.length < 4,
  );
  expect(found, `找不到「${needle}」——這一句在頁面上消失了，不只是被折起來`).toBeDefined();
  return found!;
}

const SKILL_WITH_DETAILS = {
  ...skill,
  license: {
    ...skill.license,
    expression: "MIT",
    source: "repo-license-file",
    source_note: "來自 repo 根目錄的 LICENSE。",
  },
  risk: {
    scan_status: "scanned",
    counts: { errors: 1, warnings: 2, infos: 5 },
    highlights: [
      { severity: "error", code: "embedded-script", message: "SKILL.md 內含可執行程式碼區塊。" },
    ],
    info_counts: { "external-url": 5 },
    disclosures: [{ code: "script-file", label: "含可執行 Script 檔案", note: "細項備註。" }],
    note: "以上為靜態掃描結果。",
  },
  compatibility: {
    spec_validation: { value: "passed", label: "通過", note: "" },
    capability: { value: "activated", label: "已啟用", note: "" },
    runtime: {
      value: "transpiled",
      label: "腳本未執行,由模型轉譯",
      note: "套件宣告的 Runtime 這個映像沒有,而觀察到的結果來自模型重寫程式碼、不是執行它。",
    },
    runtime_image: "ghcr.io/skillhub/runtime:2026.08-3",
    measured_at: "2026-08-10T00:00:00Z",
    note: "以上為單次沙箱實測。",
  },
} as unknown as typeof skill;

test("04 R-42(c)③ 風險與 License：判定行與最高嚴重度留在外面，逐項細節折進 <details>", async () => {
  stubPlatform({ skill: SKILL_WITH_DETAILS });
  await render(<Packaging />, () => text().includes("打包與下載"));

  for (const verdict of ["有 8 項風險，最高為錯誤。", "可再散布", "已宣告"]) {
    expect(
      elementSaying(verdict).closest("details"),
      `「${verdict}」是判定行，不准折進 <details>`,
    ).toBeNull();
  }

  for (const detail of [
    "SKILL.md 內含可執行程式碼區塊。",
    "含可執行 Script 檔案",
    "來自 repo 根目錄的 LICENSE。",
  ]) {
    expect(
      elementSaying(detail).closest("details"),
      `「${detail}」是細項，應該折進 <details> 裡`,
    ).not.toBeNull();
  }
});

test("風險判定行：只有警告時說最高為警告，只有提示時說最高為提示", async () => {
  stubPlatform({
    skill: {
      ...skill,
      risk: { ...skill.risk, counts: { errors: 0, warnings: 2, infos: 0 } },
    },
  });
  await render(<Packaging />, () => text().includes("打包與下載"));

  expect(text()).toContain("有 2 項風險，最高為警告。");
});

test("風險判定行：只有提示時說最高為提示", async () => {
  stubPlatform({
    skill: {
      ...skill,
      risk: { ...skill.risk, counts: { errors: 0, warnings: 0, infos: 3 } },
    },
  });
  await render(<Packaging />, () => text().includes("打包與下載"));

  expect(text()).toContain("有 3 項風險，最高為提示。");
});

test("04 R-42(c)③ 相容性：三軸的驗證狀態留在外面，逐軸備註與實測環境折進 <details>", async () => {
  stubPlatform({ skill: SKILL_WITH_DETAILS });
  await render(<Packaging />, () => text().includes("這個版本的相容性"));

  for (const verdict of [
    "規格驗證：通過",
    "能力相容：已啟用",
    "執行環境相容：腳本未執行,由模型轉譯",
  ]) {
    expect(
      elementSaying(verdict).closest("details"),
      `「${verdict}」是驗證狀態，不准折進 <details>`,
    ).toBeNull();
  }

  for (const detail of [
    "套件宣告的 Runtime 這個映像沒有",
    "ghcr.io/skillhub/runtime:2026.08-3",
    "以上為單次沙箱實測。",
  ]) {
    expect(
      elementSaying(detail).closest("details"),
      `「${detail}」是相容性細項，應該折進 <details> 裡`,
    ).not.toBeNull();
  }
});

test("PACK-002 環境變數需求 is on the target, and 「不需要」 is stated rather than left blank", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));

  expect(text()).toContain("這個目標不需要任何環境變數");
  expect(text()).toContain("ANTHROPIC_API_KEY");
  expect(text()).toContain("（必要）");
  expect(text()).toContain("套件裡不會有任何金鑰");
});

test("PACK-002 打包器拿掉的檔案要說出來，空與非空是兩個答案", async () => {
  stubPlatform({
    excludedFiles: [
      {
        path: "node_modules/",
        reason: "excluded_dir",
        label: "依目錄規則排除",
        note: "打包器不收 node_modules/。",
      },
    ],
  });
  await render(<Packaging />, () => text().includes("打包器拿掉的檔案"));

  expect(text()).toContain("node_modules/");
  expect(text()).toContain("依目錄規則排除");
  expect(text()).toContain("打包器不收 node_modules/。");
  expect(text()).not.toContain("沒有檔案被排除");

  await act(async () => root?.unmount());
  container.innerHTML = "";
  queryClient.clear();

  stubPlatform();
  await render(<Packaging />, () => text().includes("打包器拿掉的檔案"));
  expect(text()).toContain("沒有檔案被排除，這一份帶走的就是版本裡的全部內容");
});

test("PACK-001 放行的時候也要說出授權判定，不是只在拒絕時才談", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("打包與下載"));

  expect(text()).toContain("可再散布");
  expect(text()).toContain("已宣告");
});

test("PACK-002 依賴需求 shows the same lines the package's INSTALL.md will carry", async () => {
  stubPlatform();
  await render(<Packaging />, () => text().includes("依賴需求"));

  expect(text()).toContain("pandas");
  expect(text()).toContain("openpyxl");
  expect(text()).toContain("Skill Hub 不會替你安裝這些");
  expect(text()).toContain("隨套件內的 INSTALL.md 一起下載");
});

test("PACK-002 an empty dependency list means two different things and is never printed as one", async () => {
  stubPlatform({ blocked: true });
  await render(<Packaging />, () => text().includes("依賴需求"));

  expect(text()).toContain("還沒有讀到套件內容");
  expect(text()).not.toContain("沒有宣告依賴檔");
});

test("PACK-011 保留期限 is the server's number and it arrives before the build button", async () => {
  stubPlatform({ retentionDays: 23 });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("保留期限");
  expect(text()).toContain("23 天");

  const notice = Array.from(container.querySelectorAll("p")).find((p) =>
    (p.textContent ?? "").includes("保留期限"),
  )!;
  expect(notice).toBeTruthy();
  expect(
    notice.compareDocumentPosition(button("建立下載套件")!) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();

  expect(text()).toContain("打包是冪等的");
});

test("PACK-011 a retention under one day says 不到 1 天 rather than 0 天", async () => {
  stubPlatform({ retentionDays: 0 });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("不到 1 天");
  expect(text()).not.toContain("0 天");
});

test("PACK-011 a preview with no retention_days admits it instead of writing 保留 undefined 天", async () => {
  stubPlatform({ retentionDays: "absent" });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("沒有回答打包產物會保留多久");
  expect(text()).not.toContain("undefined");
  expect(text()).not.toContain("保留期限");
});

test("WS-004 an expired package stays in the list, says it expired, and offers no bytes", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      downloads: [
        {
          ...artifact,
          artifact_id: "expired-1",
          servable: false,
          serve_state: {
            value: "expired",
            label: "已過期,不再提供下載",
            note: "檔案已刪除,這筆紀錄保留。",
          },
          expires_at: "2099-01-01T00:00:00Z",
        },
        artifact,
      ],
    }),
  );
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(text()).toContain("已過期");
  expect(text()).toContain("檔案已刪除，這筆紀錄保留");
  expect(occurrences("沒有這一筆")).toBe(1);
  const links = Array.from(container.querySelectorAll("a")).filter((a) =>
    (a.getAttribute("href") ?? "").includes("/content"),
  );
  expect(links).toHaveLength(1);
});

test("04 丙-91 a lost package is not told the retention story", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      downloads: [
        {
          ...artifact,
          artifact_id: "lost-1",
          servable: false,
          serve_state: {
            value: "lost",
            label: "檔案遺失,不再提供下載",
            note: "這不是保存期到期——檔案在保存期內就不見了,是平台這一側的問題。同一版本重新打包一次可以拿回同樣的內容;如果再次發生,請回報。",
          },
          expires_at: "2099-01-01T00:00:00Z",
        },
      ],
    }),
  );
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(text()).toContain("是平台這一側的問題");
  expect(text()).toContain("請回報");
  const row = container.querySelector(".download-item")!;
  expect(row.textContent).not.toContain("到期後檔案刪除");
  expect(row.textContent).not.toContain("這筆紀錄保留");
  expect(row.textContent).not.toContain("到期時間");
  expect(
    Array.from(container.querySelectorAll("a")).filter((a) =>
      (a.getAttribute("href") ?? "").includes("/content"),
    ),
  ).toHaveLength(0);
});

test("丙-142 逐列複述提到清單層級：兩列，但那四句話各只印一次", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      downloads: [artifact, { ...artifact, artifact_id: "second-1", file_name: "csv-v1.zip" }],
    }),
  );
  await render(<Downloads />, () => text().includes("csv-v1.zip"));

  expect(container.querySelectorAll(".download-item")).toHaveLength(2);
  expect(occurrences("打包目標；安裝說明在套件內的 INSTALL.md")).toBe(1);
  expect(occurrences("到期後檔案刪除")).toBe(1);
  expect(occurrences("不是簽章")).toBe(1);
  expect(occurrences("與稽核事件是兩份不同的紀錄")).toBe(1);

  expect(occurrences("到期時間")).toBe(2);
  expect(occurrences("狀態：可下載")).toBe(2);
  expect(occurrences("sha256:bbbb")).toBe(2);
  expect(container.querySelector('[title*="INSTALL.md"]')).toBeNull();
});

test("§2.4 不能下載的那一列，在連結原本的位置說出是哪一種不能", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      downloads: [
        {
          ...artifact,
          servable: false,
          serve_state: {
            value: "quarantined",
            label: "檢查中(尚未可下載)",
            note: "打包完成,驗證還沒結束。這是暫時狀態(ADR-003 隔離)。",
          },
        },
      ],
    }),
  );
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(text()).not.toContain("目前不提供下載");
  const row = container.querySelector(".download-item")!;
  const paragraphs = Array.from(row.children).filter((el) => el.tagName === "P");
  const actions = paragraphs[paragraphs.length - 1];
  expect(actions.textContent).toContain("檢查中(尚未可下載)");
  expect(
    Array.from(container.querySelectorAll("a")).filter((a) =>
      (a.getAttribute("href") ?? "").includes("/content"),
    ),
  ).toHaveLength(0);
});

test("WS-002 an empty history says nothing was ever downloaded, not that records were cleared", async () => {
  vi.stubGlobal("fetch", () => json({ downloads: [] }));
  await render(<Downloads />, () => text().includes("還沒有打包過任何套件"));

  expect(text()).toContain("不是紀錄被清掉了");
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
  expect(text()).toContain("刪除的是這個套件的檔案本身");
  expect(text()).toContain("紀錄會保留");
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除")?.click());
  await waitFor(() => calls.some((c) => c.method === "DELETE"));
  expect(calls.find((c) => c.method === "DELETE")?.url).toContain(`/downloads/${ARTIFACT}`);
});

test("04 丙-14 the packaging page picks the version from a list, and ?version= is the default", async () => {
  const calls = stubPlatform();
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  const select = container.querySelector<HTMLSelectElement>("select")!;
  expect(select.value).toBe(VERSION);
  expect(text()).toContain("v2（最新）");
  expect(text()).toContain("v1");

  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(select, OLDER_VERSION);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() =>
    calls.some((u) => u.includes(`/versions/${OLDER_VERSION}/packaging/preview`)),
  );
  expect(text()).toContain(OLDER_VERSION);
  expect(text()).not.toContain("最新版本）");
});

test("WS-004 taking the file re-reads the list, so the page stops saying nobody has downloaded it", async () => {
  let served = false;
  const reads: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    reads.push(String(input));
    return json({ downloads: [{ ...artifact, download_count: served ? 1 : 0 }] });
  });
  // jsdom cannot navigate; stopping only the anchor's default action leaves
  // React's own click handler free to run.
  const stopNav = (e: Event) => e.preventDefault();
  document.addEventListener("click", stopNav, true);

  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));
  expect(text()).toContain("還沒有人下載過這個檔案");
  const readsBefore = reads.length;

  served = true;
  const link = Array.from(container.querySelectorAll("a")).find((a) =>
    (a.getAttribute("href") ?? "").includes("/content"),
  )!;
  await act(async () =>
    link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true })),
  );
  document.removeEventListener("click", stopNav, true);
  await waitFor(() => text().includes("誰下載過、什麼時候（1）"));

  expect(reads.length).toBeGreaterThan(readsBefore);
  expect(text()).not.toContain("還沒有人下載過這個檔案");
});

test("WS-004 the same link on the packaging page marks the download list stale too", async () => {
  stubPlatform();
  const stopNav = (e: Event) => e.preventDefault();
  document.addEventListener("click", stopNav, true);

  await render(<Packaging />, () => text().includes("這些設定可以打包"));
  await act(async () => button("建立下載套件")?.click());
  await waitFor(() => text().includes("套件已建立"));

  queryClient.setQueryData(["downloads"], { downloads: [] });
  expect(queryClient.getQueryState(["downloads"])?.isInvalidated).toBe(false);

  const link = Array.from(container.querySelectorAll("a")).find((a) =>
    (a.getAttribute("href") ?? "").includes("/content"),
  )!;
  await act(async () =>
    link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true })),
  );
  document.removeEventListener("click", stopNav, true);

  expect(queryClient.getQueryState(["downloads"])?.isInvalidated).toBe(true);
});

test("R4: picking a version changes the address, so the packaging preview can be linked", async () => {
  const calls = stubPlatform();
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  const select = container.querySelector<HTMLSelectElement>("select")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(select, OLDER_VERSION);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });

  expect(search.version).toBe(OLDER_VERSION);

  await waitFor(() =>
    calls.some((u) => u.includes(`/versions/${OLDER_VERSION}/packaging/preview`)),
  );
  expect(select.value).toBe(OLDER_VERSION);
});

test("丙-153 建立套件的按鈕前先說邀請限制，403 印中文並指向頁尾回報", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/versions")) return json(VERSIONS);
    if (url.includes("/packaging/targets")) return json(targets);
    if (url.includes("/packaging/preview")) {
      return json({
        target: "standard",
        allowed: true,
        validation: emptyValidation,
        dependencies: [],
        included_test_cases: [],
        excluded_test_cases: [],
        excluded_files: [],
        retention_days: 23,
      });
    }
    if (url.includes("/packaging") && init?.method === "POST") {
      return json({ error: "closed beta" }, 403);
    }
    if (url.includes(`/api/skills/${SKILL}`)) return json(skill);
    return json({ error: "not found" }, 404);
  });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("平台目前只讓有封測邀請的帳號建立下載套件。");

  await act(async () => button("建立下載套件")?.click());
  await waitFor(() => text().includes("這個帳號還沒有封測邀請"));

  expect(text()).toContain("用頁尾的「回報問題」選「我想要的東西，這裡沒有」");
  expect(text()).not.toContain("closed beta");
});

test("丙-150 建立套件失敗（非 403）說可以再按一次，不印 err.message", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/versions")) return json(VERSIONS);
    if (url.includes("/packaging/targets")) return json(targets);
    if (url.includes("/packaging/preview")) {
      return json({
        target: "standard",
        allowed: true,
        validation: emptyValidation,
        dependencies: [],
        included_test_cases: [],
        excluded_test_cases: [],
        excluded_files: [],
        retention_days: 23,
      });
    }
    if (url.includes("/packaging") && init?.method === "POST") {
      return json({ error: "internal error" }, 500);
    }
    if (url.includes(`/api/skills/${SKILL}`)) return json(skill);
    return json({ error: "not found" }, 404);
  });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  await act(async () => button("建立下載套件")?.click());
  await waitFor(() => text().includes("套件沒有建立成功，可以再按一次。"));

  expect(text()).not.toContain("internal error");
});

test("丙-150 打包預覽讀不到這個版本時，說回上一步重新挑一次版本", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/versions")) return json(VERSIONS);
    if (url.includes("/packaging/targets")) return json(targets);
    if (url.includes("/packaging/preview")) return json({ error: "skill version not found" }, 404);
    if (url.includes(`/api/skills/${SKILL}`)) return json(skill);
    return json({ error: "not found" }, 404);
  });
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));
  await waitFor(() => text().includes("這個版本讀不到"));

  expect(text()).toContain("回上一步重新挑一次版本");
  expect(text()).not.toContain("skill version not found");
});

test("丙-150 部署沒有設定打包目標時，503 說沒有預覽而不是英文句子", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.endsWith("/versions")) return json(VERSIONS);
    if (url.includes("/packaging/targets")) return json(targets);
    if (url.includes("/packaging/preview")) {
      return json({ error: "no packaging targets are configured on this deployment" }, 503);
    }
    if (url.includes(`/api/skills/${SKILL}`)) return json(skill);
    return json({ error: "not found" }, 404);
  });
  await render(<Packaging />, () => text().includes("標準 Agent Skill 套件"));
  await waitFor(() => text().includes("這個部署沒有設定任何打包目標，所以沒有預覽。"));

  expect(text()).not.toContain("no packaging targets are configured");
});

test("丙-155⑦ purged 印 serve_state.note，不印到期時間或到期後檔案刪除", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      downloads: [
        {
          ...artifact,
          artifact_id: "purged-1",
          servable: false,
          serve_state: {
            value: "purged",
            label: "檔案已不存在,紀錄保留",
            note: "儲存的位元組已經不在了,而這一列還在。同一版本可以再打包一次。",
          },
          expires_at: "2099-01-01T00:00:00Z",
        },
      ],
    }),
  );
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(text()).toContain("儲存的位元組已經不在了");
  const row = container.querySelector(".download-item")!;
  expect(row.textContent).not.toContain("到期時間");
  expect(row.textContent).not.toContain("到期後檔案刪除");
  expect(row.textContent).not.toContain("這筆紀錄保留。「已過期");
});
