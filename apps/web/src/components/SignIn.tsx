import { useId, useState } from "react";
import { API_BASE_URL, ApiError } from "../api/client";
import { useDevLogin, useDevSignIn } from "../api/me";

function signInFailureSentence(error: unknown): string {
  if (error instanceof ApiError && error.status === 400) return "使用者名稱最多 64 個字元。";
  return "登入沒有成功，可以再試一次。";
}

export function SignInAction() {
  const offline = useDevLogin();
  if (!offline) {
    return <a href={`${API_BASE_URL}/auth/github/login`}>使用 GitHub 登入</a>;
  }
  return <OfflineSignIn />;
}

function OfflineSignIn() {
  const [user, setUser] = useState("seed-importer");
  const inputId = useId();
  const signIn = useDevSignIn();

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        signIn.mutate(user.trim());
      }}
    >
      <label htmlFor={inputId}>離線登入（這台機器沒有 GitHub 可以連）</label>{" "}
      <input
        id={inputId}
        value={user}
        onChange={(e) => setUser(e.target.value)}
        autoComplete="off"
        maxLength={64}
      />{" "}
      <button type="submit" disabled={signIn.isPending}>
        登入
      </button>
      {signIn.error && <span role="alert">{signInFailureSentence(signIn.error)}</span>}
    </form>
  );
}
