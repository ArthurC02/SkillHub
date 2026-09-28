import { MAX_COMPARE } from "../../skill";
import type { HomeSearch } from "../../../app/router";
import type { SearchFilters } from "../../../core/api/types";

export const SEARCH_MAX_QUERY = 2000; // one-number: searchMaxQueryRunes

export const runes = (s: string) => [...s].length;

export function parseSelection(value: string | undefined): string[] {
  return value ? value.split(",").filter(Boolean).slice(0, MAX_COMPARE) : [];
}

export function nextSelection(current: string[], skillId: string): string[] {
  if (current.includes(skillId)) return current.filter((id) => id !== skillId);
  return current.length >= MAX_COMPARE ? current : [...current, skillId];
}

export function filtersFromSearch(
  search: Pick<HomeSearch, "script" | "validation" | "agent" | "tier" | "category">,
): SearchFilters {
  return {
    script: search.script,
    validation: search.validation,
    agent: search.agent,
    tier: search.tier,
    category: search.category,
  };
}

export function queryLengthError(trimmed: string): string | null {
  const count = runes(trimmed);
  return count > SEARCH_MAX_QUERY
    ? `搜尋文字最多 ${SEARCH_MAX_QUERY} 字，目前 ${count} 字。`
    : null;
}

export function clearedFilterFields(): SearchFilters {
  return {
    script: undefined,
    validation: undefined,
    agent: undefined,
    tier: undefined,
    category: undefined,
  };
}
