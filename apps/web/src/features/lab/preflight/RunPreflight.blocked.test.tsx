import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../../core/api/queryClient";
import { RunPreflight } from "./RunPreflight.page";
import { BLOCKED_SENTENCE } from "./preflight.model";
import { SKILL, RUN, TEST_CASE, platformResponse } from "../../../testing/fixtures/platform";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children?: unknown }) => (
    <a href={to}>{children as never}</a>
  ),
  useParams: () => ({ skillId: SKILL, runId: RUN, testCaseId: TEST_CASE }),
  useSearch: () => ({ skill: SKILL, version: "v1", test_case: TEST_CASE }),
  useNavigate: () => () => Promise.resolve(),
}));

function platformWithPreflight(extra: Record<string, unknown>) {
  vi.stubGlobal("fetch", (input: string) => {
    const { body, status } = platformResponse(String(input));
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    const payload =
      path.endsWith("/runs/preflight") && body && typeof body === "object"
        ? { ...body, ...extra }
        : body;
    return Promise.resolve(
      new Response(JSON.stringify(payload), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
}

async function renderPreflight() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <RunPreflight />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  const deadline = Date.now() + 2000;
  while (Date.now() < deadline) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (queryClient.isFetching() === 0 && (container.textContent ?? "").length > 0) break;
  }
  return container.textContent ?? "";
}

const startButton = () =>
  Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("開始 Run"),
  );

test("a pair this deployment cannot run offers no button to start it", async () => {
  platformWithPreflight({ blocked: "content_not_curated" });
  const shown = await renderPreflight();

  expect(startButton(), "the refusal is known before the click, so the click must not exist").toBe(
    undefined,
  );
  expect(shown).toContain(BLOCKED_SENTENCE.content_not_curated);
});

test("a pair this deployment can run still offers the button", async () => {
  platformWithPreflight({});
  await renderPreflight();

  expect(startButton(), "nothing was blocked, so the run must still be startable").not.toBe(
    undefined,
  );
});
