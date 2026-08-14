import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Me } from "./types";

/** GET /me — resolves the current session (401 means not logged in). */
export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => apiFetch<Me>("/me"),
    retry: false,
  });
}
