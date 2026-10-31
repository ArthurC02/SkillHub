import { useEffect, useState } from "react";
import { SignInAction } from "../../shared/ui/SignIn";

const LOGIN_FAILURE_PARAM = "login_error";

const LOGIN_FAILURES: Record<string, { say: string; offerSignIn: boolean }> = {
  no_provider: {
    say: "這個部署沒有設定登入提供者，現在無法登入，請聯絡管理者。",
    offerSignIn: false,
  },
  expired: {
    say: "這次登入已經失效（等太久，或從另一個分頁完成），請重新登入一次。",
    offerSignIn: true,
  },
  cancelled: { say: "你在 GitHub 取消了授權，所以沒有登入。", offerSignIn: true },
  github_unavailable: { say: "GitHub 暫時沒有回應，請稍後再登入一次。", offerSignIn: true },
  github_refused: { say: "GitHub 沒有完成這次登入，請重新登入一次。", offerSignIn: true },
  account_purging: {
    say: "這個帳號正在永久刪除，刪除已經不可逆，無法再登入。",
    offerSignIn: false,
  },
  email_taken: {
    say: "這個 GitHub 帳號的 email 已經被另一個帳號使用，所以無法用它登入。",
    offerSignIn: false,
  },
};

const LOGIN_FAILED = { say: "登入沒有完成，請再試一次。", offerSignIn: true };

function readLoginFailure() {
  const code = new URL(window.location.href).searchParams.get(LOGIN_FAILURE_PARAM);
  if (code === null) return undefined;
  return Object.hasOwn(LOGIN_FAILURES, code) ? LOGIN_FAILURES[code] : LOGIN_FAILED;
}

function forgetLoginFailure() {
  const url = new URL(window.location.href);
  if (!url.searchParams.has(LOGIN_FAILURE_PARAM)) return;
  url.searchParams.delete(LOGIN_FAILURE_PARAM);
  window.history.replaceState(window.history.state, "", url);
}

export function LoginFailureNotice() {
  const [failure, setFailure] = useState(readLoginFailure);
  useEffect(forgetLoginFailure, []);
  if (!failure) return null;
  return (
    <div className="notice notice-danger" role="alert">
      <p>{failure.say}</p>
      {failure.offerSignIn && (
        <p>
          <SignInAction />
        </p>
      )}
      <button type="button" onClick={() => setFailure(undefined)}>
        知道了
      </button>
    </div>
  );
}
