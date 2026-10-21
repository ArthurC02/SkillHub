import { act } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, test, vi } from "vitest";
import { RUN, TRACE_GENERAL } from "../../testing/fixtures/platform";
import { useTrace } from "./trace.service";

function TraceReader() {
  const trace = useTrace(RUN, "general");
  return (
    <button type="button" onClick={() => void trace.refetch()}>
      {trace.error ? "unreadable" : trace.data ? "loaded" : "pending"}
    </button>
  );
}

test("an unreadable Trace waits for explicit retry without interrupting the recovery control", async () => {
  vi.useFakeTimers();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  let healthy = false;
  const fetch = vi.fn(() =>
    Promise.resolve(new Response(JSON.stringify(healthy ? TRACE_GENERAL : null))),
  );
  vi.stubGlobal("fetch", fetch);
  try {
    await act(async () => {
      root.render(
        <QueryClientProvider client={client}>
          <TraceReader />
        </QueryClientProvider>,
      );
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(container.textContent).toBe("unreadable");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(container.textContent).toBe("unreadable");
    healthy = true;
    await act(async () => {
      container.querySelector("button")!.click();
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(container.textContent).toBe("loaded");
  } finally {
    await act(async () => root.unmount());
    client.clear();
    container.remove();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  }
});
