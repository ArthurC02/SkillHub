import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

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
    queryKey: ["credits"],
    queryFn: getCredits,
    retry: false,
  });
}
