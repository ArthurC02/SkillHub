import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import { queryKeys } from "./queryKeys";
import type { DataRetentionPolicy } from "./types";

export function useDataRetentionPolicy() {
  return useQuery({
    queryKey: queryKeys.dataRetentionPolicy,
    queryFn: () => apiFetch<DataRetentionPolicy>("/policy/data-retention"),
  });
}
