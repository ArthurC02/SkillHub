import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { SessionNotReadyError } from "../../core/api/client";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import { useForkSkill, useOwnSkills } from "./skills.service";

const SOURCE = "11111111-1111-1111-1111-111111111111";
const RECEIPT = {
  skill_id: "abcdef01-2345-4789-abcd-0123456789ab",
  version_id: "abcdef02-2345-4789-abcd-0123456789ab",
  version_number: 1,
  name: "fork",
  summary: "",
  redistribution: "allowed",
};
let root: Root;
let container: HTMLDivElement;
const forks: ReturnType<typeof useForkSkill>[] = [];

beforeEach(() => {
  forks.length = 0;
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  container = document.createElement("div");
  document.body.append(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  queryClient.clear();
  vi.unstubAllGlobals();
});

function Harness({ observe = false }: { observe?: boolean }) {
  forks.push(useForkSkill());
  return observe ? <OwnSkillsObserver /> : null;
}

function OwnSkillsObserver() {
  useOwnSkills();
  return null;
}

async function render(observe = false) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <QueryClientProvider client={queryClient}>
        <Harness observe={observe} />
      </QueryClientProvider>,
    );
  });
}

test("a confirmed fork is acknowledged while its active Library reread is still pending", async () => {
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      if (String(input).endsWith("/fork"))
        return new Response(JSON.stringify(RECEIPT), { status: 201 });
      reads += 1;
      if (reads > 1) await gate;
      return new Response(JSON.stringify({ skills: [], limit: 100, total: 0, truncated: false }));
    }),
  );
  try {
    await render(true);
    await pollUntil(
      () => queryClient.getQueryState(queryKeys.skills.own)?.status === "success",
      () => `reads ${reads}`,
      DEFAULT_WAIT_MS,
    );
    await act(async () => forks.at(-1)!.mutate(SOURCE));
    await pollUntil(
      () => reads === 2,
      () => `reads ${reads}`,
      DEFAULT_WAIT_MS,
    );
    await act(async () => Promise.resolve());
    expect(queryClient.getMutationCache().getAll()[0]!.state.status).toBe("success");
    expect(queryClient.getMutationCache().getAll()[0]!.state.data).toEqual(RECEIPT);
    expect(queryClient.getQueryState(queryKeys.skills.own)?.fetchStatus).toBe("fetching");
  } finally {
    release();
  }
});

for (const [outcome, invalidated] of [
  ["unavailable", true],
  ["lost response", true],
  ["malformed receipt", true],
  ["refused", false],
] as const) {
  test(`a ${outcome} fork ${invalidated ? "invalidates" : "preserves"} the Library cache without touching search`, async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        if (outcome === "lost response") throw new TypeError("Failed to fetch");
        return new Response(
          JSON.stringify(outcome === "malformed receipt" ? {} : { error: "unavailable" }),
          { status: outcome === "unavailable" ? 503 : outcome === "refused" ? 403 : 201 },
        );
      }),
    );
    queryClient.setQueryData(queryKeys.skills.own, { skills: [] });
    queryClient.setQueryData(queryKeys.skills.ownPages(24), { pages: [] });
    queryClient.setQueryData(queryKeys.skills.search("pdf", {}), { results: [] });
    await render();
    await act(async () => {
      await expect(forks.at(-1)!.mutateAsync(SOURCE)).rejects.toBeInstanceOf(Error);
    });
    expect(queryClient.getQueryState(["own-skills"])!.isInvalidated).toBe(invalidated);
    expect(queryClient.getQueryState(["own-skills", "pages", 24])!.isInvalidated).toBe(invalidated);
    expect(
      queryClient.getQueryState(["skills", "search", "pdf", "", "", "", "", "", ""])!.isInvalidated,
    ).toBe(false);
  });
}

test("an unsent fork preserves the Library cache and sends no HTTP request", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  queryClient.setQueryData(queryKeys.skills.own, { skills: [] });
  queryClient
    .getQueryCache()
    .build(queryClient, { queryKey: queryKeys.me })
    .setState({ fetchStatus: "paused" });
  await render();
  await act(async () => {
    await expect(forks.at(-1)!.mutateAsync(SOURCE)).rejects.toBeInstanceOf(SessionNotReadyError);
  });
  expect(fetch).not.toHaveBeenCalled();
  expect(queryClient.getQueryState(["own-skills"])!.isInvalidated).toBe(false);
});
