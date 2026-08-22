/**
 * The platform's answers, as one set of fixtures for both test tiers.
 *
 * These lived inside `a11y.test.tsx` and were reachable only by vitest, so the
 * browser tier (ADR-036) could render 2 of the 17 routes the jsdom tier already
 * covered — and the 15 it could not reach were the 15 nobody had ever looked at.
 * Hand-writing a second set was tried and abandoned twice: `SkillDetail` alone
 * has forty-odd fields, and the copy rots faster than the assertions it holds up.
 *
 * So the data is here and the transport is not. vitest stubs `fetch`, Playwright
 * uses `page.route`, and both ask `platformResponse` the same question.
 */

export const SKILL = "11111111-1111-1111-1111-111111111111";
export const SKILL_B = "aaaaaaaa-2222-2222-2222-222222222222";
export const VERSION = "22222222-2222-2222-2222-222222222222";
export const TEST_CASE = "33333333-3333-3333-3333-333333333333";
export const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
export const OTHER_RUN = "5c2e8a10-4b6d-4c31-9f77-2ab3d4e5f608";
export const ARTIFACT = "44444444-4444-4444-4444-444444444444";

// --- fixtures ---------------------------------------------------------------
//
// Deliberately the busy states rather than the empty ones: an empty page has no
// badges, no disclosures, no tables and no form controls, so scanning it would
// prove nothing about the markup a real reader meets.

