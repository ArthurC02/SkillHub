import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { ForkedSkill, SearchHit, SkillDetail, SkillFiles, SkillVersionSummary } from "./types";

/**
 * Contract note (implementation rule 12): only `searchSkills` and `forkSkill`
 * below call routes that exist today in contracts/openapi/public.yaml — and
 * `/skills/search` there is scoped to the caller's own workspace (INGEST-009),
 * not the logged-out DISC-001 explorer search yet.
 *
 * `getSkillDetail`, `getSkillFiles`, and `getSkillVersions` call routes that
 * DISC-006/007/008 need (skill detail with capabilities/limitations/I-O/
 * dependencies/permissions/source/license/compatibility, SKILL.md + file
 * tree, version list) but that have no OpenAPI schema yet — `/skills/{id}`
 * currently only has DELETE, and the `Skill` schema has no detail fields.
 * These are written against the shape in api/types.ts (derived from
 * 02-specifications-and-acceptance-criteria.md §3 and §4.1 DISC-003) so the
 * UI has a concrete target; they need contracts/openapi/public.yaml updated
 * and a services/platform handler before they'll resolve against a real
 * backend. Once codegen is wired (ADR-019), replace this file's request
 * plumbing with the generated packages/api-client-ts client.
 */

export function searchSkills(query: string, limit = 20) {
  const params = new URLSearchParams({ q: query, limit: String(limit) });
  return apiFetch<{ results: SearchHit[] }>(`/skills/search?${params.toString()}`);
}

export function useSkillSearch(query: string) {
  return useQuery({
    queryKey: ["skills", "search", query],
    queryFn: () => searchSkills(query),
    enabled: query.trim().length > 0,
  });
}

export function getSkillDetail(skillId: string) {
  return apiFetch<SkillDetail>(`/skills/${skillId}`);
}

export function useSkillDetail(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId],
    queryFn: () => getSkillDetail(skillId),
    enabled: skillId.length > 0,
  });
}

export function getSkillFiles(skillId: string) {
  return apiFetch<SkillFiles>(`/skills/${skillId}/files`);
}

export function useSkillFiles(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId, "files"],
    queryFn: () => getSkillFiles(skillId),
    enabled: skillId.length > 0,
  });
}

export function getSkillVersions(skillId: string) {
  return apiFetch<{ versions: SkillVersionSummary[] }>(`/skills/${skillId}/versions`);
}

export function useSkillVersions(skillId: string) {
  return useQuery({
    queryKey: ["skills", skillId, "versions"],
    queryFn: () => getSkillVersions(skillId),
    enabled: skillId.length > 0,
  });
}

export function forkSkill(skillId: string) {
  return apiFetch<ForkedSkill>(`/skills/${skillId}/fork`, { method: "POST" });
}

export function useForkSkill() {
  return useMutation({ mutationFn: forkSkill });
}
