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

/*
 * The address, standing in for the router. `?version=` is not a constant here
 * because 資訊架構 §0.1 R4 makes it the thing the picker writes to: with a fixed
 * `useSearch` and a swallowed `useNavigate`, a picker holding its choice in
 * component state — which used to WIN over the URL — would pass every
 * assertion. Components re-read through `useSyncExternalStore`.
 */
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
    // Contract-required and missing until the packaging page started rendering
    // the risk summary §2.10 第 1 項 puts on the never-fold list. Second fixture
    // this week to be short a required field（`excluded_files` was the first），
    // so the object below is pinned to the type with `satisfies` — an untyped
    // fixture is a replica that can disagree with the contract in silence.
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

/**
 * `blocked` drives the preview; `duplicate` drives what POST .../packaging
 * answers; `retentionDays` is 03:PACK-011's served period — a number, or
 * `"absent"` to send a preview without the field at all.
 */
function stubPlatform(
  options: {
    blocked?: boolean;
    duplicate?: boolean;
    retentionDays?: number | "absent";
    /** 打包器拿掉的檔案。預設空，因為幾乎每個套件都是空的——重點是非空那一個。 */
    excludedFiles?: {
      path: string;
      reason: string;
      label: string;
      note: string;
    }[];
  } = {},
) {
  // 23 and never 30. 30 is what a deployment actually configures, so a mock
  // saying 30 would be satisfied by a component that hardcoded 30 — which is the
  // exact regression the server half was built to prevent (it compares the days
  // the preview reported against the `expires_at` the create call wrote, rather
  // than against a constant). Nobody hardcodes 23.
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
              blocked_message: "nobody has established whether this skill may be redistributed",
              validation: emptyValidation,
              // A gate closed before any bytes were read, so there is nothing to
              // have dependencies — not a package that has none.
              dependencies: [],
              included_test_cases: [],
              excluded_test_cases: [
                { test_case_id: "tc1", name: "我上傳的資料", reason: "user-uploaded dataset" },
              ],
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
              // The lines the produced INSTALL.md carries, verbatim.
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
  // ADR-045: the owner getting their own upload back. Releases, and is a
  // separate value from `allowed` because it is not a licence verdict.
  expect(
    packagingGate(detail({ redistribution: { value: "self_supplied", label: "", note: "" } })),
  ).toBeNull();
  // 0037 / ADR-047 決策 4: the platform's own output. It released on the server
  // (gateFlags) from the day the value existed, and refused here — the switch
  // this table replaced sent it to `default`. A generated skill the owner could
  // not download was the visible half of that.
  expect(
    packagingGate(detail({ redistribution: { value: "generated", label: "", note: "" } })),
  ).toBeNull();
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
  // 執行環境相容, not 實測相容: that axis is a rule about whether the image
  // provides the declared runtime, and nothing observes a script running.
  expect(text()).toContain("執行環境相容：未驗證");
  expect(text()).not.toContain("實測相容");
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

/**
 * 設計 §3 第 4 條逐字寫的失效——「型別裡有、伺服器送了、頁面丟掉」。
 *
 * `excluded_files` 是 contract 的必填欄位，而在此之前全 `apps/web/src` 只有型別宣告
 * 與兩筆空陣列 fixture 碰過它：零渲染、零測試。`api/packaging.ts` 的註解甚至已經寫好
 * 它為什麼必須出現在**預覽**上——「the manifest is inside the thing the reader has
 * not decided to download yet」，答案只寫在還沒下載的那份 manifest 裡，就不是在回答
 * 這個決定。
 *
 * 具體的代價：一個 vendored 依賴的 Skill 打包後少了 `node_modules/`，作者把 zip 交給
 * 同事，同事裝不起來，而兩個人都以為那是完整的套件。唯一會浮出來的情況是 SKILL.md
 * 剛好指到被拿掉的檔（那走 `file_removed_by_packager`）；其餘每一種排除都靜音。
 *
 * 兩個答案都押：非空要逐列印出伺服器的字，空要說出「什麼都沒被拿掉」而不是消失。
 */
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

/**
 * 設計 §2.10 是一份**封閉清單**，第 3 項是 License 與可散布性判定、第 1 項是風險
 * 摘要。在此之前這一頁只在**拒絕**的時候談授權：`redistribution` 放行時 `gate` 是
 * null，整頁不再提它一個字——而 allowed／self_supplied／generated 三者放行的理由
 * 各不相同，畫面上長得完全一樣。這是全 app 唯一一個**內容會離開平台**的位址。
 */
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

// --- 03:PACK-011: how long the package will be kept -------------------------

test("PACK-011 保留期限 is the server's number and it arrives before the build button", async () => {
  // The mock says 23 days for the reason spelled out on `stubPlatform`: a test
  // that asserts 「30 天」 against a mock that also says 30 passes just as happily
  // against a component with 30 typed into it, and a second definition of a
  // deployment number that nothing compares against the one writing `expires_at`
  // is 設計 §2.2 顯示與強制成對 broken in the direction nobody notices.
  stubPlatform({ retentionDays: 23 });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("保留期限");
  expect(text()).toContain("23 天");

  // 打包之前, not after (03:PACK-011 / 02:NFR-001 的「會影響你的上限要在撞到之前
  // 看得見」): the question is 「我下週回來還在不在」, and an answer that arrives
  // after the button is an answer to a decision already spent.
  const notice = Array.from(container.querySelectorAll("p")).find((p) =>
    (p.textContent ?? "").includes("保留期限"),
  )!;
  expect(notice).toBeTruthy();
  expect(
    notice.compareDocumentPosition(button("建立下載套件")!) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();

  // 過期 ≠ 做白工. Deleting the link is not deleting the user's work, and only
  // one of those two sentences is true.
  expect(text()).toContain("打包是冪等的");
});

test("PACK-011 a retention under one day says 不到 1 天 rather than 0 天", async () => {
  // The server truncates instead of rounding (`retentionDays` in
  // delivery/http.go) so that the error falls on the side of promising less, and
  // anything under a day therefore arrives as 0. 「0 天」 reads as 「馬上就刪」 and
  // an empty string reads as 「沒有期限」; both are wrong about the same number,
  // and a deployment configured that short is already violating 02:NFR-002a — it
  // should read as wrong, not be smoothed into a plausible-looking 1.
  stubPlatform({ retentionDays: 0 });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("不到 1 天");
  expect(text()).not.toContain("0 天");
});

test("PACK-011 a preview with no retention_days admits it instead of writing 保留 undefined 天", async () => {
  // Unreachable by contract — the server answers 503 for the whole preview when
  // the deployment has no ratified DOWNLOAD_ARTIFACT_RETENTION, so no number and
  // no build. Which is exactly why nothing else would catch this branch
  // regressing into a rendered `undefined`.
  stubPlatform({ retentionDays: "absent" });
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  expect(text()).toContain("沒有回答打包產物會保留多久");
  expect(text()).not.toContain("undefined");
  expect(text()).not.toContain("保留期限");
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
          // Deliberately in the future. Both the old client-side derivation and
          // the server's word agreed while this date was in the past, so the
          // fixture could not tell them apart — and the client-side one was the
          // wrong predicate (M4 audit, 2026-08-24).
          expires_at: "2099-01-01T00:00:00Z",
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
          // In the future, and that is the point: the bytes are already gone
          // while the promise about them has not come due. This row rendered
          // 「到期後檔案刪除」 — a future tense about a past event — because the
          // component had one branch for expiry and none for loss.
          expires_at: "2099-01-01T00:00:00Z",
        },
      ],
    }),
  );
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  expect(text()).toContain("是平台這一側的問題");
  expect(text()).toContain("請回報");
  // The two sentences that would describe this as normal, neither of which is
  // true here.
  expect(text()).not.toContain("到期後檔案刪除");
  expect(text()).not.toContain("這筆紀錄保留");
  // No link to bytes that are not there.
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

