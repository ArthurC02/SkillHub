import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { usePaginationFocus } from "./usePaginationFocus";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

test("an empty completed pagination request returns focus to its heading after the response", async () => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  function EmptyPages() {
    const [state, setState] = useState<{ fetchStatus: "idle" | "fetching"; pages: unknown[] }>({
      fetchStatus: "idle",
      pages: [[]],
    });
    const { loadMore, bindTarget, headingRef } = usePaginationFocus(
      {
        data: { pages: state.pages },
        fetchStatus: state.fetchStatus,
        error: undefined,
        hasNextPage: state.pages.length === 1,
        fetchNextPage: async () => {
          setState((previous) => ({ ...previous, fetchStatus: "fetching" }));
          await gate;
          setState({ fetchStatus: "idle", pages: [[], []] });
        },
      },
      [],
    );
    return (
      <section>
        <h1 tabIndex={-1} ref={headingRef}>
          清單
        </h1>
        {state.pages.length === 1 && (
          <button ref={bindTarget} onClick={loadMore}>
            載入更多
          </button>
        )}
      </section>
    );
  }
  try {
    await act(async () => root.render(<EmptyPages />));
    const heading = container.querySelector("h1")!;
    const trigger = container.querySelector("button")!;
    trigger.focus();
    await act(async () => trigger.click());
    expect(document.activeElement).toBe(trigger);
    await act(async () => release());
    expect(container.querySelector("button")).toBeNull();
    expect(document.activeElement).toBe(heading);
  } finally {
    release();
  }
});
