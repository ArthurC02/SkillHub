import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
} from "@tanstack/react-router";
import { RootLayout } from "./RootLayout";
import { RouteNotFound } from "../shared/ui/RouteNotFound";
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

const packagingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/package",
  component: lazyRouteComponent(
    () => import("../features/packaging/build/Packaging.page"),
    "Packaging",
  ),
  validateSearch: (search: Record<string, unknown>) => ({
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
    () => import("../features/packaging/downloads/Downloads.page"),
    "Downloads",
  ),
});

const workspaceSkillsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/skills",
  component: lazyRouteComponent(
    () => import("../features/workspace/skills/WorkspaceSkills.page"),
    "WorkspaceSkills",
  ),
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
  path: "/lab/run",
  component: lazyRouteComponent(
    () => import("../features/lab/preflight/RunPreflight.page"),
    "RunPreflight",
  ),
  validateSearch: (search: Record<string, unknown>) => ({
    skill: typeof search.skill === "string" ? search.skill : undefined,
    version: typeof search.version === "string" ? search.version : undefined,
    test_case: typeof search.test_case === "string" ? search.test_case : undefined,
  }),
});

const datasetUploadRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/datasets",
  component: lazyRouteComponent(
    () => import("../features/lab/dataset-upload/DatasetUpload.page"),
    "DatasetUpload",
  ),
  validateSearch: (search: Record<string, unknown>) => ({
    test_case: typeof search.test_case === "string" ? search.test_case : undefined,
  }),
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
  validateSearch: (search: Record<string, unknown>) => ({
    against: typeof search.against === "string" ? search.against : "",
  }),
});

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const testCaseListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases",
  component: lazyRouteComponent(
    () => import("../features/lab/test-cases/TestCaseList.page"),
    "TestCaseList",
  ),
  validateSearch: (search: Record<string, unknown>): { skill?: string } => ({
    skill: typeof search.skill === "string" && UUID.test(search.skill) ? search.skill : undefined,
  }),
});

const testCaseDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases/$testCaseId",
  component: lazyRouteComponent(
    () => import("../features/lab/test-cases/TestCaseDetail.page"),
    "TestCaseDetail",
  ),
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

const routeTree = rootRoute.addChildren([
  indexRoute,
  compareRoute,
  runTraceRoute,
  runCompareRoute,
  skillDetailRoute,
  skillFilesRoute,
  packagingRoute,
  publicPublicationRoute,
  downloadsRoute,
  workspaceSkillsRoute,
  importSkillRoute,
  createSkillRoute,
  workspaceRunsRoute,
  workspaceAccountRoute,
  dataPolicyRoute,
  runPreflightRoute,
  datasetUploadRoute,
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
]);

export function createAppRouter() {
  return createRouter({ routeTree, defaultNotFoundComponent: RouteNotFound });
}

export const router = createAppRouter();

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
