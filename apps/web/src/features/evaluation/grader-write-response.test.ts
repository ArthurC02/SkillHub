import { afterEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { ME, PLATFORM_GRADER, WORKSPACE_GRADER } from "../../testing/fixtures/platform";
import { queryKeys } from "../../core/api/queryKeys";
import { createGrader, reviseGrader } from "./evaluation.service";

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

const draft = { name: "新評分器", instructions: "判斷是否附上出處", reason: "調整判準" };
const revision = { instructions: draft.instructions, reason: draft.reason };

function reply(body: unknown) {
  queryClient.setQueryData(queryKeys.me, ME);
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body), { status: 201 })),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

for (const scope of ["workspace", "platform"] as const) {
  const grader = scope === "workspace" ? WORKSPACE_GRADER : PLATFORM_GRADER;
  const path = scope === "workspace" ? "/evaluation/graders" : "/admin/evaluation/graders";
  for (const operation of ["create", "revise"] as const) {
    const valid = { ...grader, version: operation === "create" ? 1 : grader.version + 1 };
    const write = () =>
      operation === "create"
        ? createGrader(scope, draft)
        : reviseGrader(scope, grader.id, revision);
    test(`a confirmed ${scope} ${operation} returns its receipt after one POST`, async () => {
      const fetch = reply(valid);
      await expect(write()).resolves.toEqual(valid);
      expect(fetch).toHaveBeenCalledTimes(1);
      expect(fetch.mock.calls[0]?.[0]).toBe(
        path + (operation === "revise" ? `/${grader.id}/versions` : ""),
      );
      expect(fetch.mock.calls[0]?.[1]?.method).toBe("POST");
      expect(JSON.parse(fetch.mock.calls[0]?.[1]?.body as string)).toEqual(
        operation === "create" ? draft : revision,
      );
    });

    test.each<[string, unknown]>([
      ["null receipt", null],
      ["incomplete receipt", {}],
      ["unreadable instructions", { ...valid, instructions: null }],
      ["another scope", { ...valid, scope: scope === "workspace" ? "platform" : "workspace" }],
      [
        operation === "create" ? "later creation version" : "another grader",
        operation === "create"
          ? { ...valid, version: 2 }
          : { ...valid, id: "00000000-0000-4000-8000-000000000000" },
      ],
    ])(`an unconfirmed ${scope} ${operation} with %s rejects its receipt`, async (_label, body) => {
      reply(body);
      await expect(write()).rejects.toThrow("Invalid grader write response");
    });
  }

  test(`a ${scope} revision accepts UUID letter casing without changing the receipt`, async () => {
    const body = { ...grader, id: grader.id.toUpperCase(), version: grader.version + 1 };
    reply(body);
    await expect(reviseGrader(scope, grader.id, revision)).resolves.toEqual(body);
  });
}
