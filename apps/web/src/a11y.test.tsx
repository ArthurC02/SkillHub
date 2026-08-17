import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import axe from "axe-core";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";

/**
 * 03:QA-009 / DESIGN-013 — the accessibility check, run as a test so it keeps
 * being true. The bar is 02:NFR-007: 主要操作可使用鍵盤完成、表單具有標籤與清楚的
 * 驗證訊息、風險與相容與評估狀態必須同時提供文字。
 *
 * Every route in router.tsx is rendered against mocked API data and scanned with
 * axe-core; any violation fails the build. The last two sections of this file are
 * the keyboard walkthrough and the validation messages (04 丙-21 ①②), which axe
 * answers nothing about.
 *
 * Three things this cannot see, recorded here rather than switched off:
 *
 * 1. **Colour contrast.** axe answers `incomplete` for `color-contrast` on every
 *    node here, and importing index.css with `css: true` does not change that —
 *    jsdom computes no layout, so axe cannot resolve what is behind a pixel. The
 *    rule stays enabled (it is simply never decided); the palette was measured by
 *    hand instead, which is what removed the `opacity` mutes and the literal
 *    `#d33` from index.css. **A contrast regression will not fail this test.**
 * 2. **Page-level rules** (`html-has-lang`, `document-title`, …) do not run
 *    against an element context. `index.html` carries them and no page can
 *    change them at runtime.
 * 3. **The browser's own key handling.** jsdom does not turn Enter on a focused
 *    button into a click, does not implement Tab, and does not open a `<details>`
 *    on Enter over its `<summary>`. So the walkthrough below asserts the half a
 *    test can own — that every step of a journey is a native control, in the
 *    tab sequence, focusable, and that activating it moves the journey on — and
 *    leaves the half the platform owns to the platform. **What it cannot prove is
 *    that a real browser's Tab order matches DOM order**; nothing here fakes that.
 */

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  window.history.pushState({}, "", "/");
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

const SKILL = "11111111-1111-1111-1111-111111111111";
const SKILL_B = "aaaaaaaa-2222-2222-2222-222222222222";
const VERSION = "22222222-2222-2222-2222-222222222222";
const TEST_CASE = "33333333-3333-3333-3333-333333333333";
const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
const OTHER_RUN = "5c2e8a10-4b6d-4c31-9f77-2ab3d4e5f608";
const ARTIFACT = "44444444-4444-4444-4444-444444444444";

// --- fixtures ---------------------------------------------------------------
//
// Deliberately the busy states rather than the empty ones: an empty page has no
// badges, no disclosures, no tables and no form controls, so scanning it would
// prove nothing about the markup a real reader meets.

const HIT_FACETS = {
  tier: { value: "indexed", label: "已收錄", note: "收錄不等於精選。" },
  risk: {
    scan_status: "scanned",
    level: "disclosed",
    warnings: 1,
    has_scripts: true,
    note: "以上為靜態掃描結果。",
  },
  dependencies: ["pypdf"],
  compatibility: {
    spec_validation: "passed",
    capability: "unverified",
    runtime: "unverified",
    note: "尚未試跑。",
  },
  verified_at: "2026-08-01T10:00:00Z",
};

const SEARCH = {
  query: "pdf 摘要",
  results: [
    {
      ...HIT_FACETS,
      skill_id: SKILL,
      name: "PDF Summariser",
      summary: "把 PDF 整理成摘要",
      rank: 0.82,
      match_reason: "這個 Skill 直接處理 PDF 並輸出摘要。",
      match_reason_source: "model",
    },
    {
      ...HIT_FACETS,
      skill_id: SKILL_B,
      name: "Doc Splitter",
      summary: "切分文件",
      rank: null,
      rank_note: "尚未建立語意索引，未評分。",
      match_reason: "查詢與文件共同出現：pdf",
      match_reason_source: "template",
    },
  ],
  degraded: true,
  degraded_reason: "embedding unavailable; lexical search only",
  partial_index: true,
  no_results: false,
  filtered_out: false,
};

