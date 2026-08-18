import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type {
  ForkedSkill,
  PublicSearchResponse,
  SearchFilters,
  SkillDeletion,
  SkillDetail,
  SkillFiles,
  SkillVersions,
} from "./types";

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

export function searchSkills(query: string, filters: SearchFilters = {}, limit = 20) {
  const params = new URLSearchParams({ q: query, limit: String(limit) });
  // An unset dimension is omitted rather than sent empty: the server reads a
  // present-but-empty value as "not filtered" too, but omitting it keeps the
  // shared URL free of parameters the user never chose.
  if (filters.script) params.set("script", filters.script);
  if (filters.validation) params.set("validation", filters.validation);
  if (filters.agent) params.set("agent", filters.agent);
  return apiFetch<PublicSearchResponse>(`/api/skills/search?${params.toString()}`);
}

/**
 * `enabled` is the "has the user submitted yet" gate, deliberately not
 * "is the query non-empty": a blank or incomprehensible query is answered by
 * the server with no_results plus the suggestion copy, and DISC-005 wants that
 * copy shown rather than a second, hardcoded version of it in the client.
 *
 * The filters are part of the cache key: a filtered page is a different answer
 * to the same question, and serving the previous filter's results while the new
 * ones load would show a list that contradicts the controls above it.
 */
export function useSkillSearch(query: string, filters: SearchFilters, enabled: boolean) {
  return useQuery({
    queryKey: [
      "skills",
      "search",
      query,
      filters.script ?? "",
      filters.validation ?? "",
      filters.agent ?? "",
    ],
    queryFn: () => searchSkills(query, filters),
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

/**
 * GET /skills/{id}/versions (WS-001) — the version history, newest first, for
 * the screens that have to pick one to run or to package.
 *
 * Session scoped, unlike the three `/api/*` reads above: a version list is
 * workspace data, and the server resolves the scope from the session.
 */
export function getSkillVersions(skillId: string) {
  return apiFetch<SkillVersions>(`/skills/${skillId}/versions`);
}

export function useSkillVersions(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId, "versions"],
    queryFn: () => getSkillVersions(skillId),
    enabled: skillId.length > 0,
  });
}

/**
 * DELETE /skills/{id} (WS-005). Soft delete: the skill leaves this workspace's
 * lists and search at once, its version snapshots stay frozen for the 30-day
 * grace period, and package objects other forks share are untouched.
 *
 * The response's `note` is the server's statement of that scope. It arrives
 * *after* the deletion, so the screen that offers this has to say the scope
 * itself before it runs (02:WS-002 第 3 條) — see ConfirmDelete.
 */
export function deleteSkill(skillId: string) {
  return apiFetch<SkillDeletion>(`/skills/${skillId}`, { method: "DELETE" });
}

export function forkSkill(skillId: string) {
  return apiFetch<ForkedSkill>(`/skills/${skillId}/fork`, { method: "POST" });
}

export function useForkSkill() {
  return useMutation({ mutationFn: forkSkill });
}
