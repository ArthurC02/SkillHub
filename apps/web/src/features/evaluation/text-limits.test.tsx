import {
  EvaluationGraderDraftNameMaxLength,
  EvaluationGraderRevisionInstructionsMaxLength,
  SetEvaluationFeedbackBodyCommentMaxLength,
} from "@skillhub/api-client-ts/limits";
import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import {
  EVALUATION,
  ME,
  PLATFORM_GRADER,
  RUN,
  WORKSPACE_GRADER,
} from "../../testing/fixtures/platform";
import { FeedbackForm } from "../runs/evaluation/components/FeedbackForm";
import type { Evaluation } from "../runs/evaluation.service";
import { GraderManager } from "./components/GraderManager";
import type { Grader } from "./evaluation.service";

let container: HTMLDivElement;
let root: Root;
let writes: { path: string; method: string; body: Record<string, unknown> }[];

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  queryClient.clear();
  writes = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  queryClient.clear();
  container.remove();
  vi.unstubAllGlobals();
});

const fields = [
  {
    name: "feedback",
    selector: "#feedback-comment",
    maximum: SetEvaluationFeedbackBodyCommentMaxLength,
  },
  {
    name: "create name",
    selector: "#grader-create-name",
    maximum: EvaluationGraderDraftNameMaxLength,
  },
  {
    name: "create instructions",
    selector: "#grader-create-instructions",
    maximum: EvaluationGraderRevisionInstructionsMaxLength,
  },
  {
    name: "revision instructions",
    selector: `#grader-${WORKSPACE_GRADER.id}-instructions`,
    maximum: EvaluationGraderRevisionInstructionsMaxLength,
  },
] as const;
type Field = (typeof fields)[number];

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

function stubPlatform(field: Field) {
  let saved: Grader | undefined;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input).replace(/^https?:\/\/[^/]+/, "");
    if (init?.method === "POST" || init?.method === "PUT") {
      const body = JSON.parse(init.body as string);
      writes.push({ path, method: init.method, body });
      if (field.name === "feedback") {
        return json({ ...EVALUATION, feedback: { ...body, submitted_at: "2026-10-22T00:00:00Z" } });
      }
      saved = {
        ...WORKSPACE_GRADER,
        id:
          field.name === "revision instructions"
            ? WORKSPACE_GRADER.id
            : "55555555-5555-4555-8555-555555555555",
        version_id: "66666666-6666-4666-8666-666666666666",
        version: field.name === "revision instructions" ? WORKSPACE_GRADER.version + 1 : 1,
        name: body.name ?? WORKSPACE_GRADER.name,
        instructions: body.instructions,
        revised_at: "2026-10-22T00:00:00Z",
      };
      return json(saved, 201);
    }
    if (path === "/me") return json(ME);
    const graders = [PLATFORM_GRADER, WORKSPACE_GRADER];
    if (saved) {
      if (field.name === "revision instructions") graders[1] = saved;
      else graders.push(saved);
    }
    return json({ graders });
  });
}

function input(selector: string) {
  const node = container.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector);
  if (!node) throw new Error(`Missing ${selector}`);
  return node;
}

async function edit(selector: string, value: string) {
  const node = input(selector);
  const prototype =
    node instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(prototype, "value")!.set!.call(node, value);
    node.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function render(node: ReactNode) {
  await act(async () => {
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
}

async function prepare(field: Field) {
  stubPlatform(field);
  if (field.name === "feedback") {
    await render(
      <FeedbackForm runId={RUN} evaluation={EVALUATION as Evaluation} disabled={false} />,
    );
    return;
  }
  await render(<GraderManager scope="workspace" />);
  await pollUntil(
    () => container.querySelector(`#grader-${WORKSPACE_GRADER.id}-instructions`) !== null,
    () => container.textContent,
    DEFAULT_WAIT_MS,
  );
  if (field.name === "revision instructions") {
    await edit(`#grader-${WORKSPACE_GRADER.id}-reason`, "調整評分依據");
  } else {
    await edit("#grader-create-name", "新評分器");
    await edit("#grader-create-instructions", "新的評分指示");
    await edit("#grader-create-reason", "調整評分依據");
  }
}

function saveButton(field: Field, helpful: boolean) {
  if (field.name !== "feedback") {
    return input(field.selector)
      .closest("form")!
      .querySelector<HTMLButtonElement>('button[type="submit"]')!;
  }
  return Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent === (helpful ? "有幫助" : "沒幫助"),
  )!;
}

function expectedWrite(field: Field, value: string, helpful: boolean) {
  if (field.name === "feedback")
    return {
      path: `/runs/${RUN}/evaluation/feedback`,
      method: "PUT",
      body: { helpful, comment: value },
    };
  if (field.name === "revision instructions")
    return {
      path: `/evaluation/graders/${WORKSPACE_GRADER.id}/versions`,
      method: "POST",
      body: { instructions: value, reason: "調整評分依據" },
    };
  return {
    path: "/evaluation/graders",
    method: "POST",
    body: {
      name: field.name === "create name" ? value : "新評分器",
      instructions: field.name === "create instructions" ? value : "新的評分指示",
      reason: "調整評分依據",
    },
  };
}

async function checkField(
  field: Field,
  {
    value,
    count,
    over,
    helpful,
  }: { value: string; count: number; over: boolean; helpful: boolean },
) {
  await edit(field.selector, value);
  const node = input(field.selector);
  expect(node.value).toBe(value);
  expect(node.hasAttribute("maxlength")).toBe(false);
  expect(node.getAttribute("aria-invalid")).toBe(String(over));
  const description = node.getAttribute("aria-describedby");
  expect(description).toBeTruthy();
  const hint = container.querySelector(`#${description}`)!;
  expect(hint.textContent).toContain(`最多 ${field.maximum} 個字`);
  if (over) expect(hint.textContent).toContain(`目前 ${count} 個字`);
  const save = saveButton(field, helpful);
  expect(save.disabled).toBe(over);
  if (over) expect(save.getAttribute("aria-describedby")).toBe(description);
  await act(async () => save.click());
  if (over) {
    expect(writes).toEqual([]);
    expect(node.value).toBe(value);
  } else {
    await pollUntil(
      () =>
        container.textContent!.includes(field.name === "feedback" ? "已送出回饋。" : "已儲存「"),
      () => container.textContent,
      DEFAULT_WAIT_MS,
    );
    expect(writes).toEqual([expectedWrite(field, value, helpful)]);
  }
}

const lengthCases = fields.flatMap((field) =>
  (field.name === "feedback" ? [true, false] : [true]).flatMap((helpful) =>
    ["a", "😀"].flatMap((character) =>
      [-1, 0, 1].map((delta) => ({
        field,
        helpful,
        character,
        delta,
        name: `${field.name} preserves and validates ${character} at max${delta >= 0 ? "+" : ""}${delta}, helpful=${helpful}`,
      })),
    ),
  ),
);

for (const { field, helpful, character, delta, name } of lengthCases) {
  test(name, async () => {
    await prepare(field);
    const count = field.maximum + delta;
    await checkField(field, { value: character.repeat(count), count, over: delta > 0, helpful });
  });
}

test.each([true, false])(
  "feedback can clear a previous optional comment, helpful=%s",
  async (helpful) => {
    await prepare(fields[0]);
    expect(input("#feedback-comment").value).toBe(EVALUATION.feedback.comment);
    await checkField(fields[0], { value: "", count: 0, over: false, helpful });
  },
);
