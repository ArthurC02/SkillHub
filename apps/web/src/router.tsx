import { Link, Outlet, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { Compare } from "./pages/Compare";
import { Home } from "./pages/Home";
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

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: Home,
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

const routeTree = rootRoute.addChildren([
  indexRoute,
  compareRoute,
  skillDetailRoute,
  skillFilesRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
