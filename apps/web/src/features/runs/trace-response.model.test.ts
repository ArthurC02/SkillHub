import { expect, test } from "vitest";
import { RUN, platformResponse } from "../../testing/fixtures/platform";
import { isTraceAdvanced, isTraceSummary } from "./trace-response.model";

const summary = platformResponse(`/runs/${RUN}/trace`).body as Record<string, unknown>;
const advanced = platformResponse(`/runs/${RUN}/trace?mode=advanced`).body as Record<
  string,
  unknown
>;

test.each([
  { name: "general summary", value: summary, check: isTraceSummary },
  { name: "advanced events", value: advanced, check: isTraceAdvanced },
  {
    name: "case-insensitive summary identity",
    value: { ...summary, run_id: RUN.toUpperCase() },
    check: isTraceSummary,
  },
  {
    name: "case-insensitive events identity",
    value: { ...advanced, run_id: RUN.toUpperCase() },
    check: isTraceAdvanced,
  },
  {
    name: "zero observed events",
    value: {
      ...advanced,
      streams: [],
      events: [],
      next_after: 0,
      has_more: false,
      complete: false,
    },
    check: isTraceAdvanced,
  },
  {
    name: "unreported cost",
    value: { ...summary, usage: { input_tokens: 0, output_tokens: 0, cost_credits: null } },
    check: isTraceSummary,
  },
  {
    name: "questions without choices",
    value: { ...summary, questions: [{ question: "Which column?", why: "Identify the key" }] },
    check: isTraceSummary,
  },
])("accepts a readable $name", ({ value, check }) => {
  expect(check(value, RUN)).toBe(true);
});

test.each([
  { name: "null summary", value: null, check: isTraceSummary },
  { name: "null events", value: null, check: isTraceAdvanced },
  { name: "array summary", value: [], check: isTraceSummary },
  {
    name: "foreign summary",
    value: { ...summary, run_id: "11111111-1111-4111-8111-111111111111" },
    check: isTraceSummary,
  },
  {
    name: "foreign events",
    value: { ...advanced, run_id: "11111111-1111-4111-8111-111111111111" },
    check: isTraceAdvanced,
  },
  {
    name: "unconfirmed completeness",
    value: { ...summary, complete: undefined },
    check: isTraceSummary,
  },
  {
    name: "unknown current status",
    value: { ...summary, status: "unknown" },
    check: isTraceSummary,
  },
  { name: "missing steps", value: { ...summary, steps: undefined }, check: isTraceSummary },
  { name: "unreadable step", value: { ...summary, steps: [null] }, check: isTraceSummary },
  { name: "unreadable skills", value: { ...summary, skills: null }, check: isTraceSummary },
  {
    name: "unreadable skill decision",
    value: { ...summary, skills: [{ name: "example", decision: "unknown" }] },
    check: isTraceSummary,
  },
  { name: "unreadable errors", value: { ...summary, errors: [null] }, check: isTraceSummary },
  {
    name: "missing tool observations",
    value: { ...summary, tool_calls: {} },
    check: isTraceSummary,
  },
  {
    name: "unreadable question choices",
    value: { ...summary, questions: [{ question: "Which?", why: "Pick", options: [null] }] },
    check: isTraceSummary,
  },
  {
    name: "unreadable cost",
    value: { ...summary, usage: { input_tokens: 0, output_tokens: 0, cost_credits: "0" } },
    check: isTraceSummary,
  },
  {
    name: "invalid last-event timestamp",
    value: { ...summary, last_event_at: "yesterday" },
    check: isTraceSummary,
  },
  {
    name: "unreadable final output",
    value: { ...summary, final_output: {} },
    check: isTraceSummary,
  },
  {
    name: "missing stream observations",
    value: { ...advanced, streams: undefined },
    check: isTraceAdvanced,
  },
  {
    name: "unreadable missing sequence",
    value: {
      ...advanced,
      streams: [
        {
          attempt: 1,
          emitted_by: "sandbox",
          received: 0,
          highest_seq: 0,
          missing_count: 1,
          late_events: 0,
          missing_seq: [null],
        },
      ],
    },
    check: isTraceAdvanced,
  },
  { name: "missing events", value: { ...advanced, events: undefined }, check: isTraceAdvanced },
  { name: "unreadable event", value: { ...advanced, events: [null] }, check: isTraceAdvanced },
  { name: "unknown next cursor", value: { ...advanced, next_after: "0" }, check: isTraceAdvanced },
  {
    name: "unknown page completion",
    value: { ...advanced, has_more: undefined },
    check: isTraceAdvanced,
  },
])("rejects a $name instead of treating it as evidence", ({ value, check }) => {
  expect(check(value, RUN)).toBe(false);
});
