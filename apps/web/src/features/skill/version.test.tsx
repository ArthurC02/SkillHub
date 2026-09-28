import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import {
  OWN_PUBLICATION,
  OWN_PUBLISHER,
  SKILL,
  SKILL_VERSIONS,
  VERSION,
  VERSION_DIFF,
  skillDetail,
} from "../../testing/fixtures/platform";
import { SkillVersion } from "./version/SkillVersion.page";

let container: HTMLDivElement;
let root: Root;
let routeVersion = VERSION;

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    children,
    className,
    ...rest
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    children?: unknown;
    className?: string;
    [key: string]: unknown;
  }) => {
    const path = Object.entries(params ?? {}).reduce(
      (current, [key, value]) => current.replace(`$${key}`, value),
      to,
    );
    const query = new URLSearchParams(
      Object.entries(search ?? {}).filter((entry): entry is [string, string] => Boolean(entry[1])),
    );
    return (
      <a className={className} href={`${path}${query.size ? `?${query}` : ""}`} {...rest}>
        {children as never}
      </a>
    );
  },
  useParams: () => ({ skillId: SKILL, versionId: routeVersion }),
}));

beforeEach(() => {
  routeVersion = VERSION;
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

function stubVersions(versions = SKILL_VERSIONS) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    calls.push(url);
    const path = url.split("?")[0];
    if (path === `/api/skills/${SKILL}`) return json(skillDetail(SKILL, "PDF Summariser"));
    if (path === `/skills/${SKILL}/versions`) return json(versions);
    if (path === "/me/publisher") return json(OWN_PUBLISHER);
    if (path === `/skills/${SKILL}/publication`) return json(OWN_PUBLICATION);
    if (path === `/skills/${SKILL}/diff`) return json(VERSION_DIFF);
    return json({ error: "not found" }, 404);
  });
  return calls;
}

async function render(settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <SkillVersion />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(settled);
}

async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

const text = () => container.textContent ?? "";

test("an owned immutable version becomes one shareable context for validation, package and release", async () => {
  stubVersions();
  await render(() => text().includes("PDF Summariser v2"));

  expect(text()).toContain("v2，最新版本");
  expect(text()).toContain("已有 Release");
  expect(container.querySelector("#skill-version-file")).not.toBeNull();
  expect(
    container.querySelector(`a[href="/lab/test-cases?skill=${SKILL}&version=${VERSION}"]`),
  ).not.toBeNull();
  expect(
    container.querySelector(`a[href="/skills/${SKILL}/package?version=${VERSION}"]`),
  ).not.toBeNull();
  expect(
    container.querySelector(`a[href="/skills/${SKILL}/versions/${VERSION}"][aria-current="page"]`),
  ).not.toBeNull();
  expect(text()).not.toContain("Activity");
});

test("a version from another Skill cannot expose publish, upload or package actions", async () => {
  routeVersion = "99999999-9999-4999-8999-999999999999";
  const calls = stubVersions();
  await render(() => text().includes("無法開啟這個版本"));

  expect(text()).toContain("這個版本不屬於目前的 Skill");
  expect(container.querySelector("#skill-version-file")).toBeNull();
  expect(container.querySelector('a[href*="/package"]')).toBeNull();
  expect(calls).not.toContain("/me/publisher");
  expect(calls).not.toContain(`/skills/${SKILL}/publication`);
});

test("an empty owner-scoped version list is absence of access, not absence of history", async () => {
  stubVersions({ versions: [] });
  await render(() => text().includes("無法開啟這個版本"));

  expect(text()).toContain("無權檢視");
  expect(text()).toContain("這不代表它沒有版本");
});
