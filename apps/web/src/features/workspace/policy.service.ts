import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import type { DataRetentionPolicy } from "../../core/api/types";

export function useDataRetentionPolicy() {
  return useQuery({
    queryKey: queryKeys.dataRetentionPolicy,
    queryFn: () => apiFetch<DataRetentionPolicy>("/policy/data-retention"),
  });
}
