import type { ReactNode } from "react";
import { ApiError } from "../api/client";
import { SignInAction } from "./SignIn";

export function unauthenticated(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401;
}

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
}: {
  error: unknown;
  what: string;
  children?: ReactNode;
}) {
  if (!error) return null;
  if (unauthenticated(error)) return <LoginRequired what={what} />;
  if (children) return <>{children}</>;
  return (
    <p role="alert">
      無法讀取{what}：{error instanceof Error ? error.message : String(error)}
    </p>
  );
}
