import { Link } from "@tanstack/react-router";
import { unauthenticated } from "../../shared/ui/LoginRequired.model";
import { SignInAction } from "../../shared/ui/SignIn";
import { ReadFailure } from "../../shared/ui/LoginRequired";
import { useMe, useSignOut } from "../../core/session/me.service";
import "./AuthControls.css";

export function AuthControls() {
  const me = useMe();
  const signOut = useSignOut();

  if (unauthenticated(me.error)) {
    return (
      <div className="auth-controls" data-auth-controls>
        <SignInAction />
      </div>
    );
  }
  if (!me.data)
    return (
      <div className="auth-controls" data-auth-controls>
        <SessionStatus me={me} logoutUnconfirmed={signOut.isError} />
      </div>
    );

  return (
    <div className="auth-controls" data-auth-controls>
      {me.isSuccess && !me.isFetching && <span className="auth-name">{me.data.display_name}</span>}
      <SessionStatus me={me} logoutUnconfirmed={signOut.isError} />
      {me.isSuccess && !me.isFetching && me.data.operator && <Link to="/admin">後台</Link>}
      <button type="button" disabled={signOut.isPending} onClick={() => signOut.mutate()}>
        登出
      </button>
    </div>
  );
}

function SessionStatus({
  me,
  logoutUnconfirmed,
}: {
  me: ReturnType<typeof useMe>;
  logoutUnconfirmed: boolean;
}) {
  if (me.isFetching)
    return (
      <span className="auth-session-message" role="status">
        正在確認登入狀態…
      </span>
    );
  const message = logoutUnconfirmed
    ? "未能確認登出結果，請確認目前登入狀態後再試一次。"
    : me.error
      ? "登入狀態暫時無法確認。"
      : undefined;
  if (!message || unauthenticated(me.error)) return null;
  if (!me.error)
    return (
      <span className="auth-session-message" role="alert">
        {message}
      </span>
    );
  return (
    <ReadFailure error={me.error} what="登入狀態">
      <span className="auth-session-message" role="alert">
        {message}
      </span>
      <button
        type="button"
        aria-label="重新確認登入狀態"
        onClick={() => {
          void me.refetch();
          document.getElementById("main-content")?.focus();
        }}
      >
        重新確認
      </button>
    </ReadFailure>
  );
}
