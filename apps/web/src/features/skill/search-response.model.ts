import {
  INTENT_FIELDS,
  INTENT_FIELD_LIMIT,
  INTENT_KEYWORD_LENGTH,
  INTENT_KEYWORD_LIMIT,
} from "../../core/api/searchIntent";
import type {
  CatalogResponse,
  PublicSearchResponse,
  ReasonedSearchResult,
} from "../../core/api/types";

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isText(value: unknown): value is string {
  return typeof value === "string";
}

function isMember(value: unknown, values: readonly string[]): boolean {
  return isText(value) && values.includes(value);
}

function optionalText(value: Record<string, unknown>, ...keys: string[]): boolean {
  return keys.every((key) => value[key] === undefined || isText(value[key]));
}

function isDate(value: unknown): boolean {
  return isText(value) && Number.isFinite(Date.parse(value));
}

function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function isLabelled(value: unknown, values?: readonly string[]): boolean {
  return (
    isRecord(value) &&
    isText(value.value) &&
    (values === undefined || values.includes(value.value)) &&
    isText(value.label) &&
    isText(value.note)
  );
}

function isRisk(value: unknown): boolean {
  return (
    isRecord(value) &&
    isMember(value.scan_status, ["scanned", "unavailable"]) &&
    isMember(value.level, ["none", "disclosed", "warning", "unknown"]) &&
    (value.scan_status !== "unavailable" || value.level === "unknown") &&
    isCount(value.warnings) &&
    Array.isArray(value.disclosures) &&
    value.disclosures.every(
      (item) => isRecord(item) && isText(item.code) && isText(item.label) && isText(item.note),
    ) &&
    isText(value.note)
  );
}

function isCompatibility(value: unknown): boolean {
  return (
    isRecord(value) &&
    isLabelled(value.spec_validation, ["unverified", "passed", "failed"]) &&
    isLabelled(value.capability, ["activated", "not_activated", "unverified"]) &&
    isLabelled(value.runtime, ["native", "transpiled", "failed", "unverified"]) &&
    isText(value.note) &&
    optionalText(value, "runtime_image") &&
    (value.measured_at === undefined || isDate(value.measured_at))
  );
}

function isRank(value: unknown): boolean {
  return (
    value === null ||
    (typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 1)
  );
}

function isResult(value: unknown): boolean {
  return (
    isRecord(value) &&
    isText(value.skill_id) &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value.skill_id) &&
    isText(value.name) &&
    isText(value.summary) &&
    isMember(value.summary_source, ["model", "package"]) &&
    isRank(value.rank) &&
    isLabelled(value.tier) &&
    isLabelled(value.category) &&
    isRisk(value.risk) &&
    Array.isArray(value.dependencies) &&
    value.dependencies.every(isText) &&
    isCompatibility(value.compatibility) &&
    optionalText(value, "rank_note", "match_reason") &&
    (value.match_reason_source === undefined ||
      isMember(value.match_reason_source, ["model", "template"])) &&
    (value.verified_at === undefined || isDate(value.verified_at))
  );
}

function isCatalog(value: unknown): value is CatalogResponse {
  return (
    isRecord(value) &&
    Array.isArray(value.results) &&
    value.results.every(isResult) &&
    isCount(value.limit) &&
    isCount(value.total) &&
    typeof value.truncated === "boolean" &&
    value.results.length <= value.limit &&
    (value.truncated ? value.total > value.results.length : value.total === value.results.length)
  );
}

function isInterpretation(value: unknown): boolean {
  if (!isRecord(value) || !isRecord(value.intent) || !isRecord(value.filters)) return false;
  const { intent, filters } = value;
  const filterValues: Record<string, readonly string[]> = {
    script: ["yes", "no"],
    validation: ["passed", "unverified"],
    agent: ["native", "transpiled", "failed", "unverified"],
    tier: ["curated", "indexed"],
    category: ["documents", "writing", "data"],
  };
  return (
    isMember(value.status, ["analyzed", "corrected", "fallback", "skipped"]) &&
    Object.keys(intent).length === INTENT_FIELDS.length &&
    INTENT_FIELDS.every(
      (key) =>
        intent[key] === null ||
        (isText(intent[key]) &&
          [...intent[key]].length > 0 &&
          [...intent[key]].length <= INTENT_FIELD_LIMIT),
    ) &&
    Array.isArray(value.keywords) &&
    value.keywords.length <= INTENT_KEYWORD_LIMIT &&
    value.keywords.every(
      (word) => isText(word) && [...word].length > 0 && [...word].length <= INTENT_KEYWORD_LENGTH,
    ) &&
    Object.keys(filters).every(
      (key) => Object.hasOwn(filterValues, key) && isMember(filters[key], filterValues[key]),
    ) &&
    optionalText(value, "model", "prompt_version") &&
    (value.fallback_reason === undefined ||
      isMember(value.fallback_reason, [
        "unavailable",
        "timeout",
        "invalid_response",
        "budget_exhausted",
      ]))
  );
}

export function readCatalogResponse(value: unknown): CatalogResponse {
  if (!isCatalog(value)) throw new Error("Invalid catalog response");
  return {
    ...value,
    results: value.results.map((row) => {
      const { match_fits: _matchFits, ...result } = row as ReasonedSearchResult;
      return result;
    }),
  };
}

export function readSearchResponse(value: unknown): PublicSearchResponse {
  if (
    !isCatalog(value) ||
    !isRecord(value) ||
    !isText(value.query) ||
    typeof value.degraded !== "boolean" ||
    typeof value.partial_index !== "boolean" ||
    typeof value.no_results !== "boolean" ||
    typeof value.filtered_out !== "boolean" ||
    (value.no_results && value.filtered_out) ||
    ((value.no_results || value.filtered_out) && value.results.length !== 0) ||
    !optionalText(value, "degraded_reason", "query_suggestion", "reason_query") ||
    (value.interpretation !== undefined && !isInterpretation(value.interpretation))
  )
    throw new Error("Invalid search response");
  return { ...value, ...readCatalogResponse(value) } as PublicSearchResponse;
}
