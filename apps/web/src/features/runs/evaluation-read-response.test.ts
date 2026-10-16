import { afterEach, expect, test, vi } from "vitest";
import { EVALUATION, OTHER_RUN, REVISIONS, RUN } from "../../testing/fixtures/platform";
import { getEvaluation, getEvaluationRevisions } from "./evaluation.service";

afterEach(() => vi.unstubAllGlobals());

const CRITERION = EVALUATION.criterion_results[0];
const EVIDENCE = CRITERION.evidence[0];
const FINDING = EVALUATION.deterministic_findings[0];
const STEP = { path: "0", kind: "match", result: "passed", reason: "符合" };
const REVISION = REVISIONS.revisions[0];

function withEvidence(evidence: unknown) {
  return { ...EVALUATION, criterion_results: [{ ...CRITERION, evidence: [evidence] }] };
}

const complete: [string, unknown, string?][] = [
  ["standard", EVALUATION],
  ...["met", "partially_met", "not_met", "undetermined"].map((overall): [string, unknown] => [
    overall,
    { ...EVALUATION, overall },
  ]),
  [
    "pending",
    {
      ...EVALUATION,
      status: "pending",
      overall: "undetermined",
      evaluated_at: null,
      criterion_results: [],
      deterministic_findings: [],
      feedback: undefined,
      judge_model: "",
      judge_prompt_version: "",
      cost: { evaluation_credits: null, source: "unreported", note: "" },
    },
  ],
  ["failed", { ...EVALUATION, status: "failed", overall: "undetermined" }],
  ["zero cost", { ...EVALUATION, cost: { ...EVALUATION.cost, evaluation_credits: 0 } }],
  [
    "unreported cost",
    { ...EVALUATION, cost: { evaluation_credits: null, source: "unreported", note: "未測量" } },
  ],
  ["estimated cost", { ...EVALUATION, cost: { ...EVALUATION.cost, source: "estimated" } }],
  [
    "maximum safe cost",
    { ...EVALUATION, cost: { ...EVALUATION.cost, evaluation_credits: Number.MAX_SAFE_INTEGER } },
  ],
  [
    "minimum safe cost",
    { ...EVALUATION, cost: { ...EVALUATION.cost, evaluation_credits: Number.MIN_SAFE_INTEGER } },
  ],
  [
    "optional fields absent",
    {
      ...EVALUATION,
      summary: undefined,
      rubric_version: undefined,
      superseded_at: undefined,
      feedback: undefined,
    },
  ],
  [
    "no judge or criteria",
    {
      ...EVALUATION,
      judge_model: "",
      judge_prompt_version: "",
      criterion_results: [],
      deterministic_findings: [],
    },
  ],
  [
    "false feedback without comment",
    { ...EVALUATION, feedback: { helpful: false, submitted_at: "2026-10-15T20:00:00+08:00" } },
  ],
  [
    "empty and populated check steps",
    {
      ...EVALUATION,
      criterion_results: [
        { ...CRITERION, steps: [] },
        {
          ...CRITERION,
          steps: ["passed", "failed", "undetermined"].map((result) => ({ ...STEP, result })),
        },
      ],
    },
  ],
  [
    "all criterion verdict sources",
    {
      ...EVALUATION,
      criterion_results: ["rule", "model", "user"].map((source) => ({ ...CRITERION, source })),
    },
  ],
  [
    "all criterion verdicts",
    {
      ...EVALUATION,
      criterion_results: ["passed", "failed", "undetermined"].map((result) => ({
        ...CRITERION,
        result,
      })),
    },
  ],
  [
    "all finding categories and severities",
    {
      ...EVALUATION,
      deterministic_findings: [
        "spec",
        "activation",
        "execution",
        "effect",
        "compatibility",
        "cost",
      ].flatMap((category) =>
        ["error", "warning", "info"].map((severity) => ({ ...FINDING, category, severity })),
      ),
    },
  ],
  ["legacy evidence without match", withEvidence({ ...EVIDENCE, match: undefined })],
  ...["exact", "normalized", "not_found", "not_checked"].map((match): [string, unknown] => [
    match + " evidence",
    withEvidence({ ...EVIDENCE, match }),
  ]),
  ...["trace_event", "artifact", "agent_output"].map((kind): [string, unknown] => [
    kind + " evidence",
    withEvidence({
      ...EVIDENCE,
      kind,
      reattributed_from: kind,
      artifact_path: "result.txt",
      byte_range: { start: 0, end: Number.MAX_SAFE_INTEGER },
      char_range: { start: 0, end: 0 },
      available: true,
      excerpt: "",
    }),
  ]),
  ["equivalent Run UUID", { ...EVALUATION, run_id: RUN.toUpperCase() }],
  [
    "requested history",
    { ...EVALUATION, superseded_at: "2026-10-15T20:00:00Z" },
    EVALUATION.evaluation_id,
  ],
  ["empty revision means current", EVALUATION, ""],
  ["extra metadata", { ...EVALUATION, extra: true }],
];

