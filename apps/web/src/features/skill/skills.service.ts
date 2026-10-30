import {
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { CorrectedSearchRequest, SetSkillCategoryRequest } from "@skillhub/api-client-ts";
import {
  ListSkillsLimitMaximum,
  SearchMatchReasonsRequestSkillIdsMaxItems,
} from "@skillhub/api-client-ts/limits";
import {
  ApiError,
  SessionNotReadyError,
  apiFetch,
  isWriteResultUnknown,
} from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import { readSearchCorrection } from "../../core/api/searchIntent";
import { readCatalogResponse, readSearchResponse } from "./search-response.model";
import { isSkillReceipt, ownSkillsFromPages, readOwnSkillPage } from "./own-skills.model";
import type {
  CatalogResponse,
  ForkedSkill,
  PublicSearchResponse,
  SearchFilters,
  SearchMatchReasons,
  SkillDeletion,
  SkillDetail,
  SkillFiles,
  SkillVersionSummary,
  SkillVersions,
} from "../../core/api/types";

export const MAX_COMPARE = 3;

export async function searchSkills(
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
  return readSearchResponse(await apiFetch<unknown>(`/api/skills/search?${params.toString()}`));
}

export function useSkillSearch(
  query: string,
  filters: SearchFilters,
  enabled: boolean,
  purpose?: "reference",
) {
  return useQuery({
    queryKey: queryKeys.skills.search(query, filters, purpose),
    queryFn: () => searchSkills(query, filters, 20, purpose),
    enabled,
  });
}

async function fetchCorrectedSearch(query: string, filters: SearchFilters, correction: string) {
  return readSearchResponse(
    await apiFetch<unknown>("/api/skills/search", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query,
        ...readSearchCorrection(correction),
        filters,
        limit: 20,
      } satisfies CorrectedSearchRequest),
    }),
  );
}

export function useCorrectedSkillSearch(
  query: string,
  filters: SearchFilters,
  enabled: boolean,
  correction: string | undefined,
) {
  return useQuery({
    queryKey:
      correction === undefined
        ? queryKeys.skills.search(query, filters, undefined)
        : queryKeys.skills.correctedSearch(query, filters, correction),
    queryFn: () =>
      correction === undefined
        ? searchSkills(query, filters, 20)
        : fetchCorrectedSearch(query, filters, correction),
    enabled,
  });
}

function isMatchReasons(value: unknown, skillIds: string[]): value is SearchMatchReasons {
  if (!value || typeof value !== "object") return false;
  const reasons = (value as Partial<SearchMatchReasons>).reasons;
  if (!Array.isArray(reasons)) return false;
  const expectedIds = new Set(skillIds.map((id) => id.toLowerCase()));
  const seenIds = new Set<string>();
  return reasons.every((item: unknown) => {
    if (!item || typeof item !== "object") return false;
    const reason = item as Partial<SearchMatchReasons["reasons"][number]>;
    const id = typeof reason.skill_id === "string" ? reason.skill_id.toLowerCase() : "";
    if (
      !expectedIds.has(id) ||
      seenIds.has(id) ||
      typeof reason.reason !== "string" ||
      reason.reason.trim().length === 0 ||
      typeof reason.fits !== "boolean"
    )
      return false;
    seenIds.add(id);
    return true;
  });
}

async function fetchMatchReasons(query: string, skillIds: string[]) {
  const result = await apiFetch<unknown>("/api/skills/search/reasons", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, skill_ids: skillIds }),
  });
  if (!isMatchReasons(result, skillIds)) throw new Error("Invalid search reasons response");
  return result;
}

function reasonCandidates(search: PublicSearchResponse | undefined) {
  return (search?.results ?? [])
    .slice(0, SearchMatchReasonsRequestSkillIdsMaxItems)
    .map((hit) => hit.skill_id);
}

export function useMatchReasons(search: PublicSearchResponse | undefined) {
  const query = search?.reason_query ?? "";
  const skillIds = reasonCandidates(search);
  return useQuery({
    queryKey: queryKeys.skills.matchReasons(query, skillIds),
    queryFn: () => fetchMatchReasons(query, skillIds),
    enabled: query !== "" && skillIds.length > 0,
    staleTime: Infinity,
  });
}

