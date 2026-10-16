import { afterEach, expect, test, vi } from "vitest";
import { COMPARISON, EVALUATION, OTHER_RUN, RUN } from "../../testing/fixtures/platform";
import { getRunComparison } from "./evaluation.service";

afterEach(() => vi.unstubAllGlobals());

const SIDE = COMPARISON.runs[0];
const ROW = COMPARISON.criterion_matrix[0];

function withSide(patch: Record<string, unknown>) {
  return { ...COMPARISON, runs: [{ ...SIDE, ...patch }, COMPARISON.runs[1]] };
}

function withRow(patch: Record<string, unknown>) {
  return { ...COMPARISON, criterion_matrix: [{ ...ROW, ...patch }] };
}

const readable: [string, unknown][] = [
  ["standard", COMPARISON],
  ["no criteria", { ...COMPARISON, criterion_matrix: [] }],
  ["no version diff", { ...COMPARISON, version_diff_url: undefined }],
  ["extra metadata", { ...COMPARISON, extra: true }],
  ["future Run status", withSide({ status: "future-status" })],
  [
    "optional side fields absent",
    withSide({
      evaluation: undefined,
      test_case_id: undefined,
      final_output: undefined,
      errors: undefined,
      duration_ms: undefined,
    }),
  ],
  ["empty errors", withSide({ errors: [] })],
  ["error without optional details", withSide({ errors: [{}] })],
  [
    "different skill and test case",
    withSide({ skill_id: "another-skill", test_case_id: "another-case" }),
  ],
  [
    "equivalent UUID casing",
    {
      ...COMPARISON,
      runs: COMPARISON.runs.map((side) => ({ ...side, run_id: side.run_id.toUpperCase() })),
      criterion_matrix: [
        {
          ...ROW,
          results: ROW.results.map((result) => ({
            ...result,
            run_id: result.run_id.toUpperCase(),
          })),
        },
      ],
    },
  ],
  ...["pending", "completed", "failed"].map((status): [string, unknown] => [
    status + " evaluation",
    withSide({ evaluation: { ...EVALUATION, status } }),
  ]),
  ...["met", "partially_met", "not_met", "undetermined"].map((overall): [string, unknown] => [
    overall + " evaluation",
    withSide({ evaluation: { ...EVALUATION, overall } }),
  ]),
  ...[null, 0, Number.MIN_SAFE_INTEGER, Number.MAX_SAFE_INTEGER].map(
    (credits): [string, unknown] => [
      String(credits) + " Run cost",
      withSide({ cost: { ...SIDE.cost, credits } }),
    ],
  ),
  ...[Number.MIN_SAFE_INTEGER, 0, Number.MAX_SAFE_INTEGER].map((duration_ms): [string, unknown] => [
    String(duration_ms) + " duration",
    withSide({ duration_ms }),
  ]),
  ...["passed", "failed", "undetermined", null].map((result): [string, unknown] => [
    String(result) + " criterion",
    withRow({ results: [{ run_id: RUN, result }, ROW.results[1]] }),
  ]),
  ...["rule", "model", "user"].map((source): [string, unknown] => [
    source + " criterion",
    withRow({ results: [{ ...ROW.results[0], source }, ROW.results[1]] }),
  ]),
];

test.each(readable)("a readable %s comparison preserves the response", async (_name, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getRunComparison(RUN, OTHER_RUN)).resolves.toEqual(JSON.parse(JSON.stringify(body)));
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe(`/runs/${RUN}/comparison?against=${OTHER_RUN}`);
});