function skillDetail(id: string, name: string) {
  return {
    skill_id: id,
    name,
    summary: "把 PDF 整理成摘要",
    scope: "catalog",
    tier: { value: "indexed", label: "已收錄", note: "收錄不等於精選。" },
    enrichment: {
      status: "enriched",
      summary: "讀 PDF，輸出重點摘要。",
      task_examples: ["把季報整理成三段摘要。"],
      tags: { inputs: ["pdf"], outputs: ["markdown"], tools: [], dependencies: ["pypdf"] },
      model: "gpt-5-mini",
      prompt_version: "enrich-2026-08",
      note: "本區塊由模型產生。",
    },
    limitations: [
      { text: "不處理掃描件的手寫字。", source: "model" },
      { text: "套件內含可執行 Script。", source: "scan" },
    ],
    allowed_tools: ["Bash"],
    source: {
      type: "git",
      url: "https://github.com/example/pdf",
      source_version: "abc123",
      fetched_at: "2026-08-01T10:00:00Z",
      last_checked_at: "2026-08-15T10:00:00Z",
      content_hash: "sha256:beef",
      trust: { value: "traceable", label: "來源可追溯", note: "已保存來源紀錄。" },
    },
    license: {
      expression: "MIT",
      source: "repo-license-file",
      source_note: "來自 repo 根目錄的 LICENSE。",
      status: { value: "declared", label: "License 已宣告", note: "尚未經人工核對。" },
    },
    redistribution: { value: "allowed", label: "可再散布", note: "MIT，可再散布。" },
    derivation: { is_fork: false, label: "來源關係", note: "非 Fork。" },
    version: {
      version_id: VERSION,
      version_number: 2,
      content_hash: "sha256:aa",
      created_at: "2026-08-01T00:00:00Z",
    },
    risk: {
      scan_status: "scanned",
      counts: { errors: 0, warnings: 1, infos: 321 },
      highlights: [
        {
          severity: "warning",
          code: "embedded-script",
          path: "SKILL.md",
          message: "SKILL.md 內含可執行程式碼區塊。",
        },
      ],
      info_counts: { "external-url": 320, "large-file": 1 },
      has_embedded_script: true,
      note: "以上為靜態掃描結果。",
    },
    compatibility: {
      spec_validation: "passed",
      capability: "activated",
      runtime: "transpiled",
      runtime_image: "ghcr.io/skillhub/runtime:2026.08-3",
      measured_at: "2026-08-10T00:00:00Z",
      note: "以上為單次沙箱實測。",
    },
  };
}

const FILES = {
  skill_id: SKILL,
  version_id: VERSION,
  version_number: 2,
  skill_md: "---\nname: pdf\n---\n\n用法說明。",
  skill_md_truncated: true,
  tree: [
    { path: "SKILL.md", size: 42, is_script: false },
    { path: "scripts/run.py", size: 17, is_script: true },
  ],
  embedded_script_note: "SKILL.md 內含可執行程式碼。",
  note: "tree 為套件內檔案清單與大小。",
};

const TARGETS = {
  targets: [
    {
      id: "standard",
      kind: "standard_package",
      version: "1.0.0",
      display_name: "標準 Agent Skill 套件",
      support_status: "unverified",
      verification_steps: ["Unzip the package. SKILL.md must be at the root of the archive."],
      notes: ["Any spec-compliant agent may try it; Skill Hub has not tried it on yours."],
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
          example: "<your own key>",
        },
      ],
    },
  ],
};

const PREVIEW = {
  target: "standard",
  allowed: true,
  validation: {
    blocked: false,
    errors: [],
    warnings: [
      { code: "frontmatter.long_description", message: "description is long", path: "SKILL.md" },
    ],
    infos: [{ code: "external-url", message: "SKILL.md 內有外部網址", details: ["example.com"] }],
  },
  dependencies: ["requirements.txt: package declares external dependencies", "pypdf"],
  included_test_cases: [{ test_case_id: TEST_CASE, name: "去重複列", slug: "dedupe" }],
  excluded_test_cases: [
    { test_case_id: "tc-2", name: "我上傳的資料", reason: "user-uploaded dataset" },
  ],
};

