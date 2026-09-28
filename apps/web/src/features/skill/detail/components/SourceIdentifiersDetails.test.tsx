import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
import type { SkillSource } from "../../../../core/api/types";
import { SourceIdentifiersDetails } from "./SourceIdentifiersDetails";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
});

async function mount(node: React.ReactElement) {
  root = createRoot(container);
  await act(async () => {
    root.render(<StrictMode>{node}</StrictMode>);
  });
}

const BASE: SkillSource = {
  type: "git",
  trust: { value: "traceable", label: "來源可追溯", note: "" },
};

test("renders nothing when there is no identifier and no successful availability check to show", async () => {
  await mount(<SourceIdentifiersDetails source={BASE} />);

  expect(container.querySelector("details")).toBeNull();
});

test("shows the details block when only the source version is known", async () => {
  await mount(<SourceIdentifiersDetails source={{ ...BASE, source_version: "abc123" }} />);

  expect(container.querySelector("details")).not.toBeNull();
  expect(container.textContent).toContain("abc123");
});

test("shows the details block when only a successful availability check is known", async () => {
  await mount(
    <SourceIdentifiersDetails source={{ ...BASE, last_checked_at: "2026-09-20T10:00:00Z" }} />,
  );

  expect(container.querySelector("details")).not.toBeNull();
  expect(container.textContent).toContain("最近一次來源可用性檢查");
});

test("hides the last-checked line, and the whole block if it was the only identifier, when the source is currently unavailable", async () => {
  await mount(
    <SourceIdentifiersDetails
      source={{
        ...BASE,
        last_checked_at: "2026-09-20T10:00:00Z",
        unavailable_since: "2026-09-01T10:00:00Z",
      }}
    />,
  );

  expect(container.querySelector("details")).toBeNull();
});
