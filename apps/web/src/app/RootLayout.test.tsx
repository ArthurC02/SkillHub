import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RootLayout } from "./RootLayout";

const mocks = vi.hoisted(() => ({ generateExposed: false, pathname: "/workspace" }));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children: unknown }) => (
    <a href={to}>{children as never}</a>
  ),
  Outlet: () => <div />,
  useNavigate: () => () => Promise.resolve(),
  useRouterState: ({ select }: { select: (state: { location: { pathname: string } }) => string }) =>
    select({ location: { pathname: mocks.pathname } }),
}));

vi.mock("../features/creation", () => ({
  useGenerateEntryPoint: () => mocks.generateExposed,
}));

vi.mock("../core/session/me.service", () => ({
  useMe: () => ({ data: { display_name: "tester" } }),
}));

vi.mock("./shell/AuthControls", () => ({
  AuthControls: () => <div data-auth-controls>tester</div>,
}));

vi.mock("./shell/CleanModeNotice", () => ({
  CleanModeNotice: () => null,
}));

vi.mock("./shell/FeedbackLauncher", () => ({
  FeedbackLauncher: () => null,
}));

vi.mock("../shared/ui/NavScrollCue", () => ({
  NavScrollCue: () => null,
}));

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  mocks.generateExposed = false;
  mocks.pathname = "/workspace";
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function renderShell() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <RootLayout />
      </StrictMode>,
    );
  });
}

test("the platform shell exposes stable places and hides Studio until generation is enabled", async () => {
  await renderShell();

  const nav = container.querySelector('nav[aria-label="主要導覽"]')!;
  expect(container.querySelector('[aria-label="目前 Workspace"]')?.textContent).toBe(
    "Workspacetester",
  );
  expect(nav.textContent).toContain("首頁");
  expect(nav.textContent).toContain("Catalog");
  expect(nav.textContent).toContain("資產庫");
  expect(nav.querySelector('a[href="/library"]')).not.toBeNull();
  const trialActivity = Array.from(nav.querySelectorAll("a")).find(
    (link) => link.textContent === "試跑活動",
  );
  expect(trialActivity?.getAttribute("href")).toBe("/workspace/runs");
  expect(nav.textContent).toContain("發佈");
  expect(nav.textContent).not.toContain("Studio");
  expect(nav.textContent).not.toContain("匯入 Skill");
  expect(nav.textContent).not.toContain("Test Case");
  expect(container.querySelector('form[role="search"]')).not.toBeNull();
});

test("the platform shell exposes Studio when generation is enabled", async () => {
  mocks.generateExposed = true;
  await renderShell();

  const studio = Array.from(container.querySelectorAll("nav a")).find(
    (link) => link.textContent === "Studio",
  );
  expect(studio?.getAttribute("href")).toBe("/workspace/creations");
});

test("Catalog owns its search instead of receiving a duplicate shell form", async () => {
  mocks.pathname = "/";
  await renderShell();

  expect(container.querySelector('form[role="search"]')).toBeNull();
});