const ARTIFACT_ROW = {
  artifact_id: ARTIFACT,
  skill_id: SKILL,
  skill_version_id: VERSION,
  target: "standard",
  file_name: "pdf-summariser-v2.zip",
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

const DOWNLOADS = {
  downloads: [
    ARTIFACT_ROW,
    {
      ...ARTIFACT_ROW,
      artifact_id: "expired-1",
      status: "rejected",
      status_reason: "打包後未通過驗證。",
      expires_at: "2026-01-01T00:00:00Z",
    },
  ],
};

const RUNS = {
  runs: [
    {
      run_id: RUN,
      status: "succeeded",
      skill_id: SKILL,
      skill_name: "PDF Summariser",
      skill_version_id: VERSION,
      test_case_id: TEST_CASE,
      provider: "self-hosted",
      cleanup_status: "failed",
      created_at: "2026-08-17T00:00:00Z",
      finished_at: "2026-08-17T00:04:00Z",
    },
  ],
};

const RUN_ARTIFACTS = {
  artifacts: [
    {
      artifact_id: "aaaa1111-2222-3333-4444-555566667777",
      file_name: "summary.md",
      content_type: "text/markdown",
      size_bytes: 2048,
      content_hash: "sha256:dddd",
      created_at: "2026-08-17T00:03:00Z",
      purged: false,
    },
  ],
};

const TEST_CASE_DRAFT = {
  test_case_id: TEST_CASE,
  skill_id: SKILL,
  name: "去重複列",
  user_prompt: "把重複的列刪掉，保留第一次出現的那一列。",
  acceptance_criteria: [
    { id: "c1", text: "輸出的列數少於輸入", source: "user", confirmed_at: "2026-08-17T01:00:00Z" },
    { id: "c2", text: "沒有刪掉原始欄位", source: "suggested", confirmed_at: null },
  ],
  rubric: {
    version: "content-007/writing/v1",
    items: [{ id: "c1", text: "引出顯示列數變少的那一句。", weight: 2, evidence_required: true }],
  },
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T01:00:00Z",
};

const PREFLIGHT = {
  summary_hash: "hash-one",
  estimated_cost: {
    currency: "USD",
    low: 0.01,
    typical: 0.06,
    high: 0.3,
    basis: "估計值，非報價。",
  },
  quota: {
    remaining_today: 3,
    remaining_window: 21,
    window_resets_at: "2026-09-01T00:00:00Z",
    limits: { daily: 5, window: 30, window_days: 30, concurrent: 2 },
  },
  notes: ["以上任何一項變更都會產生新的摘要，必須重新確認。"],
  summary: {
    skill_version_id: VERSION,
    skill_content_hash: "sha256:abc",
    test_case_id: TEST_CASE,
    datasets: [
      {
        dataset_id: "d0",
        file_name: "rows.csv",
        content_type: "text/csv",
        size_bytes: 1024,
        content_hash: "h0",
      },
    ],
    dataset_total_bytes: 1024,
    scripts: { status: "present", findings: ["scripts/run.py"] },
    tools: ["sandbox filesystem (/work, /out)"],
    mcp_servers: [],
    network: { mode: "default_deny", allow: [] },
    injected_secrets: ["ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"],
    provider: { name: "self-hosted", isolation_level: "gvisor", rootless: true },
    resource_limits: {
      vcpu: 2,
      memory_bytes: 4 << 30,
      disk_bytes: 8 << 30,
      max_pids: 256,
      max_open_files: 1024,
      wall_clock_soft_seconds: 600,
      wall_clock_hard_seconds: 900,
      artifact_total_bytes: 100 << 20,
      artifact_file_bytes: 25 << 20,
      token_budget: { max_input_tokens: 300000, max_output_tokens: 60000 },
    },
  },
};

const LIMITS = {
  max_file_bytes: 25 << 20,
  max_test_case_bytes: 100 << 20,
  max_files_per_test_case: 20,
  retention_days: 90,
  allowed_kinds: ["text (.txt .md .csv)", "documents (.pdf .docx)"],
  note: "file type is decided by content, not by file extension",
};

const TRACE_GENERAL = {
  run_id: RUN,
  status: "failed",
  status_reason: "the provider could not carry the attempt",
  complete: false,
  skills: [{ name: "pdf-summarise", decision: "activated", reason: "prompt 提到 PDF" }],
  resources_read: 1,
  tool_calls: {
    total: 2,
    succeeded: 1,
    failed: 1,
    total_duration_ms: 3532,
    slowest_duration_ms: 3412,
    slowest_tool: "bash",
  },
  errors: [{ category: "provision", code: "provider_error", message: "no slot" }],
  final_output: "Removed 17 duplicate rows.",
  usage: { model: "gpt-5-mini", input_tokens: 27042, output_tokens: 1180, cost_usd: null },
  steps: ["queued: run requested", "failed: the provider could not carry the attempt"],
};

const TRACE_ADVANCED = {
  run_id: RUN,
  complete: false,
  streams: [
    {
      attempt: 1,
      emitted_by: "sandbox",
      received: 2,
      highest_seq: 3,
      missing_seq: [2],
      late_events: 1,
    },
  ],
  events: [
    {
      event_id: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
      attempt: 1,
      seq: 1,
      occurred_at: "2026-08-16T09:12:04.002Z",
      emitted_by: "sandbox",
      type: "script_log",
      status: "error",
      late: true,
      masked_fields: ["/message"],
      payload: { stream: "stderr", message: "boom", truncated: false },
    },
  ],
};

const EVALUATION = {
  evaluation_id: "eval-2",
  run_id: RUN,
  status: "completed",
  overall: "not_met",
  summary: "產出的檔案缺少要求的欄位。",
  criterion_results: [
    {
      criterion_id: "c1",
      text: "輸出的 CSV 含有 email 欄位",
      result: "failed",
      source: "model",
      reason: "最終輸出的表頭沒有 email。",
      evidence: [
        {
          kind: "trace_event",
          trace_event_id: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
          occurred_at: "2026-08-16T09:12:04.002Z",
          available: false,
          excerpt: "header: name,phone",
          excerpt_truncated: true,
        },
      ],
    },
    {
      criterion_id: "c3",
      text: "報告有貼出正文",
      result: "undetermined",
      source: "model",
      reason: "evidence_unverifiable: no trace event was sent to the judge.",
      evidence: [],
    },
  ],
  deterministic_findings: [
    {
      category: "activation",
      severity: "warning",
      message: "沒有出現 Skill 啟用事件。",
      evidence: [],
    },
  ],
  judge_model: "gpt-5.6-terra",
  judge_prompt_version: "judge-2026-08-17",
  rubric_version: "content-007/writing/v1",
  evidence_complete: false,
  cost: { evaluation_usd: 0.0212, source: "gateway", note: "閘道對這次評估的實付。" },
  feedback: { helpful: false, comment: "沒說到重點。", submitted_at: "2026-08-17T03:00:00Z" },
  evaluated_at: "2026-08-17T02:00:00Z",
  superseded_at: null,
};

// Two revisions, which is what puts the 評估版本 <select> on screen.
const REVISIONS = {
  revisions: [
    {
      evaluation_id: "eval-2",
      judge_prompt_version: "judge-2026-08-17",
      overall: "not_met",
      evaluated_at: "2026-08-17T02:00:00Z",
      superseded_at: null,
    },
    {
      evaluation_id: "eval-1",
      judge_prompt_version: "judge-2026-08-01",
      overall: "undetermined",
      evaluated_at: "2026-08-16T02:00:00Z",
      superseded_at: "2026-08-17T02:00:00Z",
    },
  ],
};

const SUGGESTIONS = {
  evaluation_id: "eval-2",
  suggestions: [
    {
      suggestion_id: "s1",
      category: "skill",
      problem: "SKILL.md 沒有交代輸出欄位。",
      evidence: [],
      target_path: "SKILL.md",
      expected_impact: "模型會照著列出的欄位輸出。",
      decision: "accepted",
    },
  ],
};

function comparisonSide(runId: string, evaluated: boolean) {
  return {
    run_id: runId,
    skill_id: SKILL,
    skill_version_id: VERSION,
    test_case_id: TEST_CASE,
    status: evaluated ? "succeeded" : "failed",
    evaluation: evaluated
      ? {
          evaluation_id: "eval-2",
          status: "completed",
          overall: "not_met",
          cost: { evaluation_usd: 0.02, source: "gateway", note: "" },
        }
      : undefined,
    final_output: "Removed 17 duplicate rows.",
    errors: [{ category: "provision", code: "provider_error", message: "no slot" }],
    duration_ms: 4200,
    cost: { usd: 0.13, is_lower_bound: true, authoritative_source: "模型閘道 per-key 實付" },
    inputs_available: evaluated,
  };
}

const COMPARISON = {
  runs: [comparisonSide(RUN, true), comparisonSide(OTHER_RUN, false)],
  criterion_matrix: [
    {
      criterion_id: "c1",
      text: "輸出的 CSV 含有 email 欄位",
      results: [
        { run_id: RUN, result: "failed", source: "model" },
        { run_id: OTHER_RUN, result: null },
      ],
    },
  ],
  version_diff_url: `/skills/${SKILL}/versions/diff?from=v1&to=v2`,
};

const VERSION_DIFF = {
  files: [
    { path: "SKILL.md", status: "modified", diff: "@@ -1 +1 @@\n-old\n+new" },
    { path: "assets/logo.png", status: "modified" },
  ],
};

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

/** One fake platform for every route; ordered most specific first. */
function stubPlatform() {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];

    if (path.startsWith("/api/skills/search")) return json(SEARCH);
    if (path.endsWith("/files")) return json(FILES);
    if (path.startsWith("/api/skills/"))
      return json(skillDetail(path.slice("/api/skills/".length), "PDF Summariser"));

    if (path === "/me") return json({ user_id: "u-1", login: "tester" });
    if (path === "/packaging/targets") return json(TARGETS);
    if (path.endsWith("/packaging/preview")) return json(PREVIEW);
    if (path === "/downloads") return json(DOWNLOADS);
    if (path === `/downloads/${ARTIFACT}/records`)
      return json({ records: [{ downloaded_at: "2026-08-17T09:00:00Z", actor: "tester" }] });
    if (path === "/runs") return json(RUNS);
    if (path.endsWith("/artifacts")) return json(RUN_ARTIFACTS);

    if (path === "/test-cases/limits") return json(LIMITS);
    if (path.endsWith("/datasets"))
      return json({
        datasets: [
          { dataset_id: "d0", file_name: "rows.csv", content_type: "text/csv", size_bytes: 1024 },
        ],
        total_bytes: 1024,
      });
    if (path === "/test-cases") return json({ test_cases: [TEST_CASE_DRAFT] });
    if (path.startsWith("/test-cases/")) return json(TEST_CASE_DRAFT);
    if (path === "/skills")
      return json({ skills: [{ skill_id: SKILL, name: "PDF Summariser", summary: "摘要" }] });
    if (path.includes("/versions/diff")) return json(VERSION_DIFF);
    if (path.endsWith("/runs/preflight")) return json(PREFLIGHT);

    if (path.endsWith("/trace"))
      return json(url.includes("advanced") ? TRACE_ADVANCED : TRACE_GENERAL);
    if (path.endsWith("/evaluation/revisions")) return json(REVISIONS);
    if (path.endsWith("/evaluation")) return json(EVALUATION);
    if (path.endsWith("/suggestions")) return json(SUGGESTIONS);
    if (path.endsWith("/comparison")) return json(COMPARISON);
    if (path.startsWith("/runs/"))
      return json({
        run_id: RUN,
        skill_id: SKILL,
        skill_version_id: VERSION,
        test_case_snapshot_id: "snap-1",
        test_case_id: TEST_CASE,
      });

    return json({ error: "not found" }, 404);
  });
}

