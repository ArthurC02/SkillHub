import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
import { LoginFailureNotice } from "./LoginFailureNotice";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  window.history.replaceState(null, "", "/");
});

async function arriveAt(address: string) {
  window.history.replaceState(null, "", address);
  root = createRoot(container);
  await act(async () =>
    root.render(
      <StrictMode>
        <LoginFailureNotice />
      </StrictMode>,
    ),
  );
}

const signInLink = () =>
  Array.from(container.querySelectorAll("a")).some((a) => a.textContent === "使用 GitHub 登入");

test.each([
  { code: "cancelled", say: "你在 GitHub 取消了授權，所以沒有登入。", offerSignIn: true },
  {
    code: "expired",
    say: "這次登入已經失效（等太久，或從另一個分頁完成），請重新登入一次。",
    offerSignIn: true,
  },
  { code: "github_unavailable", say: "GitHub 暫時沒有回應，請稍後再登入一次。", offerSignIn: true },
  { code: "github_refused", say: "GitHub 沒有完成這次登入，請重新登入一次。", offerSignIn: true },
  { code: "failed", say: "登入沒有完成，請再試一次。", offerSignIn: true },
  { code: "<script>", say: "登入沒有完成，請再試一次。", offerSignIn: true },
  { code: "toString", say: "登入沒有完成，請再試一次。", offerSignIn: true },
  {
    code: "email_taken",
    say: "這個 GitHub 帳號的 email 已經被另一個帳號使用，所以無法用它登入。",
    offerSignIn: false,
  },
  {
    code: "account_purging",
    say: "這個帳號正在永久刪除，刪除已經不可逆，無法再登入。",
    offerSignIn: false,
  },
  {
    code: "no_provider",
    say: "這個部署沒有設定登入提供者，現在無法登入，請聯絡管理者。",
    offerSignIn: false,
  },
])(
  "a sign-in that came back with $code says why and offers sign-in only when it can work",
  async ({ code, say, offerSignIn }) => {
    await arriveAt(`/workspace?tab=1&login_error=${encodeURIComponent(code)}`);

    expect(container.querySelector('[role="alert"] p')?.textContent).toBe(say);
    expect(signInLink()).toBe(offerSignIn);
    expect(window.location.pathname + window.location.search).toBe("/workspace?tab=1");
  },
);

test("an address without a sign-in failure shows nothing", async () => {
  await arriveAt("/workspace?tab=1");

  expect(container.innerHTML).toBe("");
  expect(window.location.search).toBe("?tab=1");
});

test("the notice can be put away", async () => {
  await arriveAt("/?login_error=cancelled");

  await act(async () =>
    Array.from(container.querySelectorAll("button"))
      .find((b) => b.textContent === "知道了")!
      .click(),
  );

  expect(container.innerHTML).toBe("");
});
