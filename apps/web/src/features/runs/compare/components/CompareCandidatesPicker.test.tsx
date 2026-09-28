import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { RunListItem } from "../../runs.service";
import { CompareCandidatesPicker } from "./CompareCandidatesPicker";

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

const BASE = {
  selfPending: false,
  selfError: null,
  testCaseId: "tc-1",
  siblingsPending: false,
  siblingsError: null,
  candidates: [] as RunListItem[],
  draft: "",
  onDraftChange: () => {},
  onPick: () => {},
};

async function mount(props: Partial<typeof BASE>) {
  root = createRoot(container);
  await act(async () => {
    root.render(
      <StrictMode>
        <CompareCandidatesPicker {...BASE} {...props} />
      </StrictMode>,
    );
  });
}

const text = () => container.textContent ?? "";

test("shows loading for the current run before its own load settles", async () => {
  await mount({ selfPending: true });
  expect(text()).toContain("載入目前這次 Run");
});

test("says the test case can no longer be resolved when the current run has none", async () => {
  await mount({ testCaseId: undefined });
  expect(text()).toContain("這次 Run 的 Test Case 已無法解析");
});

test("says there is nothing to compare when the test case has no other runs", async () => {
  await mount({ candidates: [] });
  expect(text()).toContain("這個 Test Case 目前只有這一次 Run");
});

test("lists each candidate run with a button to pick it", async () => {
  const onPick = vi.fn();
  const candidate: RunListItem = {
    run_id: "run-2",
    status: "succeeded",
    evaluation: { value: "met", label: "符合", note: "" },
    skill_id: "skill-1",
    skill_name: "Skill",
    skill_version_id: "v1",
    provider: "sandbox",
    cleanup_status: { value: "done", label: "已清理", note: "" },
    created_at: "2026-01-01T00:00:00Z",
  };
  await mount({ candidates: [candidate], onPick });

  const button = container.querySelector("button")!;
  expect(button.textContent).toContain("與這一次比較");
  await act(async () => button.click());
  expect(onPick).toHaveBeenCalledWith("run-2");
});