async function mount() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
}

/** Polls until the query has settled and React has flushed the result. */
async function waitFor(done: () => boolean, timeoutMs = 4000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

const has = (needle: string) => () => (container.textContent ?? "").includes(needle);

/**
 * Every rule axe knows, minus nothing: the WCAG 2.0/2.1/2.2 A and AA tags plus
 * the best-practice set. Lowering this list to make a page pass would be exactly
 * the move QA-009 exists to prevent.
 */
const TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"];

async function scan(where: string) {
  const results = await axe.run(container, {
    runOnly: { type: "tag", values: TAGS },
    resultTypes: ["violations"],
  });
  const report = results.violations.map(
    (v) =>
      `${v.id} (${v.impact}): ${v.help}\n    ${v.nodes
        .map((n) => n.html.slice(0, 160))
        .join("\n    ")}`,
  );
  expect(report, `${where} has accessibility violations:\n  ${report.join("\n  ")}`).toEqual([]);

  // Keyboard reach, asserted structurally because axe cannot press Tab: a
  // <details> without a <summary> is a disclosure no keyboard can open, and a
  // positive tabindex reorders focus away from reading order.
  const details = container.querySelectorAll("details");
  for (const node of details) {
    expect(
      node.querySelector(":scope > summary"),
      `${where}: <details> without <summary>`,
    ).not.toBeNull();
  }
  expect(
    container.querySelectorAll('[tabindex]:not([tabindex="0"]):not([tabindex="-1"])'),
    `${where}: positive tabindex breaks focus order`,
  ).toHaveLength(0);
}

/**
 * The tab sequence as the platform would build it: native interactive elements
 * in DOM order, minus the ones a browser skips — disabled controls, anything
 * hidden from the accessibility tree, and the contents of a closed `<details>`
 * (its `<summary>` stays, which is how a keyboard opens it).
 *
 * Positive `tabindex` is asserted absent in scan(), so DOM order IS the sequence
 * here. That equivalence is the assumption this helper rests on, and it is the
 * one thing only a real browser can confirm (QA-008).
 */
const FOCUSABLE =
  'a[href], button, input, select, textarea, summary, [tabindex]:not([tabindex="-1"])';

function tabbables(): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => {
    if (el.hasAttribute("disabled") || el.getAttribute("aria-hidden") === "true") return false;
    const closed = el.closest("details:not([open])");
    return closed === null || el === closed.querySelector(":scope > summary");
  });
}

