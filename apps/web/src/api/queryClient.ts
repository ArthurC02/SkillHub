import { QueryClient } from "@tanstack/react-query";

/**
 * Two defaults, and a smaller claim than this file used to make.
 *
 * `search_performed` (skill/discovery/service.go) and `skill_detail_viewed`
 * (skill/discovery/detail.go) are written when the server answers the read, so
 * React Query's defaults - refetch on mount, on window focus and on reconnect -
 * make the funnel count things the user did not do.
 *
 * What these two flags remove is exactly one of those: the read nobody asked
 * for. An alt-tab and a reconnect are not a search.
 *
 * What they do NOT remove, and what this comment used to imply they did: the two
 * mount-time duplicates. 打包與下載 and 並排比較 both read
 * GET /api/skills/{id}, and `refetchOnMount` is untouched - deliberately,
 * because a stale detail page after an import is a real cost to pay for an
 * analytics problem. Those two are fixed at the source instead: both surfaces
 * read with `?view=embedded`, the server records nothing for such a read
 * (public.yaml), and the assertions live in disc.test.tsx and
 * beta_integration_test.go rather than here.
 *
 * `staleTime` is deliberately NOT raised, for the same reason, and because it
 * would not have helped anyway: the aggregation counts distinct (session, day)
 * pairs, so a repeat inside one session was never what inflated it.
 *
 * The duplicate that did inflate segment 1 was server-side and is fixed: two
 * concurrent cold requests each minted a session id, so one visitor became two
 * sessions (04 丙-57).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    },
  },
});