test("WS-004 taking the file re-reads the list, so the page stops saying nobody has downloaded it", async () => {
  // The record is written by the server when it serves `/downloads/{id}/content`,
  // and `download_count` is computed from that table. Nothing here unmounts on a
  // click and refetchOnWindowFocus is off, so before the invalidate the count
  // stayed 0 — and DownloadHistory's `enabled` guard reads that same 0, so the
  // disclosure did not even ask.
  let served = false;
  const reads: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    reads.push(String(input));
    return json({ downloads: [{ ...artifact, download_count: served ? 1 : 0 }] });
  });
  // jsdom cannot navigate; only the anchor's default action is stopped, so
  // React's own click handler still runs.
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
  // The twin of the link above. Nothing on this page observes ["downloads"], so
  // what is asserted is the stale mark rather than a refetch — that is exactly
  // what makes 到下載紀錄 arrive with the count the server has.
  stubPlatform();
  const stopNav = (e: Event) => e.preventDefault();
  document.addEventListener("click", stopNav, true);

  await render(<Packaging />, () => text().includes("這些設定可以打包"));
  await act(async () => button("建立下載套件")?.click());
  await waitFor(() => text().includes("套件已建立"));

  // The build itself invalidates the list; re-seed so the click is the only
  // thing this can be measuring.
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

/*
 * 資訊架構 §0.1 R4 — 「你在看哪一份東西」進網址.
 *
 * The picker used to write to `useState` and that state WON over `?version=`
 * (`picked || version || …`). So: open …/package?version=A, pick B, copy the
 * address, send it — the recipient got A's preview, and so did the sender after
 * a reload. A lossy URL is bad; a URL that actively disagrees with the screen
 * is worse, and this is the screen whose whole subject is which immutable
 * version a set of bytes came from (ADR-003).
 */
test("R4: picking a version changes the address, so the packaging preview can be linked", async () => {
  const calls = stubPlatform();
  await render(<Packaging />, () => text().includes("這些設定可以打包"));

  const select = container.querySelector<HTMLSelectElement>("select")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(select, OLDER_VERSION);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });

  // The pick landed in the address. Held in component state it did not, and
  // nothing else on the page could tell the difference.
  expect(search.version).toBe(OLDER_VERSION);

  // And the address is what the page reads back: the preview it fetched belongs
  // to the version the URL now names.
  await waitFor(() =>
    calls.some((u) => u.includes(`/versions/${OLDER_VERSION}/packaging/preview`)),
  );
  expect(select.value).toBe(OLDER_VERSION);
});
