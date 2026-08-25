import { useMutation, useQueryClient } from "@tanstack/react-query";
import { API_BASE_URL } from "../api/client";
import { unauthenticated } from "./LoginRequired";
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
    return <a href={`${API_BASE_URL}/auth/github/login`}>使用 GitHub 登入</a>;
  }
  if (!me.data) return null;

  return (
    <span>
      {me.data.display_name}{" "}
      <button type="button" disabled={signOut.isPending} onClick={() => signOut.mutate()}>
        登出
      </button>
      {signOut.error && <span role="alert">登出失敗：{signOut.error.message}</span>}
    </span>
  );
}
