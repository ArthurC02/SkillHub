import { QueryClient } from "@tanstack/react-query";

/**
 * Defaults chosen for one reason: two GET handlers write funnel events.
 *
 * `search_performed` (skill/discovery/service.go) and `skill_detail_viewed`
 * (skill/discovery/detail.go) are written when the server answers the read, so
 * with React Query's defaults — `staleTime: 0` plus refetch on mount, on window
 * focus and on reconnect — the funnel counts alt-tabs. Measured shapes before
 * this: returning to a results tab wrote a second `search_performed` for the
 * same query in the same session; opening 打包與下載 from a skill wrote a second
 * `skill_detail_viewed` because Packaging calls useSkillDetail; and Compare's
 * useQueries over `["skills", id]` wrote one `skill_detail_viewed` per compared
 * skill, from a table where no detail page was opened (M4 audit, 2026-08-24 —
 * the same class as the M5 invalidate-key defect, but at the source).
 *
 * 01 §11.2's first funnel segment has one chance with twelve people, so the
 * default is the conservative one and a query that genuinely needs freshness
 * says so locally — usePackagingPreview already does exactly that with
 * `staleTime: 0, gcTime: 0`.
 *
 * `staleTime` is deliberately NOT raised. A global one would have made search
 * results stale for minutes after an import, which is a real cost paid for an
 * analytics problem; and it would not have fixed the funnel anyway, because the
 * aggregation counts distinct (session, day) pairs — a repeat of the same read
 * inside one session was never the thing inflating it. What these two flags
 * remove is the read the USER did not ask for: an alt-tab and a reconnect are
 * not a search.
 *
 * The duplicate that does inflate segment 1 is server-side and is not fixed
 * here: two concurrent cold requests each mint a different session id, so one
 * visitor becomes two sessions (04 丙-57).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    },
  },
});
