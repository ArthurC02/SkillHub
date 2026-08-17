import { Link, Outlet, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { Compare } from "./pages/Compare";
import { DatasetUpload } from "./pages/DatasetUpload";
import { Downloads } from "./pages/Downloads";
import { Home } from "./pages/Home";
import { Packaging } from "./pages/Packaging";
import { RunCompare } from "./pages/RunCompare";
import { RunPreflight } from "./pages/RunPreflight";
import { RunTrace } from "./pages/RunTrace";
import { SkillDetail } from "./pages/SkillDetail";
import { SkillFiles } from "./pages/SkillFiles";
import { TestCaseDetail, TestCaseList } from "./pages/TestCases";
import type { AgentRuntime } from "./api/types";

function RootLayout() {
  return (
    <div className="app-shell">
      <header className="app-header">
        <Link to="/" className="app-title">
          Skill Hub
        </Link>{" "}
        <Link to="/lab/test-cases">Test Case</Link> <Link to="/workspace/downloads">下載紀錄</Link>
      </header>
      <main>
        <Outlet />
      </main>
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
 * 03:TEST-008/009. The three ids live in the URL because the Lab has no picker
 * yet (see the scope note in RunPreflight.tsx); when DESIGN-007 lands, the page
 * that owns the test case links here with them filled in.
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
