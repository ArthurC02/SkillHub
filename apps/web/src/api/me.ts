import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import { queryKeys } from "./queryKeys";
import type { AccountDeletion, Me } from "./types";

export function useMe() {
  return useQuery({
    queryKey: queryKeys.me,
    queryFn: () => apiFetch<Me>("/me"),
  });
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
  return useMutation({ mutationFn: devLogin, onSuccess: () => client.clear() });
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
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.me }),
  });
}

export function useCancelAccountDeletion() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: cancelAccountDeletion,
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.me }),
  });
}

export function logout() {
  return apiFetch<void>("/auth/logout", { method: "POST" });
}

export function useSignOut() {
  const client = useQueryClient();
  return useMutation({ mutationFn: logout, onSuccess: () => client.clear() });
}
