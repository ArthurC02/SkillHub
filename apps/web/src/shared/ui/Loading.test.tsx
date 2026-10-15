import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Loading } from "./Loading";

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test("loading keeps its status label and exposes a decorative three-line skeleton", async () => {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  await act(async () => root.render(<Loading what="Catalog" />));

  const status = container.querySelector<HTMLElement>('[role="status"]');
  expect(status?.textContent).toBe("載入Catalog中…");
  expect(status?.classList.contains("loading-state")).toBe(true);
  expect(status?.querySelector(".loading-skeleton")?.getAttribute("aria-hidden")).toBe("true");
  expect(status?.querySelectorAll(".loading-skeleton > span")).toHaveLength(3);

  await act(async () => root.unmount());
  container.remove();
});

test("a paused read names the connection wait without animating work", async () => {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  try {
    await act(async () => root.render(<Loading what="目錄" paused />));
    const status = container.querySelector<HTMLElement>('[role="status"]')!;
    expect(status.textContent).toBe("等待連線以讀取目錄；恢復連線後會繼續讀取。");
    expect(status.hasAttribute("data-loading")).toBe(true);
    expect(status.querySelector(".loading-skeleton")).toBeNull();
  } finally {
    await act(async () => root.unmount());
    container.remove();
  }
});
