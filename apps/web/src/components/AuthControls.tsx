import { useMutation, useQueryClient } from "@tanstack/react-query";
import { API_BASE_URL, ApiError } from "../api/client";
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

  if (me.error instanceof ApiError && me.error.status === 401) {
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
