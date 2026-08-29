import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { AccountDeletion, Me } from "./types";

/** GET /me — resolves the current session (401 means not logged in). */
export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<Me>("/me"),
    retry: false,
  });
}

/**
 * Set by cmd/api's clean-mode static handler (apps/platform/cmd/api/main.go),
 * which rewrites a placeholder comment in apps/web/index.html into this
 * assignment — but only on the response it serves itself, with
 * SKILLHUB_CLEAN_MODE=1. Every other build, and every other way of serving
 * this app, leaves `window.__SKILLHUB_CLEAN_MODE__` undefined.
 *
 * This exists because `GET /me` requires a session (RequireSession) while `/`
 * and `/skills/$id` are reachable signed out — so a flag that only ever came
 * from `/me` never reached an anonymous visitor, which is the half of
 * PORT-003 this fills in.
 */
declare global {
  interface Window {
    __SKILLHUB_CLEAN_MODE__?: true;
  }
}

/**
 * Whether this deployment is running in 淨測試模式 (PORT-003), the same
 * flag-from-`/me` shape `useGenerateEntryPoint` in api/generate.ts uses for
 * ADR-052 and for the same reason: a build-time constant cannot serve one
 * cohort that sees the disclosure and one that does not, and `false` while
 * `me` has not resolved yet is the correct default — there is nothing to
 * disclose before the flag is known.
 *
 * The injected `window.__SKILLHUB_CLEAN_MODE__` is checked first so a
 * signed-out visitor sees the disclosure without waiting on a session; `/me`
 * stays the fallback so a signed-in visit is unaffected by how (or whether)
 * this page was served. The injected flag can only ever be `true` — it is
 * never written as `false` — so this can only turn the notice on, never off
 * an existing `/me` disclosure (⛔ boundary).
 */
export function useCleanMode(): boolean {
  const me = useMe();
  if (typeof window !== "undefined" && window.__SKILLHUB_CLEAN_MODE__ === true) {
    return true;
  }
  return me.data?.features?.clean_mode === true;
}

/**
 * DELETE /me — CORE-007. Starts a 30-day grace period; it deletes nothing yet,
 * which is why the account screen can show the request as a state to be followed
 * rather than as a farewell.
 *
 * Idempotent by contract: asking twice keeps the original start time instead of
 * extending the wait, so a double submit cannot cost the user 30 more days.
 */
export function requestAccountDeletion() {
  return apiFetch<AccountDeletion>("/me", { method: "DELETE" });
}

/** POST /me/deletion/cancel — valid for the whole grace period, idempotent. */
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
