import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { ForkedSkill, PublicSearchResponse, SkillDetail, SkillFiles } from "./types";

/**
 * Every route below exists in contracts/openapi/public.yaml (implementation
 * rule 12). The `/api/*` three are the public Explorer surface and are mounted
 * without RequireSession, so an anonymous visitor can search and read detail
 * (DISC-001, DISC-010); scope is resolved server-side and no request parameter
 * can widen it (CORE-006, ADR-011).
 *
 * Once codegen is wired (ADR-019), replace this file's request plumbing with
 * the generated packages/api-client-ts client.
 */

export function searchSkills(query: string, limit = 20) {
  const params = new URLSearchParams({ q: query, limit: String(limit) });
  return apiFetch<PublicSearchResponse>(`/api/skills/search?${params.toString()}`);
}

/**
 * `enabled` is the "has the user submitted yet" gate, deliberately not
 * "is the query non-empty": a blank or incomprehensible query is answered by
 * the server with no_results plus the suggestion copy, and DISC-005 wants that
 * copy shown rather than a second, hardcoded version of it in the client.
 */
export function useSkillSearch(query: string, enabled: boolean) {
  return useQuery({
    queryKey: ["skills", "search", query],
    queryFn: () => searchSkills(query),
    enabled,
  });
}

export function getSkillDetail(skillId: string) {
  return apiFetch<SkillDetail>(`/api/skills/${skillId}`);
}

export function useSkillDetail(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId],
    queryFn: () => getSkillDetail(skillId),
    enabled: skillId.length > 0,
  });
}

export function getSkillFiles(skillId: string) {
  return apiFetch<SkillFiles>(`/api/skills/${skillId}/files`);
}

export function useSkillFiles(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId, "files"],
    queryFn: () => getSkillFiles(skillId),
    enabled: skillId.length > 0,
  });
}

export function forkSkill(skillId: string) {
  return apiFetch<ForkedSkill>(`/skills/${skillId}/fork`, { method: "POST" });
}

export function useForkSkill() {
  return useMutation({ mutationFn: forkSkill });
}
