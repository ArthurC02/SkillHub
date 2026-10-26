import { expect, test } from "vitest";
import { CATALOG, SEARCH } from "../../testing/fixtures/platform";
import { emptySearchIntent } from "../../core/api/searchIntent";
import { readCatalogResponse, readSearchResponse } from "./search-response.model";

const malformed: [string, unknown][] = [
  ["null envelope", null],
  ["array envelope", []],
  ["missing results", { ...CATALOG, results: undefined }],
  ["null results", { ...CATALOG, results: null }],
  ["object results", { ...CATALOG, results: {} }],
  ["null row", { ...CATALOG, results: [null] }],
  ["missing total", { ...CATALOG, total: undefined }],
  ["negative total", { ...CATALOG, total: -1 }],
  ["fractional total", { ...CATALOG, total: 3.5 }],
  ["unsafe total", { ...CATALOG, total: 9007199254740992 }],
  ["missing limit", { ...CATALOG, limit: undefined }],
  ["negative limit", { ...CATALOG, limit: -1 }],
  ["fractional limit", { ...CATALOG, limit: 3.5 }],
  ["unsafe limit", { ...CATALOG, limit: 9007199254740992 }],
  ["missing truncation", { ...CATALOG, truncated: undefined }],
  ["text truncation", { ...CATALOG, truncated: "false" }],
  ["hidden truncation", { ...CATALOG, total: 4 }],
  ["false truncation", { ...CATALOG, truncated: true }],
  ["rows over limit", { ...CATALOG, limit: 2 }],
];

const row = CATALOG.results[0];
const invalidRows: [string, unknown][] = [
  ["missing identity", { ...row, skill_id: undefined }],
  ["invalid identity", { ...row, skill_id: "not-an-id" }],
  ["missing name", { ...row, name: undefined }],
  ["missing summary", { ...row, summary: undefined }],
  ["missing summary source", { ...row, summary_source: undefined }],
  ["unknown summary source", { ...row, summary_source: "unknown" }],
  ["missing score", { ...row, rank: undefined }],
  ["negative score", { ...row, rank: -0.01 }],
  ["score above one", { ...row, rank: 1.01 }],
  ["infinite score", { ...row, rank: Infinity }],
  ["missing tier", { ...row, tier: undefined }],
  ["null category", { ...row, category: null }],
  ["missing label", { ...row, tier: { value: "curated", note: "" } }],
  ["missing label note", { ...row, category: { value: "documents", label: "文件" } }],
  ["missing risk", { ...row, risk: undefined }],
  ["missing scan status", { ...row, risk: { ...row.risk, scan_status: undefined } }],
  ["unknown risk", { ...row, risk: { ...row.risk, level: "safe" } }],
  [
    "unavailable scan claims clean",
    { ...row, risk: { ...row.risk, scan_status: "unavailable", level: "none" } },
  ],
  ["missing warnings", { ...row, risk: { ...row.risk, warnings: undefined } }],
  ["negative warnings", { ...row, risk: { ...row.risk, warnings: -1 } }],
  ["missing disclosures", { ...row, risk: { ...row.risk, disclosures: undefined } }],
  ["null disclosure", { ...row, risk: { ...row.risk, disclosures: [null] } }],
  [
    "missing disclosure note",
    { ...row, risk: { ...row.risk, disclosures: [{ code: "future", label: "揭露" }] } },
  ],
  ["missing risk note", { ...row, risk: { ...row.risk, note: undefined } }],
  ["missing dependencies", { ...row, dependencies: undefined }],
  ["invalid dependency", { ...row, dependencies: [null] }],
  ["missing compatibility", { ...row, compatibility: undefined }],
  ...(["spec_validation", "capability", "runtime"] as const).map((axis): [string, unknown] => [
    `missing ${axis}`,
    { ...row, compatibility: { ...row.compatibility, [axis]: undefined } },
  ]),
  [
    "unknown runtime",
    {
      ...row,
      compatibility: { ...row.compatibility, runtime: { value: "safe", label: "", note: "" } },
    },
  ],
  [
    "missing compatibility note",
    { ...row, compatibility: { ...row.compatibility, note: undefined } },
  ],
  [
    "invalid measurement time",
    { ...row, compatibility: { ...row.compatibility, measured_at: "bad" } },
  ],
  [
    "invalid runtime image",
    { ...row, compatibility: { ...row.compatibility, runtime_image: null } },
  ],
  ["invalid verification time", { ...row, verified_at: "bad" }],
  ["invalid score note", { ...row, rank_note: 1 }],
  ["invalid match reason", { ...row, match_reason: {} }],
  ["unknown match source", { ...row, match_reason_source: "unknown" }],
];

