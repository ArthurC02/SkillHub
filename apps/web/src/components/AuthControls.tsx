import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { unauthenticated } from "./LoginRequired";
import { SignInAction } from "./SignIn";
import { logout, useMe } from "../api/me";

export function AuthControls() {
  const me = useMe();
  const queryClient = useQueryClient();
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      queryClient.clear();
    },
  });

  if (unauthenticated(me.error)) {
    return <SignInAction />;
  }
  if (!me.data) return null;

  return (
    <span>
      {me.data.display_name}{" "}
      {me.data.operator && (
        <>
          <Link to="/admin">後台</Link>{" "}
        </>
      )}
      <button type="button" disabled={signOut.isPending} onClick={() => signOut.mutate()}>
        登出
      </button>
      {signOut.error && <span role="alert">登出沒有完成，可以再試一次。</span>}
    </span>
  );
}