/**
 * Reaches a control the way a keyboard user does — find it in the tab sequence,
 * put focus on it, activate it — and fails loudly when it is not in that
 * sequence at all. `click()` stands in for Enter/Space on a focused native
 * control, which is the browser behaviour jsdom does not implement.
 */
async function keyboardActivate(label: string, match: (el: HTMLElement) => boolean) {
  const target = tabbables().find(match);
  expect(target, `${label} is not reachable by keyboard`).toBeDefined();
  target!.focus();
  expect(document.activeElement, `${label} did not take focus`).toBe(target);
  await act(async () => target!.click());
}

const byText = (text: string) => (el: HTMLElement) => (el.textContent ?? "").includes(text);

// --- one case per route in router.tsx ---------------------------------------

test("QA-009: 首頁與搜尋結果", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf 摘要" } });
  });
  await waitFor(has("PDF Summariser"));
  await scan("/");
}, 30000);

test("QA-009: Skill 詳情", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId", params: { skillId: SKILL } });
  });
  await waitFor(has("可散布性與打包"));
  await scan("/skills/$skillId");
}, 30000);

test("QA-009: Skill 檔案（進階模式）", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/skills/$skillId/files", params: { skillId: SKILL } });
  });
  await waitFor(has("scripts/run.py"));
  await scan("/skills/$skillId/files");
}, 30000);

