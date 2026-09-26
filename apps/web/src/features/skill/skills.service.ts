import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import type { CorrectedSearchRequest, SetSkillCategoryRequest } from "@skillhub/api-client-ts";
import { apiFetch } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import { readSearchCorrection } from "../../core/api/searchIntent";
import type {
  CatalogResponse,
  ForkedSkill,
  OwnSkills,
  PublicSearchResponse,
  SearchFilters,
  SkillDeletion,
  SkillDetail,
  SkillFiles,
  SkillVersions,
} from "../../core/api/types";

export const MAX_COMPARE = 3;

export function searchSkills(
  query: string,
  filters: SearchFilters = {},
  limit = 20,
  purpose?: "reference",
) {
  const params = new URLSearchParams({ q: query, limit: String(limit) });
  if (filters.script) params.set("script", filters.script);
  if (filters.validation) params.set("validation", filters.validation);
  if (filters.agent) params.set("agent", filters.agent);
  if (filters.tier) params.set("tier", filters.tier);
  if (filters.category) params.set("category", filters.category);
  if (purpose) params.set("purpose", purpose);
  return apiFetch<PublicSearchResponse>(`/api/skills/search?${params.toString()}`);
}

export function useSkillSearch(
  query: string,
  filters: SearchFilters,
  enabled: boolean,
  purpose?: "reference",
  correction?: string,
) {
  return useQuery({
    queryKey:
      correction === undefined
        ? queryKeys.skills.search(query, filters, purpose)
        : queryKeys.skills.correctedSearch(query, filters, correction),
    queryFn: () =>
      correction === undefined
        ? searchSkills(query, filters, 20, purpose)
        : apiFetch<PublicSearchResponse>("/api/skills/search", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              query,
              ...readSearchCorrection(correction),
              filters,
              limit: 20,
            } satisfies CorrectedSearchRequest),
          }),
    enabled,
  });
}

export function browseCatalog(filters: SearchFilters = {}, limit = 100) {
  const params = new URLSearchParams({ limit: String(limit) });
  if (filters.script) params.set("script", filters.script);
  if (filters.validation) params.set("validation", filters.validation);
  if (filters.agent) params.set("agent", filters.agent);
  if (filters.tier) params.set("tier", filters.tier);
  if (filters.category) params.set("category", filters.category);
  return apiFetch<CatalogResponse>(`/api/skills/catalog?${params.toString()}`);
}

export function useCatalog(filters: SearchFilters, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.skills.catalog(filters),
    queryFn: () => browseCatalog(filters),
    enabled,
  });
}

export function useCatalogTotal(filters: SearchFilters) {
  return useQuery({
    queryKey: queryKeys.skills.catalogTotal(filters),
    queryFn: () => browseCatalog(filters, 1),
  });
}

export function getSkillDetail(skillId: string) {
  return apiFetch<SkillDetail>(`/api/skills/${skillId}`);
}

export function getEmbeddedSkillDetail(skillId: string) {
  return apiFetch<SkillDetail>(`/api/skills/${skillId}?view=embedded`);
}

export function useSkillDetail(skillId: string) {
  return useQuery({
    queryKey: queryKeys.skills.detail(skillId),
    queryFn: () => getSkillDetail(skillId),
    enabled: skillId.length > 0,
  });
}

export function useEmbeddedSkillDetail(skillId: string) {
  return useQuery({
    queryKey: queryKeys.skills.embedded(skillId),
    queryFn: () => getEmbeddedSkillDetail(skillId),
    enabled: skillId.length > 0,
  });
}

// useQueries, not a loop of useQuery: the id list length varies with the URL,
// and hook count must stay fixed across renders.
export function useEmbeddedSkillDetails(skillIds: string[]) {
  return useQueries({
    queries: skillIds.map((id) => ({
      queryKey: queryKeys.skills.embedded(id),
      queryFn: () => getEmbeddedSkillDetail(id),
    })),
  });
}

export function getSkillFiles(skillId: string) {
  return apiFetch<SkillFiles>(`/api/skills/${skillId}/files`);
}

export function useSkillFiles(skillId: string) {
  return useQuery({
    queryKey: queryKeys.skills.files(skillId),
    queryFn: () => getSkillFiles(skillId),
    enabled: skillId.length > 0,
  });
}

export function getSkillVersions(skillId: string) {
  return apiFetch<SkillVersions>(`/skills/${skillId}/versions`);
}

export function useSkillVersions(skillId: string) {
  return useQuery({
    queryKey: queryKeys.skills.versions(skillId),
    queryFn: () => getSkillVersions(skillId),
    enabled: skillId.length > 0,
  });
}

export function useOwnSkills() {
  return useQuery({
    queryKey: queryKeys.skills.own,
    queryFn: () => apiFetch<OwnSkills>("/skills"),
  });
}

export function skillDiffUrl(skillId: string, from: string, to: string) {
  const params = new URLSearchParams({ from, to });
  return `/skills/${skillId}/diff?${params.toString()}`;
}

export function deleteSkill(skillId: string) {
  return apiFetch<SkillDeletion>(`/skills/${skillId}`, { method: "DELETE" });
}

export function useDeleteSkill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: deleteSkill,
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.skills.own }),
  });
}

export function forkSkill(skillId: string) {
  return apiFetch<ForkedSkill>(`/skills/${skillId}/fork`, { method: "POST" });
}

export function useForkSkill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: forkSkill,
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.skills.own }),
  });
}

export type SkillCategoryChoice = SetSkillCategoryRequest["category"];

export function setSkillCategory(skillId: string, category: SkillCategoryChoice) {
  return apiFetch<unknown>(`/skills/${skillId}/category`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ category }),
  });
}

export function useSetSkillCategory(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (category: SkillCategoryChoice) => setSkillCategory(skillId, category),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.skills.detail(skillId) }),
  });
}
