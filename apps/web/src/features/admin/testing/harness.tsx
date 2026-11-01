import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeAll, beforeEach, expect, vi } from "vitest";
import App from "../../../app/App";
import { queryClient } from "../../../core/api/queryClient";
import { createAppRouter } from "../../../app/router";
import { platformResponse } from "../../../testing/fixtures/platform";
import { DEFAULT_WAIT_MS, pollUntil } from "../../../testing/poll";
import { preloadEveryPage } from "../../../testing/pages";

beforeAll(preloadEveryPage);

export type Call = { method: string; url: string; body?: Record<string, unknown> };
export type Reply = { body: unknown; status: number } | undefined;

export let container: HTMLDivElement;
export let root: Root;
export let calls: Call[];
export let router: ReturnType<typeof createAppRouter>;

beforeEach(() => {
  queryClient.clear();
  window.history.pushState({}, "", "/");
  router = createAppRouter();
  container = document.createElement("div");
  document.body.appendChild(container);
  calls = [];
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
  delete window.__SKILLHUB_CLEAN_MODE__;
});

export function stub(
  operator: boolean,
  override: (path: string, method: string, url: string) => Reply = () => undefined,
) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const { body, status } = override(path, method, url) ?? platformResponse(url);
    const payload = path === "/me" ? { ...(body as object), operator } : body;
    return Promise.resolve(
      new Response(status === 204 ? null : JSON.stringify(payload), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
}

export async function mountAt(to: string, search?: Record<string, string>) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App router={router} />
      </StrictMode>,
    );
  });
  await go(to, search);
}

export async function go(to: string, search?: Record<string, string>) {
  await act(async () => {
    await router.navigate({ to: to as "/", search: (search ?? {}) as never });
  });
}

export function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

export const has = (needle: string) => () => (container.textContent ?? "").includes(needle);

export function field<T extends HTMLElement>(selector: string): T {
  const el = container.querySelector<T>(selector);
  expect(el, `no element matches ${selector}`).not.toBeNull();
  return el!;
}

export async function type(selector: string, value: string) {
  const el = field<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(selector);
  const proto = Object.getPrototypeOf(el) as object;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(el, value);
    el.dispatchEvent(
      new Event(el instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }),
    );
  });
}

export function button(text: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll("button")).find(
    (b) => (b.textContent ?? "").trim() === text,
  );
  expect(found, `no button reads 「${text}」`).toBeDefined();
  return found!;
}

export async function click(el: HTMLElement) {
  await act(async () => el.click());
}

export async function submit(selector: string) {
  const form = field<HTMLElement>(selector).closest("form")!;
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

export async function render(node: ReactNode) {
  root = createRoot(container);
  await act(async () => root.render(node));
}
