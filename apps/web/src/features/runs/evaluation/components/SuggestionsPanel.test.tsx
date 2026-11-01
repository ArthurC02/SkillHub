import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../../../core/api/queryClient";
import { queryKeys } from "../../../../core/api/queryKeys";
import {
  ME,
  RUN,
  SKILL,
  SUGGESTIONS,
  platformResponse,
} from "../../../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../../../testing/poll";
import { SuggestionsPanel } from "./SuggestionsPanel";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    ...props
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
  }) => {
    const path = Object.entries(params ?? {}).reduce(
      (value, [key, replacement]) => value.replace(`$${key}`, replacement),
      to,
    );
    const query = new URLSearchParams(
      Object.entries(search ?? {}).filter((entry): entry is [string, string] => Boolean(entry[1])),
    );
    return <a {...props} href={`${path}${query.size ? `?${query}` : ""}`} />;
  },
}));

let container: HTMLDivElement;
let root: Root | undefined;
let originalScroll: typeof HTMLElement.prototype.scrollIntoView | undefined;
let scrolled: HTMLElement[];

beforeEach(() => {
  scrolled = [];
  originalScroll = HTMLElement.prototype.scrollIntoView;
  HTMLElement.prototype.scrollIntoView = function () {
    scrolled.push(this);
  };
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  queryClient.setQueryData(queryKeys.me, ME);
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  root = undefined;
  container.remove();
  queryClient.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  if (originalScroll) HTMLElement.prototype.scrollIntoView = originalScroll;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const NEW_VERSION = "99999999-1111-4111-8111-111111111111";
const receipt = {
  skill_id: SKILL,
  version_id: NEW_VERSION,
  version_number: 3,
  content_hash: "sha256:cccc",
  duplicate: false,
  applied_suggestion_ids: ["s1"],
  rejected_suggestions: [],
};

const waitFor = (ready: () => boolean) =>
  pollUntil(ready, () => container.textContent, DEFAULT_WAIT_MS);

for (const hasTestCase of [true, false]) {
  for (const leavesForm of [false, true]) {
    test(`an applied ${hasTestCase ? "rerun" : "version"} continuation ${leavesForm ? "preserves another control" : "reveals the completed attempt"}`, async () => {
      const requests: { body: unknown; resolve: (response: Response) => void }[] = [];
      vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
        const url = new URL(String(input), "http://localhost");
        if (url.pathname.endsWith("/versions/from-suggestions"))
          return new Promise<Response>((resolve) => {
            requests.push({ body: JSON.parse(String(init?.body)), resolve });
          });
        const result = platformResponse(url.toString());
        const body =
          url.pathname === `/runs/${RUN}`
            ? { ...(result.body as object), ...(!hasTestCase ? { test_case_id: undefined } : {}) }
            : result.body;
        return Promise.resolve(
          new Response(JSON.stringify(body), {
            status: result.status,
            headers: { "Content-Type": "application/json" },
          }),
        );
      });
      await act(async () => {
        root = createRoot(container);
        root.render(
          <StrictMode>
            <QueryClientProvider client={queryClient}>
              <input aria-label="另一個草稿" defaultValue="保留這段文字" />
              <SuggestionsPanel runId={RUN} evaluationId={SUGGESTIONS.evaluation_id} readable />
            </QueryClientProvider>
          </StrictMode>,
        );
      });
      const apply = () =>
        Array.from(container.querySelectorAll("button")).find((button) =>
          button.textContent?.includes("以已接受的 1 項建議建立新版本"),
        )!;
      await waitFor(() => Boolean(apply()) && !apply().disabled);
      const input = container.querySelector("input")!;
      const expectedPath = hasTestCase
        ? `/skills/${SKILL}/test-cases/33333333-3333-3333-3333-333333333333/runs/new?version=${NEW_VERSION}&against=${RUN}`
        : `/skills/${SKILL}/versions/${NEW_VERSION}`;

      await act(async () => {
        apply().focus();
        apply().click();
      });
      await waitFor(() => requests.length === 1);
      if (leavesForm) await act(async () => input.focus());
      await act(async () => {
        requests[0].resolve(
          new Response(JSON.stringify(receipt), {
            status: 201,
            headers: { "Content-Type": "application/json" },
          }),
        );
      });
      await waitFor(() => container.textContent!.includes("已建立新版本"));
      const continuation = container.querySelector<HTMLAnchorElement>(`a[href="${expectedPath}"]`)!;
      expect(continuation).not.toBeNull();
      expect(document.activeElement).toBe(leavesForm ? input : continuation);
      expect(scrolled).toHaveLength(leavesForm ? 0 : 1);
      if (!leavesForm) expect(scrolled.at(-1)).toBe(continuation);
      expect(input.value).toBe("保留這段文字");
      expect(requests.map((request) => request.body)).toEqual([
        { evaluation_id: "eval-2", suggestion_ids: ["s1"] },
      ]);
    });
  }
}

test("suggestions already applied to a version are not sent again and the build waits for new ones", async () => {
  const sent: unknown[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname.endsWith("/versions/from-suggestions"))
      sent.push(JSON.parse(String(init?.body)));
    const result = platformResponse(url.toString());
    const body =
      url.pathname === `/runs/${RUN}/suggestions`
        ? {
            ...SUGGESTIONS,
            suggestions: SUGGESTIONS.suggestions.map((s) => ({
              ...s,
              applied_skill_version_id: NEW_VERSION,
            })),
          }
        : result.body;
    return Promise.resolve(
      new Response(JSON.stringify(body), {
        status: result.status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <SuggestionsPanel runId={RUN} evaluationId={SUGGESTIONS.evaluation_id} readable />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() =>
    container.textContent!.includes("已接受的建議都已套用於新版本；要再建立版本，先接受其他建議。"),
  );
  const build = Array.from(container.querySelectorAll("button")).find((button) =>
    button.textContent?.includes("項建議建立新版本"),
  )!;
  expect(build.textContent).toContain("以已接受的 0 項建議建立新版本");
  expect(build.disabled).toBe(true);
  expect(build.getAttribute("aria-describedby")).toBe("suggestions-all-applied-note");
  expect(sent).toEqual([]);
});
