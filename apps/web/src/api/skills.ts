import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type {
  CatalogResponse,
  ForkedSkill,
  PublicSearchResponse,
  SearchFilters,
  SkillDeletion,
  SkillDetail,
  SkillFiles,
  SkillVersions,
} from "./types";

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
) {
  return useQuery({
    queryKey: [
      "skills",
      "search",
      query,
      filters.script ?? "",
      filters.validation ?? "",
      filters.agent ?? "",
      filters.tier ?? "",
      filters.category ?? "",
      purpose ?? "",
    ],
    queryFn: () => searchSkills(query, filters, 20, purpose),
    enabled,
    retry: false,
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
    queryKey: ["skills", "catalog", ...filterKey(filters)],
    queryFn: () => browseCatalog(filters),
    enabled,
    retry: false,
  });
}

function filterKey(filters: SearchFilters) {
  return [
    filters.script ?? "",
    filters.validation ?? "",
    filters.agent ?? "",
    filters.tier ?? "",
    filters.category ?? "",
  ];
}

export function useCatalogTotal(filters: SearchFilters) {
  return useQuery({
    queryKey: ["skills", "catalog", "total", ...filterKey(filters)],
    queryFn: () => browseCatalog(filters, 1),
    retry: false,
  });
}

export function getSkillDetail(skillId: string) {
  return apiFetch<SkillDetail>(`/api/skills/${skillId}`);
}

export function getEmbeddedSkillDetail(skillId: string) {
  return apiFetch<SkillDetail>(`/api/skills/${skillId}?view=embedded`);
}

export const embeddedSkillKey = (skillId: string) => ["skills", skillId, "embedded"];

export function useSkillDetail(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId],
    queryFn: () => getSkillDetail(skillId),
    enabled: skillId.length > 0,
    retry: false,
  });
}

export function useEmbeddedSkillDetail(skillId: string) {
  return useQuery({
    queryKey: embeddedSkillKey(skillId),
    queryFn: () => getEmbeddedSkillDetail(skillId),
    enabled: skillId.length > 0,
    retry: false,
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
    retry: false,
  });
}

export function getSkillVersions(skillId: string) {
  return apiFetch<SkillVersions>(`/skills/${skillId}/versions`);
}

export function useSkillVersions(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId, "versions"],
    queryFn: () => getSkillVersions(skillId),
    enabled: skillId.length > 0,
    retry: false,
  });
}

export function skillDiffUrl(skillId: string, from: string, to: string) {
  const params = new URLSearchParams({ from, to });
  return `/skills/${skillId}/diff?${params.toString()}`;
}

export function deleteSkill(skillId: string) {
  return apiFetch<SkillDeletion>(`/skills/${skillId}`, { method: "DELETE" });
}

export function forkSkill(skillId: string) {
  return apiFetch<ForkedSkill>(`/skills/${skillId}/fork`, { method: "POST" });
}

export function useForkSkill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: forkSkill,
    onSuccess: () => client.invalidateQueries({ queryKey: ["own-skills"] }),
  });
}
