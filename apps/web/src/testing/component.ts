import { act, createElement, StrictMode, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach } from "vitest";

export function mountedComponent() {
  let host: HTMLDivElement;
  let root: Root | undefined;
  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
  });
  afterEach(async () => {
    await act(async () => root?.unmount());
    host.remove();
  });
  return {
    container: () => host,
    text: () => host.textContent ?? "",
    async mount(node: ReactElement) {
      const created = createRoot(host);
      root = created;
      await act(async () => {
        created.render(createElement(StrictMode, null, node));
      });
    },
  };
}
