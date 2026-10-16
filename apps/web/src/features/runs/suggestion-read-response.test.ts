import { afterEach, expect, test, vi } from "vitest";
import { SUGGESTIONS } from "../../testing/fixtures/platform";
import { getRunSuggestions, getSuggestionDiff } from "./evaluation.service";

afterEach(() => vi.unstubAllGlobals());

function reply(body: unknown) {
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

function withItem(fields: Record<string, unknown>) {
  return { ...SUGGESTIONS, suggestions: [{ ...SUGGESTIONS.suggestions[0], ...fields }] };
}

test.each<[string, unknown]>([
  ["an empty list", { ...SUGGESTIONS, suggestions: [] }],
  ["a readable suggestion", SUGGESTIONS],
  [
    "all categories and choices",
    {
      ...SUGGESTIONS,
      suggestions: ["skill", "runtime", "mcp", "tool", "dataset"].map((category, index) => ({
        ...SUGGESTIONS.suggestions[0],
        suggestion_id: `s${index + 1}`,
        category,
        decision: ["pending", "accepted", "rejected"][index % 3],
      })),
    },
  ],
  [
    "optional provenance and evidence",
    withItem({
      decided_at: "2026-10-16T09:00:00Z",
      applied_skill_version_id: "version-1",
      evidence: [{ kind: "agent_output", excerpt: "", excerpt_truncated: false, available: true }],
    }),
  ],
  ["empty descriptive strings", withItem({ problem: "", target_path: "", expected_impact: "" })],
  ["additional fields", { ...withItem({ future_field: true }), future_field: true }],
])("%s is readable after one scoped GET", async (_label, body) => {
  const fetch = reply(body);
  await expect(getRunSuggestions("run /?")).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/runs/run%20%2F%3F/suggestions");
  expect(fetch.mock.calls[0]?.[1]?.method ?? "GET").toBe("GET");
});

test.each<[string, unknown]>([
  ["null", null],
  ["an array", []],
  ["an incomplete envelope", {}],
  ["a missing evaluation", { suggestions: [] }],
  ["a blank evaluation", { evaluation_id: " ", suggestions: [] }],
  ["a missing list", { evaluation_id: SUGGESTIONS.evaluation_id }],
  ["a null list", { ...SUGGESTIONS, suggestions: null }],
  ["an object list", { ...SUGGESTIONS, suggestions: {} }],
  ["a null item", { ...SUGGESTIONS, suggestions: [null] }],
  ["a missing identifier", withItem({ suggestion_id: undefined })],
  ["a blank identifier", withItem({ suggestion_id: " " })],
  ["an unknown category", withItem({ category: "unknown" })],
  ["a missing problem", withItem({ problem: undefined })],
  ["an object problem", withItem({ problem: {} })],
  ["null evidence", withItem({ evidence: null })],
  ["a null evidence reference", withItem({ evidence: [null] })],
  ["an unreadable evidence reference", withItem({ evidence: [{ kind: "agent_output" }] })],
  ["a missing target", withItem({ target_path: undefined })],
  ["an object impact", withItem({ expected_impact: {} })],
  ["a missing choice", withItem({ decision: undefined })],
  ["an unknown choice", withItem({ decision: "approve" })],
  ["an unreadable decision date", withItem({ decided_at: "not a timestamp" })],
  ["a null decision date", withItem({ decided_at: null })],
  ["an object applied version", withItem({ applied_skill_version_id: {} })],
  ["a blank applied version", withItem({ applied_skill_version_id: " " })],
])("a suggestions list with %s is rejected before rendering", async (_label, body) => {
  reply(body);
  await expect(getRunSuggestions("run-1")).rejects.toThrow();
});

test.each<[string, unknown]>([
  [
    "an applicable change",
    { target_path: "SKILL.md", applicable: true, unified_diff: "+ clearer" },
  ],
  ["an empty change", { target_path: "SKILL.md", applicable: true, unified_diff: "" }],
  ["an unspecified refusal", { target_path: "SKILL.md", applicable: false }],
  ["an empty target", { target_path: "", applicable: false }],
  ...[
    "path_out_of_bounds",
    "target_changed",
    "validation_blocked",
    "access_restricted",
    "diff_unavailable",
  ].map<[string, unknown]>((reason) => [
    reason,
    { target_path: "SKILL.md", applicable: false, blocked_reason: reason },
  ]),
  ["additional fields", { target_path: "SKILL.md", applicable: true, future_field: true }],
])("%s is a readable preview after one scoped GET", async (_label, body) => {
  const fetch = reply(body);
  await expect(getSuggestionDiff("s /?")).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/suggestions/s%20%2F%3F/diff");
  expect(fetch.mock.calls[0]?.[1]?.method ?? "GET").toBe("GET");
});

test.each<[string, unknown]>([
  ["null", null],
  ["an array", []],
  ["an incomplete envelope", {}],
  ["a missing target", { applicable: false }],
  ["an object target", { target_path: {}, applicable: false }],
  ["a missing applicability", { target_path: "SKILL.md" }],
  ["a string applicability", { target_path: "SKILL.md", applicable: "true" }],
  ["an object change", { target_path: "SKILL.md", applicable: true, unified_diff: {} }],
  ["a null change", { target_path: "SKILL.md", applicable: true, unified_diff: null }],
  ["an unknown reason", { target_path: "SKILL.md", applicable: false, blocked_reason: "unknown" }],
  ["a null reason", { target_path: "SKILL.md", applicable: false, blocked_reason: null }],
])("a preview with %s is rejected before inventing a result", async (_label, body) => {
  reply(body);
  await expect(getSuggestionDiff("s1")).rejects.toThrow();
});
