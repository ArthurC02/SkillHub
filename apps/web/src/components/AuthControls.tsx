import { useMutation, useQueryClient } from "@tanstack/react-query";
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

  // The predicate now lives in LoginRequired, because every page needs the same
  // question answered; this was the only place that asked it (資訊架構 IA-6).
  if (unauthenticated(me.error)) {
    return <SignInAction />;
  }
  if (!me.data) return null;

  return (
    <span>
      {me.data.display_name}{" "}
      <button type="button" disabled={signOut.isPending} onClick={() => signOut.mutate()}>
        登出
      </button>
      {/* 04 丙-150／149: this was `signOut.error.message`, the Go server's raw
          English body. */}
      {signOut.error && <span role="alert">登出沒有完成，可以再試一次。</span>}
    </span>
  );
}