const unreadable: [string, unknown][] = [
  ["null body", null],
  ["array body", []],
  ["missing sides", { ...COMPARISON, runs: undefined }],
  ["null sides", { ...COMPARISON, runs: null }],
  ["no sides", { ...COMPARISON, runs: [] }],
  ["one side", { ...COMPARISON, runs: [SIDE] }],
  ["three sides", { ...COMPARISON, runs: [...COMPARISON.runs, SIDE] }],
  ["null side", { ...COMPARISON, runs: [null, COMPARISON.runs[1]] }],
  ["reversed sides", { ...COMPARISON, runs: [...COMPARISON.runs].reverse() }],
  ["wrong other Run", { ...COMPARISON, runs: [SIDE, { ...COMPARISON.runs[1], run_id: RUN }] }],
  ...[
    ["missing Run identity", { run_id: undefined }],
    ["different Run", { run_id: OTHER_RUN }],
    ["missing skill", { skill_id: undefined }],
    ["blank skill", { skill_id: " " }],
    ["missing Version", { skill_version_id: undefined }],
    ["blank Version", { skill_version_id: " " }],
    ["invalid test case", { test_case_id: {} }],
    ["missing status", { status: undefined }],
    ["object output", { final_output: {} }],
    ["missing input availability", { inputs_available: undefined }],
    ["string input availability", { inputs_available: "false" }],
    ["missing cost", { cost: undefined }],
    ["null cost", { cost: null }],
    ["missing cost amount", { cost: { ...SIDE.cost, credits: undefined } }],
    ["missing cost floor", { cost: { ...SIDE.cost, is_lower_bound: undefined } }],
    ["unconfirmed cost floor", { cost: { ...SIDE.cost, is_lower_bound: false } }],
    ["missing cost source", { cost: { ...SIDE.cost, authoritative_source: undefined } }],
    ["null errors", { errors: null }],
    ["null error", { errors: [null] }],
    ["invalid error category", { errors: [{ category: {} }] }],
    ["invalid error code", { errors: [{ code: {} }] }],
    ["invalid error message", { errors: [{ message: {} }] }],
    ["null evaluation", { evaluation: null }],
    ["missing evaluation identity", { evaluation: { ...EVALUATION, evaluation_id: undefined } }],
    ["blank evaluation identity", { evaluation: { ...EVALUATION, evaluation_id: " " } }],
    ["unknown evaluation status", { evaluation: { ...EVALUATION, status: "future" } }],
    ["unknown evaluation verdict", { evaluation: { ...EVALUATION, overall: "future" } }],
    ["missing evaluation cost", { evaluation: { ...EVALUATION, cost: undefined } }],
  ].map(([name, patch]): [string, unknown] => [
    String(name),
    withSide(patch as Record<string, unknown>),
  ]),
  ...[-9007199254740992, 9007199254740992, 0.5].flatMap((amount): [string, unknown][] => [
    [String(amount) + " Run cost", withSide({ cost: { ...SIDE.cost, credits: amount } })],
    [String(amount) + " duration", withSide({ duration_ms: amount })],
  ]),
  ["missing criteria", { ...COMPARISON, criterion_matrix: undefined }],
  ["null criteria", { ...COMPARISON, criterion_matrix: null }],
  ["null criterion", { ...COMPARISON, criterion_matrix: [null] }],
  ...[
    ["missing criterion identity", { criterion_id: undefined }],
    ["missing criterion text", { text: undefined }],
    ["missing results", { results: undefined }],
    ["null results", { results: null }],
    ["no results", { results: [] }],
    ["one result", { results: [ROW.results[0]] }],
    ["three results", { results: [...ROW.results, ROW.results[0]] }],
    ["null result", { results: [null, ROW.results[1]] }],
    ["reversed results", { results: [...ROW.results].reverse() }],
    ["wrong result Run", { results: [{ ...ROW.results[0], run_id: OTHER_RUN }, ROW.results[1]] }],
    ["missing result", { results: [{ run_id: RUN }, ROW.results[1]] }],
    ["unknown result", { results: [{ run_id: RUN, result: "future" }, ROW.results[1]] }],
    ["unknown source", { results: [{ ...ROW.results[0], source: "future" }, ROW.results[1]] }],
    ["null source", { results: [{ ...ROW.results[0], source: null }, ROW.results[1]] }],
  ].map(([name, patch]): [string, unknown] => [
    String(name),
    withRow(patch as Record<string, unknown>),
  ]),
  ["invalid diff URL", { ...COMPARISON, version_diff_url: {} }],
];

test.each(unreadable)("a comparison with %s remains unreadable", async (_name, body) => {
  vi.stubGlobal("fetch", () => Promise.resolve(new Response(JSON.stringify(body))));
  await expect(getRunComparison(RUN, OTHER_RUN)).rejects.toThrow("Invalid run comparison response");
});
