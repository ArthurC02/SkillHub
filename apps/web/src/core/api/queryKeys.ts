import type { SearchFilters } from "./types";

function filterKey(filters: SearchFilters) {
  return [
    filters.script ?? "",
    filters.validation ?? "",
    filters.agent ?? "",
    filters.tier ?? "",
    filters.category ?? "",
  ];
}

// A key is a path: invalidating one key also invalidates every key it prefixes,
// so skills.detail(id) refreshes that skill's files and versions as well.
export const queryKeys = {
  me: ["me"],
  credits: ["credits"],
  dataRetentionPolicy: ["policy", "data-retention"],
  skills: {
    importLimits: ["skills", "import-limits"],
    search: (query: string, filters: SearchFilters, purpose?: string) => [
      "skills",
      "search",
      query,
      ...filterKey(filters),
      purpose ?? "",
    ],
    catalog: (filters: SearchFilters) => ["skills", "catalog", ...filterKey(filters)],
    catalogTotal: (filters: SearchFilters) => ["skills", "catalog", "total", ...filterKey(filters)],
    detail: (skillId: string) => ["skills", skillId],
    embedded: (skillId: string) => ["skills", skillId, "embedded"],
    files: (skillId: string) => ["skills", skillId, "files"],
    versions: (skillId: string) => ["skills", skillId, "versions"],
    own: ["own-skills"],
  },
  generate: { failures: ["generate", "failures"] },
  creation: {
    sessions: ["creation-sessions"],
    limits: ["creation-limits"],
    session: (id: string) => ["creation-session", id],
  },
  testCases: {
    all: ["test-cases"],
    lists: ["test-cases", "list"],
    list: (skillId?: string) => ["test-cases", "list", skillId ?? ""],
    detail: (testCaseId: string) => ["test-cases", testCaseId],
    datasets: (testCaseId: string) => ["test-cases", testCaseId, "datasets"],
  },
  lab: {
    datasetLimits: ["dataset-limits"],
    preflight: (skillId: string, versionId: string, testCaseId: string) => [
      "preflight",
      skillId,
      versionId,
      testCaseId,
    ],
  },
  runs: {
    lists: ["runs"],
    list: (testCaseId?: string) => ["runs", testCaseId ?? ""],
    detail: (runId: string) => ["run", runId],
    artifacts: (runId: string) => ["run", runId, "artifacts"],
  },
  trace: {
    run: (runId: string) => ["trace", runId],
    page: (runId: string, mode: string, after: number) => ["trace", runId, mode, after],
  },
  evaluation: {
    run: (runId: string) => ["evaluation", runId],
    revision: (runId: string, revision?: string) => ["evaluation", runId, revision ?? "current"],
    revisions: (runId: string) => ["evaluation", runId, "revisions"],
    suggestions: (runId: string) => ["suggestions", runId],
    suggestionDiff: (suggestionId: string) => ["suggestion-diff", suggestionId],
    comparison: (runId: string, against: string) => ["comparison", runId, against],
    versionDiff: (url: string) => ["version-diff", url],
  },
  packaging: {
    targets: ["packaging", "targets"],
    preview: (skillId: string, versionId: string, target: string, includeTestCases: boolean) => [
      "packaging",
      "preview",
      skillId,
      versionId,
      target,
      includeTestCases,
    ],
    downloads: ["downloads"],
    downloadRecords: (artifactId: string) => ["downloads", artifactId, "records"],
  },
  admin: {
    account: (email: string) => ["admin", "account", email],
    ledger: (workspaceId: string) => ["admin", "ledger", workspaceId],
    skills: ["admin", "skills"],
    skillSearch: (q: string) => ["admin", "skills", q],
    dispatch: ["admin", "dispatch"],
    rosters: ["admin", "rosters"],
    auditLog: ["admin", "audit-log"],
    costStatistics: ["admin", "cost-statistics"],
    modelBudgets: ["admin", "model-budgets"],
    trend: (path: string, days: number) => ["admin", "trends", path, days],
  },
} as const;
