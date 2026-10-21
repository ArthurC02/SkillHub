import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { ApiError, apiFetch } from "../api/client";
import { queryKeys } from "../api/queryKeys";
import type { AccountDeletion, Me } from "../api/types";

const reloadingClients = new WeakSet<ReturnType<typeof useQueryClient>>();

function reloadSession(client: ReturnType<typeof useQueryClient>) {
  if (reloadingClients.has(client)) return;
  reloadingClients.add(client);
  client.clear();
  window.location.reload();
}

export async function getMe(): Promise<Me> {
  const result = await apiFetch<unknown>("/me");
  if (!isMe(result)) throw new Error("Invalid session response");
  return result;
}

function isMe(value: unknown): value is Me {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const profile = value as Partial<Me>;
  return (
    [profile.user_id, profile.workspace_id].every(
      (id) =>
        typeof id === "string" &&
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id),
    ) &&
    typeof profile.email === "string" &&
    typeof profile.display_name === "string" &&
    typeof profile.operator === "boolean" &&
    (profile.deletion_scope === null || typeof profile.deletion_scope === "string") &&
    [profile.deletion_requested_at, profile.purge_after].every(
      (at) => at === null || (typeof at === "string" && Number.isFinite(Date.parse(at))),
    ) &&
    (profile.features === undefined ||
      (profile.features !== null &&
        typeof profile.features === "object" &&
        !Array.isArray(profile.features) &&
        Object.values(profile.features).every((enabled) => typeof enabled === "boolean")))
  );
}

export function useMe() {
  return useQuery({
    queryKey: queryKeys.me,
    queryFn: getMe,
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
      reloadSession(client);
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
      reloadSession(client);
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
          queryFn: getMe,
          staleTime: 0,
        });
      } catch (error) {
        if (error instanceof ApiError && error.status === 401) {
          reloadSession(client);
        }
      }
    },
    onSuccess: () => {
      reloadSession(client);
    },
  });
}
