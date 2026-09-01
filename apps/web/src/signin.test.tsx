import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SignInAction } from "./components/SignIn";

/**
 * 淨測試模式的登入。
 *
 * 缺陷的形狀值得留著：`POST /auth/dev/login` 一直都掛著、`DEV_LOGIN=1` 一直都由
 * `tools/cleanmode/start.mjs` 設，**而畫面上唯一的登入入口是一個 GitHub 連結**。
 * 在那台只到得了模型供應商的機器上（`02:PORT-005`），那個連結離開產品、落在瀏覽器
 * 的連線錯誤頁，於是每一件需要 session 的事都從畫面上到不了——**包含 `04` 丙-114
 * 記下的示範走法本身**（「以 `seed-importer` 的身分展示」）。能力一直都在，沒有人
 * 去呼叫它。
 */

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

// 沒有旗標就是一般部署：GitHub 是對的，而且必須維持是唯一的。
test("without the injected flag it offers GitHub and no offline form", async () => {
  await mount();
  expect(githubLink()).toBeTruthy();
  expect(offlineBox()).toBeNull();
});

// 有旗標就是那台機器：**GitHub 連結必須消失**，不是並排多一個選項。並排會讓操作者
// 在台上按到那個必然失敗的按鈕，而失敗的樣子是瀏覽器的錯誤頁，不是產品的。
test("with the flag it offers offline sign-in and drops the GitHub link", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  await mount();
  expect(offlineBox()).toBeTruthy();
  expect(githubLink()).toBeUndefined();
});

// 預設身分不是隨便挑的：目錄工作區就是 `seed-importer` 的工作區，所以只有它能直接
// 對策展 Skill 建 Test Case 並跑起來（丙-114 實測 queued → running → succeeded）。
test("it defaults to the identity that can actually run the curated catalogue", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  await mount();
  expect(offlineBox()?.value).toBe("seed-importer");
  // 而且那個輸入框要有標籤，否則它在螢幕閱讀器上是一個沒有名字的欄位（NFR-007 第 4 條）。
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
  // React 攔截了 value 的 setter，所以直接指派不會觸發 onChange（同 disc.test.tsx:74）。
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

// 失敗不得被吞掉：畫面上不說的話，操作者看到的是一個按了沒有反應的按鈕。
test("a refused sign-in says so instead of looking like nothing happened", async () => {
  window.__SKILLHUB_DEV_LOGIN__ = true;
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("nope", { status: 500 }));
  await mount();

  await act(async () => {
    container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true }));
  });

  expect(container.querySelector('[role="alert"]')?.textContent ?? "").toContain("登入失敗");
});
