import { Link, Outlet, createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
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

const routeTree = rootRoute.addChildren([indexRoute, skillDetailRoute, skillFilesRoute]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
