import { useEffect, useRef } from "react";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../api/client";
import { queryKeys } from "../api/queryKeys";

export interface CreditEstimate {
  low_credits: number;
  high_credits: number;
  sample_size: number;
  estimated: boolean;
}

export interface CreditBalance {
  balance_credits: number;
  debt_floor_credits: number;
  estimated_session: CreditEstimate;
  can_start: boolean;
  block_reason?: string;
}

export const getCredits = () => apiFetch<CreditBalance>("/me/credits");

export function useCredits() {
  return useQuery({
    queryKey: queryKeys.credits,
    queryFn: getCredits,
  });
}

export function useCreditsRereadWhenSessionCloses(hasSession: boolean) {
  const client = useQueryClient();
  const hadSession = useRef(hasSession);
  useEffect(() => {
    if (hadSession.current && !hasSession)
      void client.invalidateQueries({ queryKey: queryKeys.credits });
    hadSession.current = hasSession;
  }, [client, hasSession]);
}

export interface CreditStatementEntry {
  id: string;
  kind: "debit" | "grant" | "topup" | "adjustment";
  label: string;
  delta_credits: number;
  estimated: boolean;
  created_at: string;
  run_id?: string;
}

export interface CreditStatement {
  entries: CreditStatementEntry[];
  next_before?: string;
  note: string;
}

export function useCreditStatement() {
  return useInfiniteQuery({
    queryKey: queryKeys.creditStatement,
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      apiFetch<CreditStatement>(
        pageParam
          ? `/me/credits/entries?before=${encodeURIComponent(pageParam)}`
          : "/me/credits/entries",
      ),
    getNextPageParam: (last) => last.next_before,
  });
}
