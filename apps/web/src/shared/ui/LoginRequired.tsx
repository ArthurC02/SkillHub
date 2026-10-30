import { useEffect, useRef, type ReactNode } from "react";
import { SessionNotReadyError } from "../../core/api/client";
import { SignInAction } from "./SignIn";
import { rateLimitedSentence, unauthenticated } from "./LoginRequired.model";

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
  const blockedOperation = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (error instanceof SessionNotReadyError) blockedOperation.current?.focus();
  }, [error]);
  if (!error) return null;
  if (error instanceof SessionNotReadyError)
    return (
      <p ref={blockedOperation} tabIndex={-1} role="alert">
        {error.message}
      </p>
    );
  if (unauthenticated(error)) return <LoginRequired what={what} />;
  return (
    <>
      {children || (
        <p role="alert">
          {`暫時無法讀取${what}。`}
          {rateLimitedSentence(error) ?? "請再試一次，或稍後再試。"}
        </p>
      )}
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

export function PageReadFailure({
  heading,
  error,
  what,
  retry,
  retrying,
  children,
}: {
  heading: string;
  error: unknown;
  what: string;
  retry?: () => void;
  retrying?: boolean;
  children?: ReactNode;
}) {
  return (
    <section>
      <h1>{heading}</h1>
      <ReadFailure
        error={error}
        what={what}
        retry={retry}
        retrying={retrying}
        focusTarget="main-content"
      >
        {children}
      </ReadFailure>
    </section>
  );
}