test("QA-009: 打包與下載", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/skills/$skillId/package",
      params: { skillId: SKILL },
      search: { version: VERSION },
    });
  });
  await waitFor(has("打包預覽"));
  await scan("/skills/$skillId/package");
}, 30000);

test("QA-009: 下載紀錄", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/downloads" });
  });
  await waitFor(has("pdf-summariser-v2.zip"));
  await scan("/workspace/downloads");

  // 02:WS-002 第 3 條 is a two-step delete, and the second step only exists
  // after the first: scan the confirming state too, and check that focus went
  // with it rather than being dropped on <body>.
  const ask = [...container.querySelectorAll("button")].find((b) => b.textContent === "刪除")!;
  await act(async () => ask.click());
  const confirm = [...container.querySelectorAll("button")].find(
    (b) => b.textContent === "確認刪除",
  );
  expect(document.activeElement).toBe(confirm);
  await scan("/workspace/downloads（確認刪除）");
}, 30000);

test("QA-009: 並排比較", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/compare", search: { ids: `${SKILL},${SKILL_B}` } });
  });
  await waitFor(() => container.querySelector("table.compare-table") !== null);
  await scan("/compare");
}, 30000);

test("QA-009: 執行前權限確認", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/lab/run",
      search: { skill: SKILL, version: VERSION, test_case: TEST_CASE },
    });
  });
  await waitFor(has("資源上限"));
  await scan("/lab/run");
}, 30000);

