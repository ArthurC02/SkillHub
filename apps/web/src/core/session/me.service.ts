import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { ApiError, apiFetch } from "../api/client";
import { queryKeys } from "../api/queryKeys";
import type { AccountDeletion, Me } from "../api/types";

export function useMe() {
  return useQuery({
    queryKey: queryKeys.me,
    queryFn: () => apiFetch<Me>("/me"),
    refetchOnWindowFocus: true,
  });
}

export function useSessionBoundary() {
  const client = useQueryClient();
  const me = useMe();
  const previousIdentity = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    if (!me.isSuccess && !(me.error instanceof ApiError && me.error.status === 401)) return;
    const identity = me.isSuccess ? `${me.data.user_id}/${me.data.workspace_id}` : null;
    if (previousIdentity.current !== undefined && previousIdentity.current !== identity) {
      client.clear();
      window.location.reload();
    }
    previousIdentity.current = identity;
  }, [client, me.data, me.isSuccess, me.error]);
}

declare global {
  interface Window {
    __SKILLHUB_CLEAN_MODE__?: true;
    __SKILLHUB_DEV_LOGIN__?: true;
  }
}

export function useCleanMode(): boolean {
  const me = useMe();
  if (typeof window !== "undefined" && window.__SKILLHUB_CLEAN_MODE__ === true) {
    return true;
  }
  return me.data?.features?.clean_mode === true;
}

export function useDevLogin(): boolean {
  return typeof window !== "undefined" && window.__SKILLHUB_DEV_LOGIN__ === true;
}

export function devLogin(user: string) {
  return apiFetch<void>("/auth/dev/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user }),
  });
}

export function useDevSignIn() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: devLogin,
    onSuccess: () => {
      client.clear();
      window.location.reload();
    },
  });
}

export function requestAccountDeletion() {
  return apiFetch<AccountDeletion>("/me", { method: "DELETE" });
}

export function cancelAccountDeletion() {
  return apiFetch<{ deletion_requested_at: null }>("/me/deletion/cancel", { method: "POST" });
}

export function useRequestAccountDeletion() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: requestAccountDeletion,
    onSettled: () => client.invalidateQueries({ queryKey: queryKeys.me }),
  });
}

export function useCancelAccountDeletion() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: cancelAccountDeletion,
    onSettled: () => client.invalidateQueries({ queryKey: queryKeys.me }),
  });
}

export function logout() {
  return apiFetch<void>("/auth/logout", { method: "POST" });
}

export function useSignOut() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: logout,
    onError: async () => {
      try {
        await client.fetchQuery({
          queryKey: queryKeys.me,
          queryFn: () => apiFetch<Me>("/me"),
          staleTime: 0,
        });
      } catch (error) {
        if (error instanceof ApiError && error.status === 401) {
          client.clear();
          window.location.reload();
        }
      }
    },
    onSuccess: () => {
      client.clear();
      window.location.reload();
    },
  });
}