export function withMatchReasons(
  search: PublicSearchResponse,
  answer: SearchMatchReasons | undefined,
): PublicSearchResponse {
  if (!Array.isArray(answer?.reasons) || answer.reasons.length === 0) return search;
  const written = new Map(answer.reasons.map((r) => [r.skill_id.toLowerCase(), r]));
  return {
    ...search,
    results: search.results.map((hit) => {
      const answered = written.get(hit.skill_id.toLowerCase());
      return answered?.reason
        ? {
            ...hit,
            match_reason: answered.reason,
            match_reason_source: "model",
            match_fits: answered.fits,
          }
        : hit;
    }),
  };
}

function requestCatalog(filters: SearchFilters, limit: number) {
  const params = new URLSearchParams({ limit: String(limit) });
  if (filters.script) params.set("script", filters.script);
  if (filters.validation) params.set("validation", filters.validation);
  if (filters.agent) params.set("agent", filters.agent);
  if (filters.tier) params.set("tier", filters.tier);
  if (filters.category) params.set("category", filters.category);
  return apiFetch<CatalogResponse>(`/api/skills/catalog?${params.toString()}`);
}

export async function browseCatalog(filters: SearchFilters = {}, limit = 100) {
  return readCatalogResponse(await requestCatalog(filters, limit));
}

export function useCatalog(filters: SearchFilters, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.skills.catalog(filters),
    queryFn: () => browseCatalog(filters),
    enabled,
  });
}

async function fetchCatalogTotal(filters: SearchFilters) {
  const response: Partial<CatalogResponse> | null = await requestCatalog(filters, 1);
  const total = response?.total;
  if (typeof total !== "number" || !Number.isSafeInteger(total) || total < 0) {
    throw new Error("Invalid catalog count response");
  }
  return total;
}

