import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SignInAction } from "./components/SignIn";

let container: HTMLDivElement;
let root: Root | undefined;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  delete window.__SKILLHUB_DEV_LOGIN__;
  vi.restoreAllMocks();
});

async function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => {
    root = createRoot(container);
    root.render(
      <QueryClientProvider client={qc}>
        <SignInAction />
      </QueryClientProvider>,
    );
  });
}

const offlineBox = () => container.querySelector<HTMLInputElement>("#offline-user");
const githubLink = () =>
  [...container.querySelectorAll("a")].find((a) => a.textContent?.includes("GitHub"));

test("without the injected flag it offers GitHub and no offline form", async () => {
  await mount();
  expect(githubLink()).toBeTruthy();
  expect(offlineBox()).toBeNull();
});

test("with the flag it offers offline sign-in and drops the GitHub link", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  await mount();
  expect(offlineBox()).toBeTruthy();
  expect(githubLink()).toBeUndefined();
});

test("it defaults to the identity that can actually run the curated catalogue", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  await mount();
  expect(offlineBox()?.value).toBe("seed-importer");
  const label = container.querySelector<HTMLLabelElement>('label[for="offline-user"]');
  expect(label?.textContent ?? "").toContain("離線登入");
});

test("submitting posts the typed name to the offline endpoint", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(null, { status: 204 }));
  await mount();

  const box = offlineBox()!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(box, "someone-else");
    box.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true }));
  });

  expect(fetchMock).toHaveBeenCalled();
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
  expect(String(url)).toContain("/auth/dev/login");
  expect(init.method).toBe("POST");
  expect(String(init.body)).toContain("someone-else");
});

test("a refused sign-in says so instead of looking like nothing happened", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("nope", { status: 500 }));
  await mount();

  await act(async () => {
    container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true }));
  });

  expect(container.querySelector('[role="alert"]')?.textContent ?? "").toContain(
    "登入沒有成功，可以再試一次。",
  );
});

test("丙-150 a 64-char name refused by the server says the number, not the raw body", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify({ error: "使用者名稱最多 64 個字元" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    }),
  );
  await mount();

  const deadline = Date.now() + 2000;
  let alertText = "";
  while (Date.now() < deadline && !alertText) {
    await act(async () => {
      container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    alertText = container.querySelector('[role="alert"]')?.textContent ?? "";
  }

  expect(alertText).toContain("使用者名稱最多 64 個字元。");
});

test("丙-155⑥ the offline name field caps input at 64 characters", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  await mount();

  expect(offlineBox()?.maxLength).toBe(64);
});
