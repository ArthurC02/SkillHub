/**
 * The smallest API surface that renders the three screens this tier drives.
 *
 * Deliberately not shared with `src/a11y.test.tsx`. That file's fixtures exist
 * to render every route for a structural scan; these exist to put specific
 * pixels on a real screen — two `.notice` bars whose background is an `rgba()`
 * token, and a three-column table wider than a phone. Merging them would make
 * one set answer to two purposes and neither would be readable.
 */
const SKILL_A = "11111111-1111-4111-8111-111111111111";
const SKILL_B = "22222222-2222-4222-8222-222222222222";

const FACETS = {
  tier: { value: "indexed", label: "已建索引", note: "語意索引已建立。" },
  risk: {
    scan_status: "scanned",
    level: "disclosed",
    warnings: 1,
    has_scripts: true,
    note: "含腳本，已揭露。",
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

/**
 * `degraded` and `partial_index` are both on so the page renders two `.notice`
 * bars. That is the point of this fixture: `.notice` is painted on --accent-bg,
 * an `rgba()` token, and `src/contrast.test.ts` says in its own header that it
 * cannot check those — alpha needs something that composites.
 */
export const SEARCH = {
  query: "pdf 摘要",
  results: [
    {
      ...FACETS,
      skill_id: SKILL_A,
      name: "PDF Summariser",
      summary: "把 PDF 整理成摘要",
      rank: 0.82,
      match_reason: "這個 Skill 直接處理 PDF 並輸出摘要。",
      match_reason_source: "model",
    },
    {
      ...FACETS,
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

/**
 * GET /policy/data-retention. Small enough to state in full, which is why the
 * layout test uses this page: /compare would need three SkillDetail bodies and
 * that interface has forty-odd fields, so the fixture would rot long before the
 * assertion it supports did.
 *
 * `collecting: false` is the deployment's real answer today, and it is the
 * branch that still renders the four-row table — the page discloses the events
 * either way.
 */
export const RETENTION = {
  collecting: false,
  retention_days: 0,
  note: "這個部署尚未定案保存期限。",
  events: [
    {
      name: "search_performed",
      when: "使用者送出一次搜尋",
      attributes: ["查詢字串長度", "結果筆數"],
      not_recorded: "查詢字串本身",
    },
    {
      name: "skill_detail_viewed",
      when: "開啟 Skill 詳情",
      attributes: ["skill_id"],
      not_recorded: "停留時間",
    },
    {
      name: "session_started",
      when: "登入成功",
      attributes: ["登入方式"],
      not_recorded: "IP 位址",
    },
    {
      name: "download_started",
      when: "按下下載",
      attributes: ["目標格式"],
      not_recorded: "檔案內容",
    },
  ],
};