export function useCatalogTotal(filters: SearchFilters) {
  return useQuery({
    queryKey: queryKeys.skills.catalogTotal(filters),
    queryFn: () => fetchCatalogTotal(filters),
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

function isVersionSummary(value: unknown): value is SkillVersionSummary {
  if (!value || typeof value !== "object") return false;
  const version = value as Partial<SkillVersionSummary>;
  return (
    typeof version.version_id === "string" &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(version.version_id) &&
    typeof version.version_number === "number" &&
    Number.isSafeInteger(version.version_number) &&
    version.version_number > 0 &&
    typeof version.content_hash === "string" &&
    version.content_hash.trim().length > 0 &&
    typeof version.created_at === "string" &&
    Number.isFinite(Date.parse(version.created_at))
  );
}

function isVersionList(value: unknown): value is SkillVersions {
  if (!value || typeof value !== "object") return false;
  const versions = (value as Partial<SkillVersions>).versions;
  if (!Array.isArray(versions) || !versions.every(isVersionSummary)) return false;
  return (
    new Set(versions.map((version) => version.version_id.toLowerCase())).size === versions.length &&
    versions.every(
      (version, index) =>
        index === 0 || versions[index - 1].version_number > version.version_number,
    )
  );
}

export async function getSkillVersions(skillId: string) {
  const result = await apiFetch<unknown>(`/skills/${skillId}/versions`);
  if (!isVersionList(result)) throw new Error("Invalid skill version list response");
  return result;
}

export function useSkillVersions(skillId: string) {
  return useQuery({
    queryKey: queryKeys.skills.versions(skillId),
    queryFn: () => getSkillVersions(skillId),
    enabled: skillId.length > 0,
  });
}

export function useEmbeddedSkillVersions(skillIds: string[]) {
  return useQueries({
    queries: skillIds.map((id) => ({
      queryKey: queryKeys.skills.versions(id),
      queryFn: () => getSkillVersions(id),
    })),
  });
}

export function useOwnSkills(enabled = true) {
  return useQuery({
    queryKey: queryKeys.skills.own,
    queryFn: async () => readOwnSkillPage(await apiFetch<unknown>("/skills")),
    enabled,
  });
}

export function useOwnCopiesOf(skillId: string, enabled = true) {
  const query = useOwnSkillPages(ListSkillsLimitMaximum, enabled);
  const own = ownSkillsFromPages(
    query.error || query.fetchStatus !== "idle" ? undefined : query.data?.pages,
  );
  const source = skillId.toLowerCase();
  const copies = (own?.skills ?? []).filter(
    (skill) => skill.forked_from_skill_id?.toLowerCase() === source,
  );
  const complete = Boolean(own && !own.truncated && own.skills.length === own.total);
  return {
    ...query,
    copies,
    complete,
    partial: Boolean(own && !complete),
    loaded: own?.skills.length,
    total: own?.total,
  };
}

export const OWN_SKILL_PAGE_SIZE = 24;

export async function getOwnSkillPage(limit: number, offset: number) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return readOwnSkillPage(await apiFetch<unknown>(`/skills?${params.toString()}`), offset);
}

export function useOwnSkillPages(limit = OWN_SKILL_PAGE_SIZE, enabled = true) {
  return useInfiniteQuery({
    queryKey: queryKeys.skills.ownPages(limit),
    queryFn: ({ pageParam }) => getOwnSkillPage(limit, pageParam),
    initialPageParam: 0,
    enabled,
    getNextPageParam: (lastPage, _pages, lastPageParam) =>
      lastPage.truncated ? lastPageParam + lastPage.skills.length : undefined,
  });
}

export function skillDiffUrl(skillId: string, from: string, to: string) {
  const params = new URLSearchParams({ from, to });
  return `/skills/${skillId}/diff?${params.toString()}`;
}

export function deleteSkill(skillId: string) {
  return apiFetch<SkillDeletion>(`/skills/${skillId}`, { method: "DELETE" });
}

export function unreadableSkillSentence(error: unknown): string | undefined {
  if (!(error instanceof ApiError)) return undefined;
  if (error.status === 410) return "這個小工具已從目錄下架，內容不再提供。";
  if (error.status === 404) return "找不到這個小工具；它可能已被刪除，或你沒有權限看到它。";
  return undefined;
}

export function isSkillAlreadyGone(error: unknown) {
  return error instanceof ApiError && error.status === 404;
}

export function useDeleteSkill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: deleteSkill,
    onSettled: (_, error) => {
      if (!error || isSkillAlreadyGone(error) || isWriteResultUnknown(error)) {
        void client.invalidateQueries({ queryKey: queryKeys.skills.own });
      }
    },
  });
}

function isForkReceipt(value: unknown): value is ForkedSkill {
  if (!isSkillReceipt(value)) return false;
  const receipt = value as Partial<ForkedSkill>;
  return (
    typeof receipt.version_id === "string" &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(receipt.version_id) &&
    typeof receipt.version_number === "number" &&
    Number.isSafeInteger(receipt.version_number) &&
    receipt.version_number > 0
  );
}

export async function forkSkill(skillId: string) {
  const result = await apiFetch<unknown>(`/skills/${skillId}/fork`, { method: "POST" });
  if (!isForkReceipt(result)) throw new Error("Invalid fork response");
  return {
    ...result,
    skill_id: result.skill_id.toLowerCase(),
    version_id: result.version_id.toLowerCase(),
  };
}

export function useForkSkill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: forkSkill,
    onSettled: (_, error) => {
      if (!error || isWriteResultUnknown(error)) {
        void client.invalidateQueries({ queryKey: queryKeys.skills.own });
      }
    },
  });
}

export type SkillCategoryChoice = SetSkillCategoryRequest["category"];

export async function setSkillCategory(skillId: string, category: SkillCategoryChoice) {
  const result = await apiFetch<unknown>(`/skills/${skillId}/category`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ category }),
  });
  if (!isSkillReceipt(result) || result.skill_id.toLowerCase() !== skillId.toLowerCase())
    throw new Error("Invalid category response");
  return result;
}

export function useSetSkillCategory(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (category: SkillCategoryChoice) => setSkillCategory(skillId, category),
    onSettled: (_, error) => {
      if (error instanceof SessionNotReadyError) return;
      void client.invalidateQueries({ queryKey: queryKeys.skills.detail(skillId) });
    },
  });
}
