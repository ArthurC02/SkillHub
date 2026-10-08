import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  redirect,
} from "@tanstack/react-router";
import { RootLayout } from "./RootLayout";
import { RouteNotFound } from "../shared/ui/RouteNotFound";
import { RouteFailure } from "./RouteFailure";
import type { AgentRuntime, SkillCategory } from "../core/api/types";

const rootRoute = createRootRoute({ component: RootLayout });

export type HomeSearch = {
  q?: string;
  correction?: string;
  script?: "yes" | "no";
  validation?: "passed" | "unverified";
  agent?: AgentRuntime;
  tier?: "curated" | "indexed";
  category?: SkillCategory;
  compare?: string;
};

const AGENT_RUNTIMES: AgentRuntime[] = ["native", "transpiled", "failed", "unverified"];

const CATEGORIES: SkillCategory[] = ["documents", "writing", "data"];

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const PUBLICATION_REFERENCE = /^[^/]+\/[^/]+$/;

export type PublishingWorkspaceSearch = {
  artifact?: string;
  publication?: string;
  bundleVersion?: string;
};

function publishingArtifact(search: Record<string, unknown>) {
  return typeof search.artifact === "string" && UUID.test(search.artifact)
    ? search.artifact
    : undefined;
}

function publishingPublication(search: Record<string, unknown>) {
  return typeof search.publication === "string" && PUBLICATION_REFERENCE.test(search.publication)
    ? search.publication
    : undefined;
}

function publishingBundleVersion(search: Record<string, unknown>) {
  return typeof search.bundleVersion === "string" && UUID.test(search.bundleVersion)
    ? search.bundleVersion
    : undefined;
}

export function validatePublishingWorkspaceSearch(
  search: Record<string, unknown>,
): PublishingWorkspaceSearch {
  return {
    artifact: publishingArtifact(search),
    publication: publishingPublication(search),
    bundleVersion: publishingBundleVersion(search),
  };
}

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: lazyRouteComponent(() => import("../features/catalog/home/Home.page"), "Home"),
  validateSearch: (search: Record<string, unknown>): HomeSearch => ({
    q: typeof search.q === "string" ? search.q : undefined,
    correction:
      typeof search.correction === "string" ? search.correction : JSON.stringify(search.correction),
    script: search.script === "yes" || search.script === "no" ? search.script : undefined,
    validation:
      search.validation === "passed" || search.validation === "unverified"
        ? search.validation
        : undefined,
    agent: AGENT_RUNTIMES.includes(search.agent as AgentRuntime)
      ? (search.agent as AgentRuntime)
      : undefined,
    tier: search.tier === "curated" || search.tier === "indexed" ? search.tier : undefined,
    category: CATEGORIES.includes(search.category as SkillCategory)
      ? (search.category as SkillCategory)
      : undefined,
    compare: typeof search.compare === "string" && search.compare ? search.compare : undefined,
  }),
});

const skillDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId",
  component: lazyRouteComponent(
    () => import("../features/skill/detail/SkillDetail.page"),
    "SkillDetail",
  ),
});

const skillFilesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/files",
  component: lazyRouteComponent(
    () => import("../features/skill/files/SkillFiles.page"),
    "SkillFiles",
  ),
});

const skillVersionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/versions/$versionId",
  component: lazyRouteComponent(
    () => import("../features/skill/version/SkillVersion.page"),
    "SkillVersion",
  ),
});

const packagingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/package",
  component: lazyRouteComponent(
    () => import("../features/packaging/build/Packaging.page"),
    "Packaging",
  ),
  validateSearch: (search: Record<string, unknown>): { version?: string } => ({
    version: typeof search.version === "string" ? search.version : undefined,
  }),
});

const publicPublicationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/p/$publisher/$name",
  component: lazyRouteComponent(
    () => import("../features/publishing/PublicPublication.page"),
    "PublicPublication",
  ),
});

const downloadsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/downloads",
  component: lazyRouteComponent(
    () => import("../features/publishing/PublishingWorkspace.page"),
    "PublishingWorkspace",
  ),
  validateSearch: (search: Record<string, unknown>): PublishingWorkspaceSearch => ({
    artifact: publishingArtifact(search),
    publication: publishingPublication(search),
    bundleVersion: publishingBundleVersion(search),
  }),
});

const workspaceHomeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace",
  component: lazyRouteComponent(
    () => import("../features/workspace/home/WorkspaceHome.page"),
    "WorkspaceHome",
  ),
});

const libraryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/library",
  component: lazyRouteComponent(
    () => import("../features/workspace/skills/WorkspaceSkills.page"),
    "WorkspaceSkills",
  ),
});

export function legacyLibraryDestination(hash: string) {
  return { to: "/library", hash } as const;
}

const legacyWorkspaceSkillsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/skills",
  beforeLoad: ({ location }) => {
    throw redirect(legacyLibraryDestination(location.hash));
  },
});

const importSkillRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/import",
  component: lazyRouteComponent(
    () => import("../features/creation/import/ImportSkill.page"),
    "ImportSkill",
  ),
});

const createSkillRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/creations",
  component: lazyRouteComponent(
    () => import("../features/creation/create/CreateSkill.page"),
    "CreateSkill",
  ),
  validateSearch: (search: Record<string, unknown>): { session?: string } => ({
    session:
      typeof search.session === "string" && UUID.test(search.session) ? search.session : undefined,
  }),
});

const workspaceAccountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/account",
  component: lazyRouteComponent(
    () => import("../features/workspace/account/WorkspaceAccount.page"),
    "WorkspaceAccount",
  ),
});

const dataPolicyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/policy",
  component: lazyRouteComponent(
    () => import("../features/workspace/policy/DataPolicy.page"),
    "DataPolicy",
  ),
});

const workspaceRunsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/runs",
  component: lazyRouteComponent(
    () => import("../features/runs/list/WorkspaceRuns.page"),
    "WorkspaceRuns",
  ),
});

const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/activity",
  component: lazyRouteComponent(() => import("../features/activity/Activity.page"), "Activity"),
});

const compareRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/compare",
  component: lazyRouteComponent(
    () => import("../features/catalog/compare/Compare.page"),
    "Compare",
  ),
  validateSearch: (search: Record<string, unknown>) => ({
    ids: typeof search.ids === "string" ? search.ids : "",
  }),
});

const runPreflightRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/test-cases/$testCaseId/runs/new",
  component: lazyRouteComponent(
    () => import("../features/lab/preflight/RunPreflight.page"),
    "RunPreflight",
  ),
  validateSearch: (search: Record<string, unknown>) => ({
    version: typeof search.version === "string" ? search.version : undefined,
  }),
});

type LegacyRunSearch = { skill?: string; version?: string; test_case?: string };

export function legacyRunDestination(search: LegacyRunSearch) {
  if (search.skill && search.test_case) {
    return {
      to: "/skills/$skillId/test-cases/$testCaseId/runs/new",
      params: { skillId: search.skill, testCaseId: search.test_case },
      search: { version: search.version },
    } as const;
  }
  if (search.test_case) {
    return {
      to: "/lab/test-cases/$testCaseId",
      params: { testCaseId: search.test_case },
      search: { version: search.version },
    } as const;
  }
  return {
    to: "/lab/test-cases",
    search: { skill: search.skill, version: search.version },
  } as const;
}

const legacyRunPreflightRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/run",
  validateSearch: (search: Record<string, unknown>) => ({
    skill: typeof search.skill === "string" ? search.skill : undefined,
    version: typeof search.version === "string" ? search.version : undefined,
    test_case: typeof search.test_case === "string" ? search.test_case : undefined,
  }),
  beforeLoad: ({ search }) => {
    throw redirect(legacyRunDestination(search));
  },
});

const datasetUploadRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases/$testCaseId/datasets",
  component: lazyRouteComponent(
    () => import("../features/lab/dataset-upload/DatasetUpload.page"),
    "DatasetUpload",
  ),
  validateSearch: (search: Record<string, unknown>) => ({
    version:
      typeof search.version === "string" && UUID.test(search.version) ? search.version : undefined,
  }),
});

type LegacyDatasetSearch = { test_case?: string; version?: string };

export function legacyDatasetDestination(search: LegacyDatasetSearch) {
  if (search.test_case) {
    return {
      to: "/lab/test-cases/$testCaseId/datasets",
      params: { testCaseId: search.test_case },
      search: { version: search.version },
    } as const;
  }
  return { to: "/lab/test-cases", search: {} } as const;
}

const legacyDatasetUploadRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/datasets",
  validateSearch: (search: Record<string, unknown>) => ({
    test_case: typeof search.test_case === "string" ? search.test_case : undefined,
    version:
      typeof search.version === "string" && UUID.test(search.version) ? search.version : undefined,
  }),
  beforeLoad: ({ search }) => {
    throw redirect(legacyDatasetDestination(search));
  },
});

export type RunSearch = { evaluation?: string; events?: string };

const runTraceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/runs/$runId",
  component: lazyRouteComponent(() => import("../features/runs/trace/RunTrace.page"), "RunTrace"),
  validateSearch: (search: Record<string, unknown>): RunSearch => ({
    evaluation: typeof search.evaluation === "string" ? search.evaluation : undefined,
    events:
      typeof search.events === "string" && /^\d+(,\d+)*$/.test(search.events)
        ? search.events
        : undefined,
  }),
});

const runCompareRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/runs/$runId/compare",
  component: lazyRouteComponent(
    () => import("../features/runs/compare/RunCompare.page"),
    "RunCompare",
  ),
  validateSearch: (search: Record<string, unknown>): { against?: string } => ({
    against:
      typeof search.against === "string" && search.against.length > 0 ? search.against : undefined,
  }),
});

const testCaseListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases",
  component: lazyRouteComponent(
    () => import("../features/lab/test-cases/TestCaseList.page"),
    "TestCaseList",
  ),
  validateSearch: (search: Record<string, unknown>): { skill?: string; version?: string } => ({
    skill: typeof search.skill === "string" && UUID.test(search.skill) ? search.skill : undefined,
    version:
      typeof search.version === "string" && UUID.test(search.version) ? search.version : undefined,
  }),
});

const testCaseDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases/$testCaseId",
  component: lazyRouteComponent(
    () => import("../features/lab/test-cases/TestCaseDetail.page"),
    "TestCaseDetail",
  ),
  validateSearch: (search: Record<string, unknown>): { version?: string } => ({
    version:
      typeof search.version === "string" && UUID.test(search.version) ? search.version : undefined,
  }),
});

const adminHomeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin",
  component: lazyRouteComponent(() => import("../features/admin/home/AdminHome.page"), "AdminHome"),
});

const adminAccountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/accounts",
  component: lazyRouteComponent(
    () => import("../features/admin/accounts/AdminAccounts.page"),
    "AdminAccounts",
  ),
});

const adminSkillsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/skills",
  component: lazyRouteComponent(
    () => import("../features/admin/skills/AdminSkills.page"),
    "AdminSkills",
  ),
  validateSearch: (search: Record<string, unknown>): { q?: string } => ({
    q: typeof search.q === "string" && search.q.trim() ? search.q.trim() : undefined,
  }),
});

const adminDispatchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/dispatch",
  component: lazyRouteComponent(
    () => import("../features/admin/dispatch/AdminDispatch.page"),
    "AdminDispatch",
  ),
});

const adminRostersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/rosters",
  component: lazyRouteComponent(
    () => import("../features/admin/rosters/AdminRosters.page"),
    "AdminRosters",
  ),
});

const adminAuditLogRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/audit-log",
  component: lazyRouteComponent(
    () => import("../features/admin/audit-log/AdminAuditLog.page"),
    "AdminAuditLog",
  ),
  validateSearch: (search: Record<string, unknown>): { workspace_id?: string } => ({
    workspace_id:
      search.workspace_id !== undefined
        ? typeof search.workspace_id === "string"
          ? search.workspace_id.trim()
          : String(search.workspace_id)
        : undefined,
  }),
});

const adminModelBudgetsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/model-budgets",
  component: lazyRouteComponent(
    () => import("../features/admin/model-budgets/AdminModelBudgets.page"),
    "AdminModelBudgets",
  ),
});

const adminCostStatisticsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/cost-statistics",
  component: lazyRouteComponent(
    () => import("../features/admin/cost-statistics/AdminCostStatistics.page"),
    "AdminCostStatistics",
  ),
});

const adminTrendsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/trends",
  component: lazyRouteComponent(
    () => import("../features/admin/trends/AdminTrends.page"),
    "AdminTrends",
  ),
  validateSearch: (search: Record<string, unknown>): { days?: 7 | 30 | 90 } => ({
    days: ([7, 30, 90] as const).find((days) => days === Number(search.days)),
  }),
});

const adminExposureRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/exposure",
  component: lazyRouteComponent(
    () => import("../features/admin/exposure/AdminExposure.page"),
    "AdminExposure",
  ),
  validateSearch: (search: Record<string, unknown>): { publication?: string } => ({
    publication:
      typeof search.publication === "string" && /^[^/]+\/[^/]+$/.test(search.publication)
        ? search.publication
        : undefined,
  }),
});

const adminAgentsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin/agents",
  component: lazyRouteComponent(
    () => import("../features/admin/agents/AdminAgents.page"),
    "AdminAgents",
  ),
  validateSearch: (
    search: Record<string, unknown>,
  ): {
    status?: "resolved" | "dismissed" | "recovered";
    finding?: string;
    proposal?: string;
    run?: string;
  } => ({
    status: (["resolved", "dismissed", "recovered"] as const).find((s) => s === search.status),
    finding:
      typeof search.finding === "string" && UUID.test(search.finding) ? search.finding : undefined,
    proposal:
      typeof search.proposal === "string" && UUID.test(search.proposal)
        ? search.proposal
        : undefined,
    run: typeof search.run === "string" && UUID.test(search.run) ? search.run : undefined,
  }),
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  compareRoute,
  runTraceRoute,
  runCompareRoute,
  skillDetailRoute,
  skillFilesRoute,
  skillVersionRoute,
  packagingRoute,
  publicPublicationRoute,
  downloadsRoute,
  workspaceHomeRoute,
  libraryRoute,
  legacyWorkspaceSkillsRoute,
  importSkillRoute,
  createSkillRoute,
  activityRoute,
  workspaceRunsRoute,
  workspaceAccountRoute,
  dataPolicyRoute,
  runPreflightRoute,
  legacyRunPreflightRoute,
  datasetUploadRoute,
  legacyDatasetUploadRoute,
  testCaseListRoute,
  testCaseDetailRoute,
  adminHomeRoute,
  adminAccountsRoute,
  adminSkillsRoute,
  adminDispatchRoute,
  adminRostersRoute,
  adminAuditLogRoute,
  adminModelBudgetsRoute,
  adminCostStatisticsRoute,
  adminTrendsRoute,
  adminExposureRoute,
  adminAgentsRoute,
]);

export function createAppRouter() {
  return createRouter({
    routeTree,
    defaultNotFoundComponent: RouteNotFound,
    defaultErrorComponent: RouteFailure,
  });
}

export const router = createAppRouter();

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