export const HIT_FACETS = {
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

export const SEARCH = {
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
  limit: 20,
  truncated: false,
  no_results: false,
  filtered_out: false,
};

export function skillDetail(id: string, name: string) {
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

export const FILES = {
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

export const TARGETS = {
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

export const PREVIEW = {
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

export const ARTIFACT_ROW = {
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

export const DOWNLOADS = {
  downloads: [
    ARTIFACT_ROW,
    {
      ...ARTIFACT_ROW,
      artifact_id: "expired-1",
      expires_at: "2026-01-01T00:00:00Z",
    },
  ],
};

export const RUNS = {
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
      // 04 丙-32: the second axis. Required and never null — 未評估 is a value, not an
      // omission, because an empty verdict beside 「執行完成」 reads as a pass.
      evaluation: {
        value: "met",
        label: "符合",
        note: "依這個 Run 當時的驗收條件判定為符合。",
      },
      created_at: "2026-08-17T00:00:00Z",
      finished_at: "2026-08-17T00:04:00Z",
    },
  ],
};

export const RUN_ARTIFACTS = {
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

export const TEST_CASE_DRAFT = {
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

export const PREFLIGHT = {
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
      // NOT `4 << 30` / `8 << 30`. Those were transcribed from
      // DefaultResourceLimits() in Go, where an untyped constant is
      // arbitrary-precision; JS `<<` is 32-bit, so both evaluate to 0 and the
      // pre-run screen asked the user to confirm 記憶體 0 B、磁碟 0 B — a
      // ceiling that passed every rule in the design system and was still
      // wrong (設計 §2.2).
      memory_bytes: 4 * 1024 ** 3, // 4 GiB
      disk_bytes: 8 * 1024 ** 3, // 8 GiB
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

export const LIMITS = {
  max_file_bytes: 25 << 20,
  max_test_case_bytes: 100 << 20,
  max_files_per_test_case: 20,
  retention_days: 90,
  allowed_kinds: ["text (.txt .md .csv)", "documents (.pdf .docx)"],
  note: "file type is decided by content, not by file extension",
};

// Collecting, so the policy page renders the table rather than the 不收集 note;
// the other branch is covered in policy.test.tsx.
export const RETENTION_POLICY = {
  collecting: true,
  retention_days: 180,
  events: [
    {
      name: "search_performed",
      when: "a search is submitted",
      attributes: ["query_length", "query_language", "result_count"],
      not_recorded: "not one word of the query itself",
    },
    {
      name: "session_started",
      when: "a visit begins",
      attributes: ["session_id", "occurred_at"],
      not_recorded: "session_id is not the login session token",
    },
  ],
  note: "there is no free-text column anywhere in the table",
};

export const TRACE_GENERAL = {
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

export const TRACE_ADVANCED = {
  run_id: RUN,
  complete: false,
  next_after: 1,
  has_more: false,
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

export const EVALUATION = {
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
export const REVISIONS = {
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

export const SUGGESTIONS = {
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

export function comparisonSide(runId: string, evaluated: boolean) {
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

export const COMPARISON = {
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

export const VERSION_DIFF = {
  files: [
    { path: "SKILL.md", status: "modified", diff: "@@ -1 +1 @@\n-old\n+new" },
    { path: "assets/logo.png", status: "modified" },
  ],
};

function ok(body: unknown, status = 200) {
  return { body, status };
}

/**
 * One URL in, one response out. Takes the whole URL rather than the path
 * because the trace read distinguishes its two modes by query string.
 *
 * Moved here verbatim from the `vi.stubGlobal("fetch", …)` it used to live in;
 * only the helper's name changed, so the routing table below is the same one
 * the accessibility suite has been asserting against all along.
 */
export function platformResponse(input: string): { body: unknown; status: number } {
  const url = String(input).replace(/^https?:\/\/[^/]+/, "");
  const path = url.split("?")[0];

  if (path.startsWith("/api/skills/search")) return ok(SEARCH);
  if (path.endsWith("/files")) return ok(FILES);
  if (path.startsWith("/api/skills/"))
    return ok(skillDetail(path.slice("/api/skills/".length), "PDF Summariser"));

  // The busy state here too: an account with a deletion already pending is the
  // one that renders the badge, the date and the way out of it.
  if (path === "/me")
    return ok({
      user_id: "u-1",
      email: "tester@example.com",
      display_name: "tester",
      workspace_id: "ws-1",
      deletion_requested_at: "2026-08-17T00:00:00Z",
      purge_after: "2026-09-16T00:00:00Z",
    });
  if (path === "/policy/data-retention") return ok(RETENTION_POLICY);
  if (path === "/packaging/targets") return ok(TARGETS);
  if (path.endsWith("/packaging/preview")) return ok(PREVIEW);
  if (path === "/downloads") return ok(DOWNLOADS);
  if (path === `/downloads/${ARTIFACT}/records`)
    return ok({ records: [{ downloaded_at: "2026-08-17T09:00:00Z", actor: "tester" }] });
  if (path === "/runs") return ok(RUNS);
  if (path.endsWith("/artifacts")) return ok(RUN_ARTIFACTS);

  if (path === "/test-cases/limits") return ok(LIMITS);
  if (path.endsWith("/datasets"))
    return ok({
      datasets: [
        {
          dataset_id: "d0",
          file_name: "rows.csv",
          content_type: "text/csv",
          size_bytes: 1024,
          expires_at: "2026-11-15T00:00:00Z",
        },
      ],
      total_bytes: 1024,
    });
  // The list row carries the aggregates the list renders; the detail read
  // does not, which is why they are added here rather than to the fixture.
  if (path === "/test-cases")
    return ok({
      test_cases: [
        {
          ...TEST_CASE_DRAFT,
          skill_name: "PDF Summariser",
          criteria_confirmed: 1,
          criteria_total: 2,
          has_rubric: true,
        },
      ],
    });
  if (path.startsWith("/test-cases/")) return ok(TEST_CASE_DRAFT);
  if (path === "/skills")
    return ok({
      skills: [
        {
          skill_id: SKILL,
          name: "PDF Summariser",
          summary: "摘要",
          // `unknown` is what a user's own import carries by default (0027), so
          // this is the common case for a workspace list rather than an edge.
          redistribution: "unknown",
          access_restriction: null,
          // 04 丙-31: the two facets that make this list decidable rather than
          // merely enumerable. An import, so both are present — the fork case,
          // where neither is, has its own test in workspace.test.tsx.
          risk: {
            scan_status: "scanned",
            level: "disclosed",
            warnings: 0,
            has_scripts: true,
            note: "來自匯入時的靜態掃描,不執行套件內任何程式碼;開啟 Skill 可看逐項結果。",
          },
          verification: {
            value: "scanned",
            label: "已掃描",
            note: "匯入這個版本時做過靜態掃描,不執行套件內任何程式碼;逐項結果在 Skill 頁面。",
            scanned_at: "2026-08-01T10:00:00Z",
          },
        },
      ],
      limit: 100,
      truncated: false,
    });
  if (path.includes("/versions/diff")) return ok(VERSION_DIFF);
  if (path.endsWith("/runs/preflight")) return ok(PREFLIGHT);

  if (path.endsWith("/trace")) return ok(url.includes("advanced") ? TRACE_ADVANCED : TRACE_GENERAL);
  if (path.endsWith("/evaluation/revisions")) return ok(REVISIONS);
  if (path.endsWith("/evaluation")) return ok(EVALUATION);
  if (path.endsWith("/suggestions")) return ok(SUGGESTIONS);
  if (path.endsWith("/comparison")) return ok(COMPARISON);
  if (path.startsWith("/runs/"))
    return ok({
      run_id: RUN,
      skill_id: SKILL,
      skill_version_id: VERSION,
      test_case_snapshot_id: "snap-1",
      test_case_id: TEST_CASE,
    });

  return ok({ error: "not found" }, 404);
}