test.each(complete)(
  "a readable %s evaluation preserves the response",
  async (_name, body, revision) => {
    const fetch = vi.fn((_input: RequestInfo | URL) =>
      Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
    );
    vi.stubGlobal("fetch", fetch);
    await expect(getEvaluation(RUN, revision)).resolves.toEqual(body);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0]?.[0]).toBe(
      `/runs/${RUN}/evaluation${revision ? `?revision=${encodeURIComponent(revision)}` : ""}`,
    );
  },
);

const badEvidence: [string, unknown][] = [
  ["null evidence", null],
  ["unknown evidence kind", { ...EVIDENCE, kind: "unknown" }],
  ["missing excerpt", { ...EVIDENCE, excerpt: undefined }],
  ["object excerpt", { ...EVIDENCE, excerpt: {} }],
  ["missing excerpt truncation", { ...EVIDENCE, excerpt_truncated: undefined }],
  ["string excerpt truncation", { ...EVIDENCE, excerpt_truncated: "false" }],
  ["missing availability", { ...EVIDENCE, available: undefined }],
  ["string availability", { ...EVIDENCE, available: "false" }],
  ["unknown match", { ...EVIDENCE, match: "verified" }],
  ["null match", { ...EVIDENCE, match: null }],
  ["unknown attribution", { ...EVIDENCE, reattributed_from: "unknown" }],
  ["object trace identifier", { ...EVIDENCE, trace_event_id: {} }],
  ["object artifact path", { ...EVIDENCE, artifact_path: {} }],
  ["unreadable occurrence time", { ...EVIDENCE, occurred_at: "not-a-date" }],
  ["null occurrence time", { ...EVIDENCE, occurred_at: null }],
  ["null byte range", { ...EVIDENCE, byte_range: null }],
  ["missing byte end", { ...EVIDENCE, byte_range: { start: 0 } }],
  ["fractional byte offset", { ...EVIDENCE, byte_range: { start: 0.5, end: 2 } }],
  ["unsafe byte end", { ...EVIDENCE, byte_range: { start: 0, end: 9007199254740992 } }],
  ["null character range", { ...EVIDENCE, char_range: null }],
  ["missing character start", { ...EVIDENCE, char_range: { end: 2 } }],
  ["unsafe character start", { ...EVIDENCE, char_range: { start: -9007199254740992, end: 0 } }],
];

const badCriteria: [string, unknown][] = [
  ["null criterion", null],
  ["missing criterion identity", { ...CRITERION, criterion_id: undefined }],
  ["missing criterion text", { ...CRITERION, text: undefined }],
  ["missing criterion reason", { ...CRITERION, reason: undefined }],
  ["object criterion reason", { ...CRITERION, reason: {} }],
  ["unknown criterion verdict", { ...CRITERION, result: "unknown" }],
  ["missing criterion source", { ...CRITERION, source: undefined }],
  ["unknown criterion source", { ...CRITERION, source: "platform" }],
  ["null criterion evidence", { ...CRITERION, evidence: null }],
  ["missing criterion evidence", { ...CRITERION, evidence: undefined }],
  ["null steps", { ...CRITERION, steps: null }],
  ...[
    null,
    { ...STEP, path: undefined },
    { ...STEP, kind: undefined },
    { ...STEP, reason: undefined },
    { ...STEP, result: "unknown" },
  ].map((step, index): [string, unknown] => [
    `unreadable step ${index}`,
    { ...CRITERION, steps: [step] },
  ]),
];

const badCosts: [string, unknown][] = [
  ["missing cost", undefined],
  ["null cost", null],
  ["missing cost measurement", { ...EVALUATION.cost, evaluation_credits: undefined }],
  ["string cost measurement", { ...EVALUATION.cost, evaluation_credits: "28" }],
  ["fractional cost", { ...EVALUATION.cost, evaluation_credits: 1.5 }],
  ["unsafe positive cost", { ...EVALUATION.cost, evaluation_credits: 9007199254740992 }],
  ["unsafe negative cost", { ...EVALUATION.cost, evaluation_credits: -9007199254740992 }],
  ["unknown cost source", { ...EVALUATION.cost, source: "unknown" }],
  ["missing cost note", { ...EVALUATION.cost, note: undefined }],
];

