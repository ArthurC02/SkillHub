import { Link } from "@tanstack/react-router";
import { unauthenticated } from "../../shared/ui/LoginRequired.model";
import { SignInAction } from "../../shared/ui/SignIn";
import { useMe, useSignOut } from "../../core/session/me.service";
import "./AuthControls.css";

export function AuthControls() {
  const me = useMe();
  const signOut = useSignOut();

  if (unauthenticated(me.error)) {
    return (
      <div data-auth-controls>
        <SignInAction />
      </div>
    );
  }
  if (!me.data) return null;

  return (
    <div data-auth-controls>
      <span className="auth-name">{me.data.display_name}</span>
      {me.data.operator && <Link to="/admin">後台</Link>}
      <button type="button" disabled={signOut.isPending} onClick={() => signOut.mutate()}>
        登出
      </button>
      {signOut.error && <span role="alert">未能確認登出結果，請確認目前登入狀態後再試一次。</span>}
    </div>
  );
}
