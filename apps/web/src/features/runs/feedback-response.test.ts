import { afterEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { EVALUATION, ME, RUN } from "../../testing/fixtures/platform";
import { setEvaluationFeedback } from "./evaluation.service";

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

const comment = "最新回饋";
const feedback = { helpful: true, comment, submitted_at: "2026-10-16T08:00:00Z" };
const receipt = { ...EVALUATION, feedback };

function reply(body: unknown) {
  queryClient.setQueryData(queryKeys.me, ME);
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

test.each<[string, boolean, string, unknown]>([
  ["helpful", true, comment, receipt],
  ["unhelpful", false, comment, { ...receipt, feedback: { ...feedback, helpful: false } }],
  [
    "cleared comment",
    true,
    "",
    { ...receipt, feedback: { helpful: true, submitted_at: feedback.submitted_at } },
  ],
  ["explicit empty comment", true, "", { ...receipt, feedback: { ...feedback, comment: "" } }],
  [
    "whitespace comment",
    true,
    " \t\n ",
    { ...receipt, feedback: { ...feedback, comment: " \t\n " } },
  ],
  [
    "Unicode comment",
    true,
    "更清楚🙂",
    { ...receipt, feedback: { ...feedback, comment: "更清楚🙂" } },
  ],
  ["additional fields", true, comment, { ...receipt, future_field: true }],
])(
  "a confirmed feedback receipt preserves %s after one PUT",
  async (_label, helpful, sent, body) => {
    const fetch = reply(body);
    await expect(setEvaluationFeedback(RUN, helpful, sent)).resolves.toEqual(body);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0]?.[0]).toBe(`/runs/${RUN}/evaluation/feedback`);
    expect(fetch.mock.calls[0]?.[1]?.method).toBe("PUT");
    expect(JSON.parse(fetch.mock.calls[0]?.[1]?.body as string)).toEqual({
      helpful,
      comment: sent,
    });
  },
);

test.each<[string, unknown]>([
  ["null receipt", null],
  ["incomplete receipt", {}],
  ["another Run", { ...receipt, run_id: "00000000-0000-4000-8000-000000000000" }],
  ["missing feedback", { ...receipt, feedback: undefined }],
  ["null feedback", { ...receipt, feedback: null }],
  ["opposite choice", { ...receipt, feedback: { ...feedback, helpful: false } }],
  ["missing choice", { ...receipt, feedback: { ...feedback, helpful: undefined } }],
  ["string choice", { ...receipt, feedback: { ...feedback, helpful: "true" } }],
  ["different comment", { ...receipt, feedback: { ...feedback, comment: "舊回饋" } }],
  ["missing nonempty comment", { ...receipt, feedback: { ...feedback, comment: undefined } }],
  ["object comment", { ...receipt, feedback: { ...feedback, comment: {} } }],
  ["missing timestamp", { ...receipt, feedback: { ...feedback, submitted_at: undefined } }],
  [
    "unreadable timestamp",
    { ...receipt, feedback: { ...feedback, submitted_at: "not a timestamp" } },
  ],
])("an unconfirmed feedback receipt with %s is rejected", async (_label, body) => {
  reply(body);
  await expect(setEvaluationFeedback(RUN, true, comment)).rejects.toThrow(
    "Invalid evaluation feedback response",
  );
});
