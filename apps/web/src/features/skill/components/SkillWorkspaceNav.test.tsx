import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SkillWorkspaceNav } from "./SkillWorkspaceNav";

let container: HTMLDivElement;
let root: Root;

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    search,
    children,
    className,
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    children?: unknown;
    className?: string;
  }) => {
    const path = Object.entries(params ?? {}).reduce(
      (current, [key, value]) => current.replace(`$${key}`, value),
      to,
    );
    const query = new URLSearchParams(
      Object.entries(search ?? {}).filter((entry): entry is [string, string] => Boolean(entry[1])),
    );
    return (
      <a className={className} href={`${path}${query.size ? `?${query}` : ""}`}>
        {children as never}
      </a>
    );
  },
}));

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
});

test("the workbench keeps every stable view on the same Skill and exact version", async () => {
  await act(async () => {
    root = createRoot(container);
    root.render(<SkillWorkspaceNav skillId="skill-1" versionId="version-2" />);
  });

  const nav = container.querySelector('nav[aria-label="這個 Skill 的工作台"]');
  expect(nav).not.toBeNull();
  expect(
    Array.from(nav!.querySelectorAll("a")).map((link) => [
      link.textContent,
      link.getAttribute("href"),
    ]),
  ).toEqual([
    ["總覽", "/skills/skill-1"],
    ["檔案", "/skills/skill-1/files"],
    ["驗證", "/lab/test-cases?skill=skill-1&version=version-2"],
    ["版本與發佈", "/skills/skill-1/versions/version-2"],
  ]);
});

test("the workbench keeps version context visible but inert until a version is known", async () => {
  await act(async () => {
    root = createRoot(container);
    root.render(<SkillWorkspaceNav skillId="skill-1" />);
  });

  const nav = container.querySelector('nav[aria-label="這個 Skill 的工作台"]')!;
  expect(nav.querySelector('a[href*="/versions/"]')).toBeNull();
  const disabled = nav.querySelector("button[disabled]");
  const reasonId = disabled?.getAttribute("aria-describedby");
  expect(reasonId).toBeTruthy();
  expect(document.getElementById(reasonId!)?.textContent).toContain("先選定一個版本");
});