test.each(malformed)("catalog rejects %s", (_name, value) => {
  expect(() => readCatalogResponse(value)).toThrow("Invalid catalog response");
});

test.each(invalidRows)("catalog rejects %s before caching", (_name, value) => {
  expect(() => readCatalogResponse({ ...CATALOG, results: [value], total: 1 })).toThrow(
    "Invalid catalog response",
  );
});

test.each(["query", "degraded", "partial_index", "no_results", "filtered_out"])(
  "search rejects missing %s",
  (key) => {
    expect(() => readSearchResponse({ ...SEARCH, [key]: undefined })).toThrow(
      "Invalid search response",
    );
  },
);

const interpretation = {
  status: "analyzed",
  intent: emptySearchIntent(),
  keywords: [],
  filters: {},
};

test.each<[string, unknown]>([
  ["catalog without search flags", CATALOG],
  ["invalid shared rows", { ...SEARCH, results: [null] }],
  [
    "conflicting empty states",
    { ...SEARCH, results: [], total: 0, no_results: true, filtered_out: true },
  ],
  ["results advertised as absent", { ...SEARCH, no_results: true }],
  ["results advertised as filtered", { ...SEARCH, filtered_out: true }],
  ["invalid reason query", { ...SEARCH, reason_query: {} }],
  ["invalid suggestion", { ...SEARCH, query_suggestion: [] }],
  ["invalid degradation reason", { ...SEARCH, degraded_reason: null }],
  ["null interpretation", { ...SEARCH, interpretation: null }],
  ...(
    [
      ["unknown status", { ...interpretation, status: "success" }],
      ["missing intent field", { ...interpretation, intent: { input: null } }],
      ["empty intent field", { ...interpretation, intent: { ...emptySearchIntent(), input: "" } }],
      [
        "intent above limit",
        { ...interpretation, intent: { ...emptySearchIntent(), input: "字".repeat(2001) } },
      ],
      ["too many keywords", { ...interpretation, keywords: Array(9).fill("字") }],
      ["empty keyword", { ...interpretation, keywords: [""] }],
      ["keyword above limit", { ...interpretation, keywords: ["字".repeat(129)] }],
      ["unknown filter", { ...interpretation, filters: { invented: "yes" } }],
      ["invalid filter", { ...interpretation, filters: { tier: "safe" } }],
      ["unknown fallback", { ...interpretation, fallback_reason: "error" }],
      ["invalid model", { ...interpretation, model: null }],
    ] satisfies [string, unknown][]
  ).map(([name, value]): [string, unknown] => [name, { ...SEARCH, interpretation: value }]),
])("search rejects %s", (_name, value) => {
  expect(() => readSearchResponse(value)).toThrow("Invalid search response");
});

test("valid rows retain disclosures and unverified facts without a server-injected fit verdict", () => {
  const value = {
    ...CATALOG,
    results: CATALOG.results.map((hit) => ({ ...hit, match_fits: true })),
  };
  const parsed = readCatalogResponse(value);
  expect(parsed).toStrictEqual(CATALOG);
  expect(parsed.results[0].risk.disclosures[0].code).toBe("script-file");
  expect(parsed.results[0].compatibility.runtime.value).toBe("unverified");
  expect(value.results[0].match_fits).toBe(true);
});

test("catalog accepts zero results and accurately declared truncation", () => {
  expect(readCatalogResponse({ results: [], limit: 0, total: 0, truncated: false })).toEqual({
    results: [],
    limit: 0,
    total: 0,
    truncated: false,
  });
  expect(readCatalogResponse({ ...CATALOG, total: 9007199254740991, truncated: true }).total).toBe(
    9007199254740991,
  );
});

test.each(["no_results", "filtered_out"])("search accepts the %s empty state", (key) => {
  const value = { ...SEARCH, results: [], total: 0, limit: 0, [key]: true };
  expect(readSearchResponse(value)).toEqual(value);
});

test("search accepts bounded scores, future disclosures and unavailable scans as unknown", () => {
  const value = {
    ...SEARCH,
    results: [0, 1].map((rank) => ({
      ...SEARCH.results[0],
      rank,
      risk: {
        scan_status: "unavailable",
        level: "unknown",
        warnings: 0,
        disclosures: [{ code: "future-code", label: "揭露", note: "內容" }],
        note: "未掃描",
      },
    })),
  };
  expect(readSearchResponse(value)).toEqual(value);
});

test("interpretation accepts per-field Unicode limits without imposing the correction request's combined limit", () => {
  const value = {
    ...SEARCH,
    interpretation: {
      ...interpretation,
      intent: { ...emptySearchIntent(), input: "😀".repeat(2000), output: "字".repeat(2000) },
      keywords: Array(8).fill("字".repeat(128)),
    },
  };
  expect(readSearchResponse(value)).toEqual(value);
});
