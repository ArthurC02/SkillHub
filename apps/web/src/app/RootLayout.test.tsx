import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Outlet } from "@tanstack/react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useFeatureAvailability } from "../shared/featureAvailability";
import { RootLayout } from "./RootLayout";

function FeatureAvailabilityProbe() {
  const availability = useFeatureAvailability();
  return <div data-creation-available={availability.creation} />;
}

const mocks = vi.hoisted(() => ({
  creationExposed: false,
  generateExposed: false,
  outletTitle: "工作台",
  pathname: "/workspace",
  invited: true,
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children: unknown }) => (
    <a href={to}>{children as never}</a>
  ),
  Outlet: () => (
    <>
      <h1>{mocks.outletTitle}</h1>
      <FeatureAvailabilityProbe />
    </>
  ),
  useNavigate: () => () => Promise.resolve(),
  useRouterState: ({ select }: { select: (state: { location: { pathname: string } }) => string }) =>
    select({ location: { pathname: mocks.pathname } }),
}));

vi.mock("../features/creation", () => ({
  useCreationEntryPoint: () => mocks.creationExposed,
  useGenerateEntryPoint: () => mocks.generateExposed,
}));

vi.mock("../core/session/me.service", () => ({
  useMe: () => ({ data: { display_name: "tester", invited: mocks.invited } }),
  useUninvited: () => mocks.invited === false,
  useSessionBoundary: () => undefined,
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
let originalHeaderClearance: string;

beforeEach(() => {
  mocks.generateExposed = false;
  mocks.creationExposed = false;
  mocks.outletTitle = "工作台";
  mocks.pathname = "/workspace";
  document.title = "Skill Hub";
  originalHeaderClearance =
    document.documentElement.style.getPropertyValue("--app-header-clearance");
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  document.documentElement.style.setProperty("--app-header-clearance", originalHeaderClearance);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function mockHeaderMeasurement() {
  let height = 65;
  let notify = () => {};
  const observe = vi.fn();
  const disconnect = vi.fn();
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    () => ({ height }) as DOMRect,
  );
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        notify = callback;
      }
      observe = observe;
      disconnect = disconnect;
    },
  );
  return {
    observe,
    disconnect,
    resize(nextHeight: number) {
      height = nextHeight;
      notify();
    },
  };
}

async function renderShell() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <RootLayout>
          <Outlet />
        </RootLayout>
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
  expect(nav.textContent).toContain("工作台");
  expect(nav.textContent).toContain("探索");
  expect(container.querySelector<HTMLAnchorElement>('header > a[href="/"]')).not.toBeNull();
  expect(nav.textContent).toContain("資產庫");
  expect(nav.querySelector('a[href="/library"]')).not.toBeNull();
  const activity = Array.from(nav.querySelectorAll("a")).find(
    (link) => link.textContent === "活動",
  );
  expect(activity?.getAttribute("href")).toBe("/activity");
  expect(nav.textContent).toContain("發佈");
  expect(nav.textContent).not.toContain("Studio");
  expect(nav.textContent).not.toContain("匯入小工具");
  expect(nav.textContent).not.toContain("測試題");
  expect(container.querySelector('form[role="search"]')).not.toBeNull();
});

test.each([
  [false, true],
  [true, false],
])(
  "an account whose invitation is %s is told about the invite-only actions: %s",
  async (invited, told) => {
    mocks.invited = invited;
    try {
      await renderShell();
      expect((container.textContent ?? "").includes("這個帳號還沒有封測邀請")).toBe(told);
    } finally {
      mocks.invited = true;
    }
  },
);

test("keyboard users can skip repeated navigation and the browser tab names the current page", async () => {
  await renderShell();

  const skip = container.querySelector<HTMLAnchorElement>('a[href="#main-content"]');
  const main = container.querySelector<HTMLElement>("main#main-content");
  expect(skip?.textContent).toBe("跳到主要內容");
  expect(main?.tabIndex).toBe(-1);
  expect(document.title).toBe("工作台 · Skill Hub");
});

test("the platform shell exposes Studio when generation is enabled", async () => {
  mocks.generateExposed = true;
  await renderShell();

  const studio = Array.from(container.querySelectorAll("nav a")).find(
    (link) => link.textContent === "Studio",
  );
  expect(studio?.getAttribute("href")).toBe("/workspace/creations");
});

test("the platform shell keeps creation continuations closed until both gates are open", async () => {
  mocks.generateExposed = true;
  mocks.creationExposed = false;
  await renderShell();

  expect(
    container.querySelector("[data-creation-available]")?.getAttribute("data-creation-available"),
  ).toBe("false");
});

test("the platform shell opens creation continuations once both gates are open", async () => {
  mocks.generateExposed = true;
  mocks.creationExposed = true;
  await renderShell();

  expect(
    container.querySelector("[data-creation-available]")?.getAttribute("data-creation-available"),
  ).toBe("true");
});

test("Catalog owns its search instead of receiving a duplicate shell form", async () => {
  mocks.pathname = "/";
  await renderShell();

  expect(container.querySelector('form[role="search"]')).toBeNull();
});

test("the viewport keeps measured header clearance when its height changes", async () => {
  const measurement = mockHeaderMeasurement();
  await renderShell();

  expect(document.documentElement.style.getPropertyValue("--app-header-clearance")).toBe("73px");
  expect(measurement.observe).toHaveBeenLastCalledWith(container.querySelector("header"));
  await act(async () => measurement.resize(109));
  expect(document.documentElement.style.getPropertyValue("--app-header-clearance")).toBe("117px");
});

test("removing the shell stops header measurement and restores previous viewport clearance", async () => {
  document.documentElement.style.setProperty("--app-header-clearance", "17px");
  const measurement = mockHeaderMeasurement();
  await renderShell();
  expect(document.documentElement.style.getPropertyValue("--app-header-clearance")).toBe("73px");
  measurement.disconnect.mockClear();

  await act(async () => root.render(null));

  expect(measurement.disconnect).toHaveBeenCalledTimes(1);
  expect(document.documentElement.style.getPropertyValue("--app-header-clearance")).toBe("17px");
});