test("QA-009: Dataset 上傳", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/datasets", search: { test_case: TEST_CASE } });
  });
  await waitFor(has("上傳前請先確認"));
  await scan("/lab/datasets");
}, 30000);

test("QA-009: Run 詳情（一般與進階模式）", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/runs/$runId", params: { runId: RUN } });
  });
  await waitFor(has("任務判定"));
  await waitFor(has("最終輸出"));
  await scan("/runs/$runId（一般模式）");

  const advanced = [...container.querySelectorAll("button")].find(
    (b) => b.textContent === "進階模式",
  )!;
  await act(async () => advanced.click());
  await waitFor(() => container.querySelector("table") !== null);
  await scan("/runs/$runId（進階模式）");
}, 30000);

test("QA-009: Run 比較", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/runs/$runId/compare",
      params: { runId: RUN },
      search: { against: OTHER_RUN },
    });
  });
  await waitFor(has("逐條驗收條件"));
  await scan("/runs/$runId/compare");
}, 30000);

test("QA-009: Test Case 列表", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/test-cases" });
  });
  await waitFor(has("建立新的 Test Case"));
  await scan("/lab/test-cases");
}, 30000);

test("QA-009: Test Case 詳情", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({
      to: "/lab/test-cases/$testCaseId",
      params: { testCaseId: TEST_CASE },
    });
  });
  await waitFor(has("Rubric（選用）"));
  await scan("/lab/test-cases/$testCaseId");
}, 30000);

test("QA-009: Run 歷史", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/runs" });
  });
  await waitFor(has("PDF Summariser"));
  await scan("/workspace/runs");
}, 30000);

test("QA-009: 我的 Skill", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/skills" });
  });
  await waitFor(has("我的 Skill"));
  await scan("/workspace/skills");
}, 30000);

// --- 02:NFR-007「主要操作可使用鍵盤完成」: the walkthrough (04 丙-21 ①) ---------
//
// Journeys rather than pages: the four handbacks this project has recorded all
// happened between two steps, and a per-page check is exactly what cannot see a
// seam. Each step asserts the same three things — in the tab sequence, takes
// focus, activating it moves the journey on.

