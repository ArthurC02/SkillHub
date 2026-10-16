import { afterEach, expect, test, vi } from "vitest";
import { PLATFORM_GRADER, WORKSPACE_GRADER } from "../../testing/fixtures/platform";
import { getGraders } from "./evaluation.service";

afterEach(() => vi.unstubAllGlobals());

test.each([
  ["empty list", { graders: [] }],
  ["both available scopes", { graders: [PLATFORM_GRADER, WORKSPACE_GRADER] }],
  ["first version", { graders: [{ ...WORKSPACE_GRADER, version: 1 }] }],
  [
    "largest safe version",
    { graders: [{ ...WORKSPACE_GRADER, version: Number.MAX_SAFE_INTEGER }] },
  ],
  [
    "empty strings allowed by the response contract",
    { graders: [{ ...WORKSPACE_GRADER, name: "", instructions: "" }] },
  ],
  [
    "additional fields",
    { graders: [{ ...WORKSPACE_GRADER, future_field: true }], future_field: true },
  ],
])("a readable %s is preserved", async (_label, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getGraders("workspace")).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/evaluation/graders");
  expect(fetch.mock.calls[0]?.[1]?.method ?? "GET").toBe("GET");
});

test("the platform list uses the operator endpoint", async () => {
  const body = { graders: [PLATFORM_GRADER] };
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getGraders("platform")).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/admin/evaluation/graders");
  expect(fetch.mock.calls[0]?.[1]?.method ?? "GET").toBe("GET");
});

test.each<[string, unknown]>([
  ["null body", null],
  ["array body", []],
  ["primitive body", true],
  ["missing collection", {}],
  ["null collection", { graders: null }],
  ["object collection", { graders: {} }],
  ["null item", { graders: [null] }],
  ["array item", { graders: [[]] }],
  ["primitive item", { graders: [true] }],
  ["missing id", { graders: [{ ...WORKSPACE_GRADER, id: undefined }] }],
  ["unverified id", { graders: [{ ...WORKSPACE_GRADER, id: "../other-grader" }] }],
  ["missing version id", { graders: [{ ...WORKSPACE_GRADER, version_id: undefined }] }],
  ["unverified version id", { graders: [{ ...WORKSPACE_GRADER, version_id: "unverified" }] }],
  ["missing name", { graders: [{ ...WORKSPACE_GRADER, name: undefined }] }],
  ["object name", { graders: [{ ...WORKSPACE_GRADER, name: {} }] }],
  ["missing scope", { graders: [{ ...WORKSPACE_GRADER, scope: undefined }] }],
  ["unknown scope", { graders: [{ ...WORKSPACE_GRADER, scope: "unknown" }] }],
  ["missing version", { graders: [{ ...WORKSPACE_GRADER, version: undefined }] }],
  ["string version", { graders: [{ ...WORKSPACE_GRADER, version: "1" }] }],
  ["zero version", { graders: [{ ...WORKSPACE_GRADER, version: 0 }] }],
  ["negative version", { graders: [{ ...WORKSPACE_GRADER, version: -1 }] }],
  ["fractional version", { graders: [{ ...WORKSPACE_GRADER, version: 1.5 }] }],
  ["unsafe version", { graders: [{ ...WORKSPACE_GRADER, version: Number.MAX_SAFE_INTEGER + 1 }] }],
  ["missing instructions", { graders: [{ ...WORKSPACE_GRADER, instructions: undefined }] }],
  ["null instructions", { graders: [{ ...WORKSPACE_GRADER, instructions: null }] }],
  ["missing revision time", { graders: [{ ...WORKSPACE_GRADER, revised_at: undefined }] }],
  ["non-string revision time", { graders: [{ ...WORKSPACE_GRADER, revised_at: 0 }] }],
  ["unreadable revision time", { graders: [{ ...WORKSPACE_GRADER, revised_at: "not-a-date" }] }],
])("a list with %s is a read failure", async (_label, body) => {
  vi.stubGlobal("fetch", () => Promise.resolve(new Response(JSON.stringify(body))));
  await expect(getGraders("workspace")).rejects.toThrow("Invalid grader list response");
});
