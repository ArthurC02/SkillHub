import { afterEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { ME, SUGGESTIONS } from "../../testing/fixtures/platform";
import { decideSuggestion } from "./evaluation.service";

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

function reply(body: unknown) {
  queryClient.setQueryData(queryKeys.me, ME);
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

test.each(["accepted", "rejected"] as const)(
  "a matching %s decision confirms exactly one PUT",
  async (decision) => {
    const body = { ...SUGGESTIONS.suggestions[0], suggestion_id: "S1", decision };
    const fetch = reply(body);
    await expect(decideSuggestion("s1", decision)).resolves.toEqual(body);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0]?.[0]).toBe("/suggestions/s1/decision");
    expect(fetch.mock.calls[0]?.[1]?.method).toBe("PUT");
    expect(JSON.parse(fetch.mock.calls[0]?.[1]?.body as string)).toEqual({ decision });
  },
);

test.each<[string, unknown]>([
  ["null", null],
  ["incomplete", {}],
  ["an array", []],
  ["a string", "accepted"],
  ["a missing identifier", { decision: "accepted" }],
  ["another suggestion", { suggestion_id: "s2", decision: "accepted" }],
  ["an object identifier", { suggestion_id: {}, decision: "accepted" }],
  ["a missing choice", { suggestion_id: "s1" }],
  ["an undecided choice", { suggestion_id: "s1", decision: "pending" }],
  ["the opposite choice", { suggestion_id: "s1", decision: "rejected" }],
  ["an object choice", { suggestion_id: "s1", decision: {} }],
])("a decision receipt with %s cannot acknowledge the requested choice", async (_label, body) => {
  reply(body);
  await expect(decideSuggestion("s1", "accepted")).rejects.toThrow("Invalid decision response");
});
