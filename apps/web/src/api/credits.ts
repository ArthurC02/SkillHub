import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

/**
 * CRED-001 (ADR-068 — Credit is the platform's only unit of account). One
 * interactive-creation session's expected cost in credits, and whether the
 * account may start a new one (gate ①: 「開始前：餘額 < 門檻就不讓開新會
 * 話」).
 */
export interface CreditEstimate {
  low_credits: number;
  high_credits: number;
  sample_size: number;
  /** True when fewer than 20 cost samples exist and a conservative fallback
   * threshold was used instead of a measured p95. */
  estimated: boolean;
}

export interface CreditBalance {
  balance_credits: number;
  /** The most negative balance_credits may go (−50, ADR-068 決策 3). */
  debt_floor_credits: number;
  estimated_session: CreditEstimate;
  can_start: boolean;
  /** Empty when can_start is true; otherwise names the deficit. */
  block_reason?: string;
}

export const getCredits = () => apiFetch<CreditBalance>("/me/credits");

/**
 * GET /me/credits is not mounted on any deployment yet: CRED-001 has no
 * operation in contracts/openapi/public.yaml, and apps/platform's router.go
 * says why next to its `Credits` field (iron rule 12 — the contract comes
 * first). Every call here 404s until that lands.
 *
 * Callers must treat `data === undefined` (still loading, 404, or any other
 * failure) as "nothing to show" rather than a block — the same "absent, never
 * a ceiling nothing enforces" rule `PreflightResponse.quota` already follows
 * for RunQuota (see api/lab.ts). Once the route is mounted this starts
 * resolving with no further change on this side.
 */
export function useCredits() {
  return useQuery({
    queryKey: ["credits"],
    queryFn: getCredits,
    retry: false,
  });
}
