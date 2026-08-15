import { Link, Outlet, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { Compare } from "./pages/Compare";
import { Home } from "./pages/Home";
import { RunPreflight } from "./pages/RunPreflight";
import { SkillDetail } from "./pages/SkillDetail";
import { SkillFiles } from "./pages/SkillFiles";

function RootLayout() {
  return (
    <div className="app-shell">
      <header className="app-header">
        <Link to="/" className="app-title">
          Skill Hub
        </Link>
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
};

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

const routeTree = rootRoute.addChildren([
  indexRoute,
  compareRoute,
  skillDetailRoute,
  skillFilesRoute,
  runPreflightRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
