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
    __SKILLHUB_DEV_LOGIN__?: true;
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
 * 這個部署有沒有掛上離線登入（`POST /auth/dev/login`，ADR-020）。
 *
 * **刻意不從 `useCleanMode()` 推導**，即使今天的啟動器兩個一起設：`clean_mode` 在
 * `public.yaml` 裡是一句**揭露**，而且那份契約逐字寫著把它當成「可以解鎖什麼」的
 * 客戶端是**讀反了**。這一個問的是完全不同的問題——**一條路由在不在**——所以它
 * 是 `generate_skill` 那個形狀的入口旗標，各自獨立。
 *
 * 沒有 `/me` 的後備，而這正是重點：需要它的人**還沒有 session**，`GET /me` 對他
 * 回 401，所以答案只可能來自 cmd/api 注入的那一行。注入端永遠不寫 `false`（同
 * `__SKILLHUB_CLEAN_MODE__` 的 ⛔ 邊界，理由更硬：畫一個按下去 404 的登入，比不
 * 畫還糟）。
 */
export function useDevLogin(): boolean {
  return typeof window !== "undefined" && window.__SKILLHUB_DEV_LOGIN__ === true;
}

/**
 * 以離線 provider 登入。名字任意；空字串由伺服器預設為 `dev`。
 *
 * 淨測試模式的示範身分是 `seed-importer`——**目錄工作區就是它的工作區**，所以只有
 * 它能直接對策展 Skill 建 Test Case 並跑起來（`04` 丙-114）。其他名字一樣登得進
 * 去，但自建內容在這個模式下仍然被派送閘門擋著（`02:PORT-010`），那是另一件事。
 */
export function devLogin(user: string) {
  return apiFetch<void>("/auth/dev/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user }),
  });
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