test("NFR-007: 搜尋 → 詳情 → 打包，全程鍵盤可達", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/", search: { q: "pdf 摘要" } });
  });
  await waitFor(has("PDF Summariser"));

  // The search field and its submit are both in the sequence before anything is
  // typed: a form whose only route in is a mouse click on a suggestion is not
  // keyboard-operable, however well labelled it is.
  expect(tabbables().some((el) => el.tagName === "INPUT")).toBe(true);

  await keyboardActivate("搜尋結果連結", byText("PDF Summariser"));
  await waitFor(has("可散布性與打包"));

  await keyboardActivate("打包入口", byText("打包並下載這個版本"));
  await waitFor(has("標準 Agent Skill 套件")); // the heading renders before the targets do

  // On the packaging page: pick a target, choose whether test cases travel, and
  // build. Radio and checkbox are native inputs, so the platform gives them
  // arrow/space handling — what matters here is that they are reachable and that
  // the button they gate is not offered while the preview refuses.
  //
  // Matched by name rather than by type: the site-wide feedback form also has
  // radios, and an earlier version of this walkthrough passed by activating one
  // of those instead of a packaging target.
  await keyboardActivate("打包目標選項", (el) => el.getAttribute("name") === "packaging-target");
  await keyboardActivate("Test Case 選項", (el) => el.getAttribute("type") === "checkbox");
  await waitFor(has("這些設定可以打包"));

  const build = tabbables().find(byText("建立下載套件"));
  expect(build, "建立下載套件 is not reachable by keyboard").toBeDefined();
  build!.focus();
  expect(document.activeElement).toBe(build);
}, 30000);

test("NFR-007: 全站回報入口是一個 <details>，用鍵盤打得開也送得出", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/workspace/downloads" });
  });
  await waitFor(has("pdf-summariser-v2.zip"));

  // Closed: the summary is the only thing in the sequence, and the form inside
  // is deliberately not — a control a browser skips must not count as reachable.
  const summary = tabbables().find(byText("回報問題"));
  expect(summary?.tagName).toBe("SUMMARY");
  expect(tabbables().some((el) => el.id === "feedback-message")).toBe(false);

  await act(async () => (summary as HTMLElement).click());
  const opened = tabbables();
  expect(opened.some((el) => el.id === "feedback-message")).toBe(true);
  expect(opened.some((el) => el.getAttribute("type") === "submit")).toBe(true);
}, 30000);

// --- 02:NFR-007「清楚的驗證訊息」 (04 丙-21 ②) --------------------------------
//
// axe covers the labels; it says nothing about what a form does when it refuses.
// The rule these three share: a refusal names what to fix, and a control that
// cannot be used yet says why rather than sitting there dead.

test("NFR-007: 空白的回報被擋下來時說得出要補什麼", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/" });
  });
  await waitFor(has("回報問題"));

  const summary = tabbables().find(byText("回報問題"))!;
  await act(async () => summary.click());
  await act(async () => {
    container
      .querySelector(".feedback-entry form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });

  const alert = container.querySelector('.feedback-entry [role="alert"]');
  expect(alert?.textContent).toContain("內容不能空白");
  await scan("/（回報問題，驗證訊息）");
}, 30000);

test("NFR-007: 不能建立的 Test Case 表單說得出還缺哪幾項", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/test-cases" });
  });
  await waitFor(has("建立新的 Test Case"));

  // The disabled submit is not the message; the sentence beside it is.
  const submit = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "建立",
  )!;
  expect(submit.disabled).toBe(true);
  const status = Array.from(container.querySelectorAll('[role="status"]')).find((el) =>
    (el.textContent ?? "").includes("還不能建立"),
  );
  expect(status?.textContent).toContain("選一個 Skill");
  expect(status?.textContent).toContain("填名稱");
  expect(status?.textContent).toContain("寫 User Prompt");
}, 30000);

test("NFR-007: 沒選檔案就按上傳，說的是下一步而不是錯誤碼", async () => {
  stubPlatform();
  await mount();
  await act(async () => {
    await router.navigate({ to: "/lab/datasets", search: { test_case: TEST_CASE } });
  });
  await waitFor(has("上傳前請先確認"));

  await keyboardActivate("上傳", byText("上傳"));
  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("請先選擇一個檔案");
}, 30000);
