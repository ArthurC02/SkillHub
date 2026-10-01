import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { RunCompare } from "./compare/RunCompare.page";
import { OTHER_RUN, RUN, platformResponse } from "../../testing/fixtures/platform";

let container: HTMLDivElement;
let root: Root;
let search: { against?: string } = {};

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ runId: RUN }),
  useSearch: () => search,
  useNavigate: () => () => Promise.resolve(),
  Link: ({ children }: { children?: unknown }) => children as never,
}));

beforeEach(() => {
  queryClient.clear();
  search = {};
  container = document.createElement("div");
  document.body.appendChild(container);
  vi.stubGlobal("fetch", (input: string) => {
    const { body, status } = platformResponse(String(input));
    return Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

function draw() {
  root.render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <RunCompare />
      </QueryClientProvider>
    </StrictMode>,
  );
}

const field = () => container.querySelector<HTMLInputElement>("#against")!;

async function type(value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(field(), value);
    field().dispatchEvent(new Event("input", { bubbles: true }));
  });
}

test("the Run ID field starts from the run already named in the address", async () => {
  search = { against: OTHER_RUN };
  await act(async () => {
    root = createRoot(container);
    draw();
  });

  expect(field().value).toBe(OTHER_RUN);
});

test("a redraw with the same address keeps what the reader is typing", async () => {
  await act(async () => {
    root = createRoot(container);
    draw();
  });
  await type("half-typed");

  await act(async () => draw());

  expect(field().value).toBe("half-typed");
});

test("a new run in the address replaces what the reader had typed", async () => {
  await act(async () => {
    root = createRoot(container);
    draw();
  });
  await type("half-typed");

  search = { against: OTHER_RUN };
  await act(async () => draw());

  expect(field().value).toBe(OTHER_RUN);
});

test("a failed comparison gives a local next step instead of the server error", async () => {
  search = { against: OTHER_RUN };
  vi.stubGlobal("fetch", () =>
    Promise.resolve(
      new Response(JSON.stringify({ error: "database unavailable" }), {
        status: 422,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
  await act(async () => {
    root = createRoot(container);
    draw();
  });
  for (
    let attempts = 0;
    attempts < 200 && !container.textContent?.includes("請稍後再試一次");
    attempts += 1
  ) {
    await act(async () => new Promise((resolve) => setTimeout(resolve, 5)));
  }

  expect(container.textContent).toContain("請稍後再試一次");
  expect(container.textContent).not.toContain("database unavailable");
});
