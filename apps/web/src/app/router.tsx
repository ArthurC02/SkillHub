import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  useRouterState,
} from "@tanstack/react-router";
import { FeedbackEntry } from "./shell/FeedbackEntry";
import { AuthControls } from "./shell/AuthControls";
import { CleanModeNotice } from "./shell/CleanModeNotice";
import { RouteNotFound } from "../shared/ui/RouteNotFound";
import { Compare } from "../features/catalog/compare/Compare.page";
import { DataPolicy } from "../features/workspace/policy/DataPolicy.page";
import { DatasetUpload } from "../features/lab/dataset-upload/DatasetUpload.page";
import { Downloads } from "../features/packaging/downloads/Downloads.page";
import { Home } from "../features/catalog/home/Home.page";
import { ImportSkill } from "../features/creation/import/ImportSkill.page";
import { CreateSkill } from "../features/creation/create/CreateSkill.page";
import { Packaging } from "../features/packaging/build/Packaging.page";
import { RunCompare } from "../features/runs/compare/RunCompare.page";
import { RunPreflight } from "../features/lab/preflight/RunPreflight.page";
import { RunTrace } from "../features/runs/trace/RunTrace.page";
import { SkillDetail } from "../features/skill/detail/SkillDetail.page";
import { SkillFiles } from "../features/skill/files/SkillFiles.page";
import { TestCaseDetail } from "../features/lab/test-cases/TestCaseDetail.page";
import { TestCaseList } from "../features/lab/test-cases/TestCaseList.page";
import { WorkspaceAccount } from "../features/workspace/account/WorkspaceAccount.page";
import { WorkspaceRuns } from "../features/runs/list/WorkspaceRuns.page";
import { WorkspaceSkills } from "../features/workspace/skills/WorkspaceSkills.page";
import type { AgentRuntime, SkillCategory } from "../core/api/types";

function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  return (
    <div className="app-shell" data-chat={pathname === "/workspace/creations" || undefined}>
      <header className="app-header">
        <Link to="/" className="app-title">
          Skill Hub
        </Link>
        <nav className="app-nav" aria-label="主要導覽">
          <Link to="/workspace/skills">我的 Skill</Link>
          <Link to="/workspace/runs">Run 歷史</Link>
          <Link to="/workspace/import">匯入 Skill</Link>
          <Link to="/lab/test-cases">Test Case</Link>
          <Link to="/workspace/downloads">下載紀錄</Link>
        </nav>
        <AuthControls />
      </header>
      <main>
        <CleanModeNotice admin={pathname === "/admin" || pathname.startsWith("/admin/")} />
        <Outlet />
      </main>
      <footer className="app-footer">
        <FeedbackEntry pathname={pathname} />
        <p className="note">
          <Link to="/policy">資料保存政策</Link>
          {" ｜ "}
          <Link to="/workspace/account">帳號與刪除</Link>
        </p>
        <details className="note">
          <summary>Build 識別碼</summary>
          <code>{import.meta.env.VITE_BUILD_ID ?? "未由建置注入（本機開發）"}</code>
        </details>
      </footer>
    </div>
  );
}

const rootRoute = createRootRoute({ component: RootLayout });

export type HomeSearch = {
  q?: string;
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
  component: Home,
  validateSearch: (search: Record<string, unknown>): HomeSearch => ({
    q: typeof search.q === "string" ? search.q : undefined,
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
  component: SkillDetail,
});

const skillFilesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/files",
  component: SkillFiles,
});

const packagingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/package",
  component: Packaging,
  validateSearch: (search: Record<string, unknown>) => ({
    version: typeof search.version === "string" ? search.version : undefined,
  }),
});

const downloadsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/downloads",
  component: Downloads,
});

const workspaceSkillsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/skills",
  component: WorkspaceSkills,
});

const importSkillRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/import",
  component: ImportSkill,
});

const createSkillRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/creations",
  component: CreateSkill,
});

const workspaceAccountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/account",
  component: WorkspaceAccount,
});

const dataPolicyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/policy",
  component: DataPolicy,
});

const workspaceRunsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/runs",
  component: WorkspaceRuns,
});

const compareRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/compare",
  component: Compare,
  validateSearch: (search: Record<string, unknown>) => ({
    ids: typeof search.ids === "string" ? search.ids : "",
  }),
});

const runPreflightRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/run",
  component: RunPreflight,
  validateSearch: (search: Record<string, unknown>) => ({
    skill: typeof search.skill === "string" ? search.skill : undefined,
    version: typeof search.version === "string" ? search.version : undefined,
    test_case: typeof search.test_case === "string" ? search.test_case : undefined,
  }),
});

const datasetUploadRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/datasets",
  component: DatasetUpload,
  validateSearch: (search: Record<string, unknown>) => ({
    test_case: typeof search.test_case === "string" ? search.test_case : undefined,
  }),
});

export type RunSearch = { evaluation?: string; events?: string };

const runTraceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/runs/$runId",
  component: RunTrace,
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
  component: RunCompare,
  validateSearch: (search: Record<string, unknown>) => ({
    against: typeof search.against === "string" ? search.against : "",
  }),
});

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const testCaseListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases",
  component: TestCaseList,
  validateSearch: (search: Record<string, unknown>): { skill?: string } => ({
    skill: typeof search.skill === "string" && UUID.test(search.skill) ? search.skill : undefined,
  }),
});

const testCaseDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases/$testCaseId",
  component: TestCaseDetail,
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

const routeTree = rootRoute.addChildren([
  indexRoute,
  compareRoute,
  runTraceRoute,
  runCompareRoute,
  skillDetailRoute,
  skillFilesRoute,
  packagingRoute,
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
  adminCostStatisticsRoute,
  adminTrendsRoute,
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
