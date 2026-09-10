import {
  Link,
  Outlet,
  createRootRoute,
  createRoute,
  createRouter,
  useRouterState,
} from "@tanstack/react-router";
import { FeedbackEntry } from "./components/FeedbackEntry";
import { AuthControls } from "./components/AuthControls";
import { CleanModeNotice } from "./components/CleanModeNotice";
import { Compare } from "./pages/Compare";
import { DataPolicy } from "./pages/DataPolicy";
import { DatasetUpload } from "./pages/DatasetUpload";
import { Downloads } from "./pages/Downloads";
import { Home } from "./pages/Home";
import { ImportSkill } from "./pages/ImportSkill";
import { CreateSkill } from "./pages/CreateSkill";
import { Packaging } from "./pages/Packaging";
import { RunCompare } from "./pages/RunCompare";
import { RunPreflight } from "./pages/RunPreflight";
import { RunTrace } from "./pages/RunTrace";
import { SkillDetail } from "./pages/SkillDetail";
import { SkillFiles } from "./pages/SkillFiles";
import { TestCaseDetail, TestCaseList } from "./pages/TestCases";
import { WorkspaceAccount } from "./pages/WorkspaceAccount";
import { WorkspaceRuns } from "./pages/WorkspaceRuns";
import { WorkspaceSkills } from "./pages/WorkspaceSkills";
import type { AgentRuntime, SkillCategory } from "./api/types";

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
        <CleanModeNotice />
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
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
