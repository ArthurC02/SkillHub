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
        </Link>
        {/*
          A real landmark instead of a row of links glued together with {" "}:
          the menu was previously indistinguishable from body text to a screen
          reader, and on a narrow viewport it wrapped into the title. Named,
          because SkillDetail and SkillFiles each carry an unlabelled <nav> of
          their own and two same-named navigation landmarks is an axe
          landmark-unique violation.

          No activeProps and no change to any Link: the router already marks the
          matching one with data-status="active" and aria-current="page", which
          is both halves of "where am I" — see .app-nav in index.css.
        */}
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
        {/*
          PORT-003. First child of <main>, not before it: system.md §3
          checklist 第 1 條 wants the headline to be the first thing in the
          first screen, and a banner outside <main> would queue ahead of it on
          every page. Renders nothing unless the deployment has declared
          SKILLHUB_CLEAN_MODE (see CleanModeNotice.tsx's header for what this
          does and does not disclose for a signed-out visitor).
        */}
        <CleanModeNotice />
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
  tier?: "curated" | "indexed";
  /**
   * DISC-009 的候選勾選，逗號分隔，與 `/compare?ids=` 同一個形狀。
   *
   * **這不是一個新決定，是把既有的那個決定套完整。** `compareRoute` 的註解逐字寫著
   * 「the selection lives in the URL so a comparison is linkable and survives a
   * reload」——而在此之前，產生那份選擇的上一步把它放在 `useState` 裡，於是
   * 「比較 → 上一頁 → 換掉一筆 → 再比較」這條 DISC-009 自己的工作流，每走一次都要
   * 重新勾兩個。同一份東西，晚一步就撐得過重新整理，早一步就撐不過。
   *
   * **不是把 `q` 放到 `/compare` 上**（04 丙-136 原本提的修法）：R4 的判準是
   * 「你在看哪一份東西」，而 `q` 對 `/compare` 既不是它在看的東西也不是偏好，
   * 是「你從哪來」，§0.2 的偏離帳只收「R 的推導前提在這裡不成立」。而且查詢本來就
   * 沒有遺失——`submitSearch` 是 `replace: true`，瀏覽器上一頁會回到 `/?q=…`；
   * 遺失的一直都是勾選。
   */
  compare?: string;
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
    // `external` is deliberately not accepted: it means "never imported", so
    // the server refuses it too (curationTierValues in discovery/http.go).
    tier: search.tier === "curated" || search.tier === "indexed" ? search.tier : undefined,
    // 與 `/compare` 同一條規則：字串收下，超過 MAX_COMPARE 的部分由頁面修剪。
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
 * distinct shareable address (資訊架構 §0.1 R4, IA-4 的裁定).
 *
 * The two params that ARE here are the other half of R4 — 「你在看哪一份東西」:
 *
 * - `evaluation` names one immutable `evaluation_id` out of this run's
 *   revisions (ADR-003/026). Without it a superseded verdict could not be
 *   linked or reloaded into, and the address snapped back to 目前的判定 — which
 *   for a re-evaluated run is a DIFFERENT verdict from the one being discussed.
 * - `events` is the advanced Trace's cursor stack, comma-joined, holding page 2
 *   onwards (page 1 is the absent param). Trace is the screen most likely to be
 *   mailed to somebody with 「你看，這裡少了三筆」 attached, and page 7 of the
 *   event stream used to be unlinkable and lost on reload. Non-numeric junk is
 *   dropped rather than passed through, like the home route's out-of-enum
 *   filters: a hand-edited address lands on page 1, not on an error.
 *
 * There used to be a `skill` one, because applying improvement suggestions posts
 * to /skills/{id}/versions/from-suggestions and the run read carried no skill
 * id; GET /runs/{id} answers `skill_id` now, so the page works however it was
 * reached rather than only via the one link that remembered to pass it.
 */
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

/** 02:EVAL-003. The other run lives in the URL so a comparison is linkable. */
const runCompareRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/runs/$runId/compare",
  component: RunCompare,
  validateSearch: (search: Record<string, unknown>) => ({
    against: typeof search.against === "string" ? search.against : "",
  }),
});

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * 03:TEST-012. The Test Case screens, which are also the picker the Lab never
 * had: /lab/run and /lab/datasets take their ids from here instead of from a
 * user who had to know them.
 *
 * `skill` filters the list to one skill, which is what the Skill detail page's
 * 「此 Skill 的 Test Case」 link asks for. Anything that is not a UUID is
 * dropped rather than passed through, for the same reason the home route drops
 * an out-of-enum filter: the server answers 400 and a hand-edited URL should
 * land on the unfiltered list, not on an error page.
 */
const testCaseListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/lab/test-cases",
  component: TestCaseList,
  // Optional key, like HomeSearch: this list is linked to from six places that
  // have no skill to name, and a required `search` would make every one of them
  // pass an empty object to say so.
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
