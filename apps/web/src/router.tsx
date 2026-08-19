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
import { Compare } from "./pages/Compare";
import { DataPolicy } from "./pages/DataPolicy";
import { DatasetUpload } from "./pages/DatasetUpload";
import { Downloads } from "./pages/Downloads";
import { Home } from "./pages/Home";
import { ImportSkill } from "./pages/ImportSkill";
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
import type { AgentRuntime } from "./api/types";

/**
 * The feedback entry lives here rather than on the pages that refuse something,
 * because BETA-004 is about journeys that stop anywhere — including on a screen
 * nobody predicted would be the one (beta-design §5). It reads the current path
 * from the router so nothing has to be threaded down to it.
 *
 * In the footer and not the header, for a keyboard reason: it is a closed
 * `<details>` holding a whole form, and above `<main>` it would sit between the
 * top of every page and that page's own first control.
 */
function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  return (
    <div className="app-shell">
      <header className="app-header">
        <Link to="/" className="app-title">
          Skill Hub
        </Link>{" "}
        <Link to="/workspace/skills">我的 Skill</Link> <Link to="/workspace/runs">Run 歷史</Link>{" "}
        <Link to="/workspace/import">匯入 Skill</Link> <Link to="/lab/test-cases">Test Case</Link>{" "}
        <Link to="/workspace/downloads">下載紀錄</Link> <AuthControls />
      </header>
      <main>
        <Outlet />
      </main>
      <footer className="app-footer">
        <FeedbackEntry pathname={pathname} />
        {/*
          The data policy is reachable from every page and only from the footer:
          02:O11Y-004 makes it a disclosure obligation rather than a feature, and
          a disclosure that lives one click from wherever the reader already is
          is the shape that meets it. Beside the feedback entry because the two
          answer the same kind of question — "what happens to what I just did".
        */}
        <p className="note">
          <Link to="/policy">資料保存政策</Link>
          {" ｜ "}
          <Link to="/workspace/account">帳號與刪除</Link>
        </p>
      </footer>
    </div>
  );
}

const rootRoute = createRootRoute({ component: RootLayout });

/**
 * DISC-001/003: the query and the filters live in the URL so a filtered result
 * page is linkable and survives a reload.
 *
 * `q` is optional on purpose and `undefined` is not the same as `""`:
 * undefined means nothing has been submitted yet and no request is made, while
 * an empty string is a blank submit, which the server answers with no_results
 * plus the DISC-005 suggestion copy. Collapsing the two would delete that state.
 *
 * Filter values outside their enum are dropped rather than passed through: the
 * server rejects them with 400, and a hand-edited URL should land on an
 * unfiltered page, not an error page.
 */
/**
 * Every key is optional, which is what keeps `<Link to="/">` from demanding a
 * search object everywhere in the app.
 */
export type HomeSearch = {
  q?: string;
  script?: "yes" | "no";
  validation?: "passed" | "unverified";
  agent?: AgentRuntime;
};

const AGENT_RUNTIMES: AgentRuntime[] = ["native", "transpiled", "failed", "unverified"];

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

/**
 * 02:PACK-001/002. The version is a search param rather than a path segment
 * because the common case is "package the latest", which the page resolves from
 * the skill itself; a link from a version-specific place fills it in.
 */
const packagingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/skills/$skillId/package",
  component: Packaging,
  validateSearch: (search: Record<string, unknown>) => ({
    version: typeof search.version === "string" ? search.version : undefined,
  }),
});

/**
 * 02:WS-002 / WS-004. Under /workspace and not at /downloads: the API owns
 * GET /downloads, and in production the SPA and the API share an origin (see
 * vite.config.ts), so a page at that address would be a collision rather than a
 * deep link.
 */
const downloadsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/downloads",
  component: Downloads,
});

/**
 * 02:WS-002 第 1 條 / WS-004: the caller's own skills, served by GET /skills.
 * Under the same /workspace prefix as the download history for the same reason —
 * these are the workspace's own lists, not the public catalogue.
 */
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

/**
 * CORE-007 / 02:SEC-006: the account and its deletion, the second of the two
 * deletion planes. Under /workspace because the account and its personal
 * workspace are the same thing here (ADR-020) — a settings section separate from
 * the workspace would suggest there is more than one.
 */
const workspaceAccountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/account",
  component: WorkspaceAccount,
});

/**
 * 02:O11Y-004 / 04 丙-25②. Not under /workspace and deliberately not behind a
 * session: what the platform records about a visitor is a question that starts
 * being asked before anybody logs in, and the endpoint behind this page is the
 * only public one outside the catalogue for the same reason.
 */
const dataPolicyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/policy",
  component: DataPolicy,
});

/** 02:WS-002 第 1 條「Run 歷史」/ WS-004, served by GET /runs. */
const workspaceRunsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspace/runs",
  component: WorkspaceRuns,
});

// DISC-009: the selection lives in the URL so a comparison is linkable and
// survives a reload. Anything past MAX_COMPARE ids is trimmed by the page.
const compareRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/compare",
  component: Compare,
  validateSearch: (search: Record<string, unknown>) => ({
    ids: typeof search.ids === "string" ? search.ids : "",
  }),
});

/**
 * 03:TEST-008/009. All three ids can arrive in the URL, and the test case
 * screens link here with them filled in. Only `version` also has a picker on
 * the page (04 丙-14): skill and test case are chosen on the screens that own
 * them, so a picker here would be a second place to choose the same thing.
 */
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

/**
 * 02:TEST-002 / 03:TEST-004. The upload rules are displayed before the file
 * input, which is what "上傳前顯示" asks for. The test case id is in the URL for
 * the same reason as on the preflight route: no picker exists yet (DESIGN-007).
 */
const datasetUploadRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/datasets",
  component: DatasetUpload,
  validateSearch: (search: Record<string, unknown>) => ({
    test_case: typeof search.test_case === "string" ? search.test_case : undefined,
  }),
});

/**
 * 03:TRACE-006/007 plus the EVAL-001/002 report. Addressed by the platform
 * run_id and nothing else: a provider's ephemeral id never appears in a URL
 * (iron rule 10). The general and advanced modes are component state rather than
 * a search param — the toggle is a reading preference, not something worth a
 * distinct shareable address.
 *
 * No search params at all. There used to be a `skill` one, because applying
 * improvement suggestions posts to /skills/{id}/versions/from-suggestions and
 * the run read carried no skill id; GET /runs/{id} answers `skill_id` now, so
 * the page works however it was reached rather than only via the one link that
 * remembered to pass it.
 */
const runTraceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/runs/$runId",
  component: RunTrace,
});

/** 02:EVAL-003. The other run lives in the URL so a comparison is linkable. */
const runCompareRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/runs/$runId/compare",
  component: RunCompare,
  validateSearch: (search: Record<string, unknown>) => ({
    against: typeof search.against === "string" ? search.against : "",
  }),
});

/**
 * 03:TEST-012. The Test Case screens, which are also the picker the Lab never
 * had: /lab/run and /lab/datasets take their ids from here instead of from a
 * user who had to know them.
 */
const testCaseListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases",
  component: TestCaseList,
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
