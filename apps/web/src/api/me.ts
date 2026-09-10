import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { AccountDeletion, Me } from "./types";

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<Me>("/me"),
    retry: false,
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

export function requestAccountDeletion() {
  return apiFetch<AccountDeletion>("/me", { method: "DELETE" });
}

export function cancelAccountDeletion() {
  return apiFetch<{ deletion_requested_at: null }>("/me/deletion/cancel", { method: "POST" });
}

export function useRequestAccountDeletion() {
  return useMutation({ mutationFn: requestAccountDeletion });
}

export function useCancelAccountDeletion() {
  return useMutation({ mutationFn: cancelAccountDeletion });
}

export function logout() {
  return apiFetch<void>("/auth/logout", { method: "POST" });
}