const bad: [string, unknown][] = [
  ["null body", null],
  ["array body", []],
  ["primitive body", true],
  ["empty body", {}],
  ["missing evaluation identity", { ...EVALUATION, evaluation_id: undefined }],
  ["blank evaluation identity", { ...EVALUATION, evaluation_id: " " }],
  ["missing Run identity", { ...EVALUATION, run_id: undefined }],
  ["foreign Run", { ...EVALUATION, run_id: OTHER_RUN }],
  ["unknown status", { ...EVALUATION, status: "succeeded" }],
  ["missing status", { ...EVALUATION, status: undefined }],
  ["unknown overall verdict", { ...EVALUATION, overall: "passed" }],
  ["missing overall verdict", { ...EVALUATION, overall: undefined }],
  ["object summary", { ...EVALUATION, summary: {} }],
  ["object rubric", { ...EVALUATION, rubric_version: {} }],
  ["missing judge", { ...EVALUATION, judge_model: undefined }],
  ["missing prompt version", { ...EVALUATION, judge_prompt_version: undefined }],
  ["missing completeness verdict", { ...EVALUATION, evidence_complete: undefined }],
  ["string completeness verdict", { ...EVALUATION, evidence_complete: "false" }],
  ["missing evaluation time", { ...EVALUATION, evaluated_at: undefined }],
  ["unreadable evaluation time", { ...EVALUATION, evaluated_at: "not-a-date" }],
  ["unreadable supersession time", { ...EVALUATION, superseded_at: "not-a-date" }],
  ["missing criteria", { ...EVALUATION, criterion_results: undefined }],
  ["null criteria", { ...EVALUATION, criterion_results: null }],
  ["missing findings", { ...EVALUATION, deterministic_findings: undefined }],
  ["null findings", { ...EVALUATION, deterministic_findings: null }],
  ...badEvidence.map(([name, evidence]): [string, unknown] => [name, withEvidence(evidence)]),
  ...badCriteria.map(([name, criterion]): [string, unknown] => [
    name,
    { ...EVALUATION, criterion_results: [criterion] },
  ]),
  ...[
    null,
    { ...FINDING, category: "unknown" },
    { ...FINDING, severity: "unknown" },
    { ...FINDING, message: undefined },
    { ...FINDING, evidence: null },
    { ...FINDING, evidence: [null] },
  ].map((finding, index): [string, unknown] => [
    `unreadable finding ${index}`,
    { ...EVALUATION, deterministic_findings: [finding] },
  ]),
  ...badCosts.map(([name, cost]): [string, unknown] => [name, { ...EVALUATION, cost }]),
  ...[
    null,
    { helpful: "false", submitted_at: "2026-10-15T20:00:00Z" },
    { helpful: false },
    { helpful: false, submitted_at: "not-a-date" },
    { helpful: false, submitted_at: "2026-10-15T20:00:00Z", comment: {} },
  ].map((feedback, index): [string, unknown] => [
    `unreadable feedback ${index}`,
    { ...EVALUATION, feedback },
  ]),
  [
    "mixed readable and unreadable criteria",
    { ...EVALUATION, criterion_results: [CRITERION, null] },
  ],
];

test.each(bad)("an evaluation with %s remains an unreadable response", async (_name, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(getEvaluation(RUN)).rejects.toThrow("Invalid evaluation response");
});

test("a different Evaluation cannot answer a requested revision", async () => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(EVALUATION), { status: 200 })),
  );
  await expect(getEvaluation(RUN, "eval-1")).rejects.toThrow("Invalid evaluation response");
});

test.each<[string, unknown]>([
  ["standard", REVISIONS],
  ["empty", { revisions: [] }],
  [
    "pending",
    {
      revisions: [
        {
          ...REVISION,
          overall: "undetermined",
          evaluated_at: null,
          superseded_at: null,
          judge_prompt_version: "",
        },
      ],
    },
  ],
  [
    "all verdicts",
    {
      revisions: ["met", "partially_met", "not_met", "undetermined"].map((overall) => ({
        ...REVISION,
        overall,
        rubric_version: "",
      })),
    },
  ],
  ["extra metadata", { revisions: [{ ...REVISION, extra: true }], extra: true }],
])("a readable %s history preserves the response", async (_name, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL) =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getEvaluationRevisions(RUN)).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe(`/runs/${RUN}/evaluation/revisions`);
});

test.each<[string, unknown]>([
  ["null body", null],
  ["array body", []],
  ["missing list", {}],
  ["null list", { revisions: null }],
  ...[
    null,
    { ...REVISION, evaluation_id: undefined },
    { ...REVISION, evaluation_id: " " },
    { ...REVISION, judge_prompt_version: undefined },
    { ...REVISION, overall: "unknown" },
    { ...REVISION, rubric_version: {} },
    { ...REVISION, evaluated_at: undefined },
    { ...REVISION, evaluated_at: "not-a-date" },
    { ...REVISION, superseded_at: undefined },
    { ...REVISION, superseded_at: "not-a-date" },
  ].map((revision, index): [string, unknown] => [
    `unreadable row ${index}`,
    { revisions: [revision] },
  ]),
  ["mixed readable and unreadable history", { revisions: [REVISION, null] }],
])("a history with %s remains an unreadable response", async (_name, body) => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
  );
  await expect(getEvaluationRevisions(RUN)).rejects.toThrow(
    "Invalid evaluation revisions response",
  );
});
