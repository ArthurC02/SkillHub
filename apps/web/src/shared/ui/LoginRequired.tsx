import type { ReactNode } from "react";
import { SignInAction } from "./SignIn";
import { unauthenticated } from "./LoginRequired.model";

export function LoginRequired({ what }: { what: string }) {
  return (
    // div, not p: SignInAction can render a <form>, and HTML forbids a form inside a <p>.
    <div role="status">
      {what}需要登入。
      <SignInAction />
    </div>
  );
}

export function ReadFailure({
  error,
  what,
  children,
  retry,
  retrying = false,
  focusTarget,
}: {
  error: unknown;
  what: string;
  children?: ReactNode;
  retry?: () => void;
  retrying?: boolean;
  focusTarget?: string;
}) {
  if (!error) return null;
  if (unauthenticated(error)) return <LoginRequired what={what} />;
  if (children) return <>{children}</>;
  return (
    <>
      <p role="alert">暫時無法讀取{what}。請重新整理，或稍後再試。</p>
      {retry && (
        <button
          type="button"
          disabled={retrying}
          onClick={() => {
            retry();
            if (focusTarget) document.getElementById(focusTarget)?.focus();
          }}
        >
          {retrying ? "重新讀取中…" : "再試一次"}
        </button>
      )}
    </>
  );
}
