import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { DEFAULT_WAIT_MS, pollUntil } from "../testing/poll";
import { createAppRouter } from "./router";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.restoreAllMocks();
});

function BrokenPage(): never {
  throw new Error("internal route details");
}

test.each([
  ["page", false],
  ["root shell", true],
] as const)(
  "%s render failure offers a reload and catalog route without exposing internals",
  async (_name, onRoot) => {
    const rootRoute = createRootRoute({ component: onRoot ? BrokenPage : Outlet });
    const indexRoute = createRoute({
      getParentRoute: () => rootRoute,
      path: "/",
      component: onRoot ? () => null : BrokenPage,
    });
    const productionRouter = createAppRouter();
    const testRouter = createRouter({
      routeTree: rootRoute.addChildren([indexRoute]),
      history: createMemoryHistory({ initialEntries: ["/"] }),
      defaultErrorComponent: productionRouter.options.defaultErrorComponent,
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<RouterProvider router={testRouter} />);
    });
    await pollUntil(
      () => container.querySelector('[role="alert"]') !== null,
      () => container.textContent,
      DEFAULT_WAIT_MS,
    );

    const alert = container.querySelector('[role="alert"]')!;
    expect(alert.querySelector("h1")?.textContent).toBe("這一頁暫時無法顯示");
    expect(alert.textContent).toContain("請重新整理再試，或回到目錄繼續瀏覽。");
    expect(alert.textContent).not.toContain("internal route details");
    const links = alert.querySelectorAll("a");
    expect(links).toHaveLength(2);
    expect(links[0].href).toBe(window.location.href);
    expect(links[0].textContent).toBe("重新整理");
    expect(links[1].getAttribute("href")).toBe("/");
    expect(links[1].textContent).toBe("回目錄");
  },
);
