import {
  EvaluationGraderDraftNameMaxLength,
  EvaluationGraderRevisionInstructionsMaxLength,
  SetEvaluationFeedbackBodyCommentMaxLength,
} from "@skillhub/api-client-ts/limits";
import { expect, test } from "vitest";
import {
  EVALUATION,
  PLATFORM_GRADER,
  RUN,
  WORKSPACE_GRADER,
} from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";
import type { Grader } from "./evaluation.service";

const app = renderedApp();

const scopes = [
  { name: "workspace", url: "/lab/graders", path: "/evaluation/graders", grader: WORKSPACE_GRADER },
  {
    name: "platform",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    grader: PLATFORM_GRADER,
  },
] as const;
type Scope = (typeof scopes)[number];
type Field =
  | { kind: "feedback"; name: string; maximum: number }
  | { kind: "name" | "instructions" | "revision"; name: string; maximum: number; scope: Scope };

const feedback: Field = {
  kind: "feedback",
  name: "feedback",
  maximum: SetEvaluationFeedbackBodyCommentMaxLength,
};
const fields: Field[] = [feedback];
for (const scope of scopes) {
  fields.push(
    {
      kind: "name",
      name: `${scope.name} create name`,
      maximum: EvaluationGraderDraftNameMaxLength,
      scope,
    },
    {
      kind: "instructions",
      name: `${scope.name} create instructions`,
      maximum: EvaluationGraderRevisionInstructionsMaxLength,
      scope,
    },
    {
      kind: "revision",
      name: `${scope.name} revision instructions`,
      maximum: EvaluationGraderRevisionInstructionsMaxLength,
      scope,
    },
  );
}

function controls(page: App, field: Field, helpful: boolean) {
  if (field.kind === "feedback") {
    const form = page.locator(".evaluation-feedback");
    return {
      input: page.locator("#feedback-comment"),
      save: form.locator("button").filter({ hasText: helpful ? "有幫助" : "沒幫助" }),
      selector: ".evaluation-feedback",
      receipt: '.evaluation-feedback > p[role="status"]',
    };
  }
  const prefix = field.kind === "revision" ? `grader-${field.scope.grader.id}` : "grader-create";
  const selector = `form:has(#${prefix}-instructions)`;
  return {
    input: page.locator(`#${prefix}-${field.kind === "name" ? "name" : "instructions"}`),
    save: page.locator(selector).locator('button[type="submit"]'),
    selector,
    receipt: "#grader-result",
  };
}

async function prepare(page: App, field: Field) {
  if (field.kind === "feedback") {
    await page.goto(`/runs/${RUN}`);
    return;
  }
  await page.goto(field.scope.url);
  if (field.kind === "revision") {
    const summary = page.getByText(`修改「${field.scope.grader.name}」的評分指示`, {
      exact: true,
    });
    await page.eventually(() => expect(summary.count()).toBe(1));
    await summary.click();
    await page.locator(`#grader-${field.scope.grader.id}-reason`).fill("調整評分依據");
  } else {
    const summary = page.locator("details > summary").filter({ hasText: /^新增評分器$/ });
    await page.eventually(() => expect(summary.count()).toBe(1));
    await summary.click();
    await page.locator("#grader-create-name").fill("新評分器");
    await page.locator("#grader-create-instructions").fill("新的評分指示");
    await page.locator("#grader-create-reason").fill("調整評分依據");
  }
}

function expectedBody(field: Field, value: string, helpful: boolean) {
  if (field.kind === "feedback") return { helpful, comment: value };
  if (field.kind === "revision") return { instructions: value, reason: "調整評分依據" };
  return {
    name: field.kind === "name" ? value : "新評分器",
    instructions: field.kind === "instructions" ? value : "新的評分指示",
    reason: "調整評分依據",
  };
}

function receiveWrites(page: App, field: Field) {
  const writes: { method: string; body: unknown }[] = [];
  let reads = 0;
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  const received = new Promise<void>((resolve) => {
    started = resolve;
  });
  if (field.kind === "feedback") {
    let saved = EVALUATION.feedback;
    page.route(`**/runs/${RUN}/evaluation`, () => {
      reads++;
      return { json: { ...EVALUATION, feedback: saved } };
    });
    page.route(`**/runs/${RUN}/evaluation/feedback`, async ({ method, body }) => {
      writes.push({ method, body });
      started();
      await held;
      saved = { ...(body as object), submitted_at: "2026-10-22T00:00:00Z" } as typeof saved;
      return { json: { ...EVALUATION, feedback: saved } };
    });
  } else {
    const scope = field.scope;
    let saved: Grader | undefined;
    const receive = async ({ method, body }: { method: string; body: unknown }) => {
      const sent = body as { name?: string; instructions: string };
      writes.push({ method, body });
      started();
      await held;
      saved = {
        ...scope.grader,
        scope: scope.name,
        id: field.kind === "revision" ? scope.grader.id : "55555555-5555-4555-8555-555555555555",
        version_id: "66666666-6666-4666-8666-666666666666",
        version: field.kind === "revision" ? scope.grader.version + 1 : 1,
        name: sent.name ?? scope.grader.name,
        instructions: sent.instructions,
        revised_at: "2026-10-22T00:00:00Z",
      };
      return { status: 201, json: saved };
    };
    page.route(`**${scope.path}`, (request) => {
      if (request.method !== "GET") return receive(request);
      reads++;
      const initial =
        scope.name === "platform" ? [PLATFORM_GRADER] : [PLATFORM_GRADER, WORKSPACE_GRADER];
      const current = saved;
      const graders = !current
        ? initial
        : field.kind === "revision"
          ? initial.map((grader) => (grader.id === current.id ? current : grader))
          : [...initial, current];
      return { json: { graders } };
    });
    page.route(`**${scope.path}/${scope.grader.id}/versions`, receive);
  }
  return { writes, readCount: () => reads, release, received };
}

const editable = (input: HTMLElement) =>
  !(input as HTMLTextAreaElement).readOnly && !(input as HTMLTextAreaElement).disabled;

const scenarios = [
  ...fields.flatMap((field) => ["at limit", "over limit"].map((mode) => ({ field, mode }))),
  { field: feedback, mode: "optional clear" },
];
for (const { field, mode } of scenarios) {
  for (const width of [375, 1280]) {
    test(`${field.name} keeps Unicode ${mode} at ${width}px`, async () => {
      const page = app();
      const state = receiveWrites(page, field);
      const helpful = width === 375;
      const { input, save, selector, receipt } = controls(page, field, helpful);
      const requested = "😀".repeat(field.maximum + (mode === "over limit" ? 1 : 0));
      try {
        await prepare(page, field);
        await page.eventually(() => expect(editable(input.element())).toBe(true));
        await input.fill(requested);
        await page.eventually(() => {
          expect(input.value()).toBe(requested);
          expect(input.attribute("aria-invalid")).toBe(String(mode === "over limit"));
        });
        const hintId = input.attribute("aria-describedby");
        expect(hintId).toBeTruthy();
        const hint = page.locator(`#${hintId}`);
        expect(hint.text()).toContain(`最多 ${field.maximum} 個字`);
        if (mode === "over limit") {
          expect(hint.text()).toContain(`目前 ${field.maximum + 1} 個字`);
          expect(save.isDisabled()).toBe(true);
          expect(save.attribute("aria-describedby")).toBe(hintId);
          expect(state.writes).toEqual([]);
        } else {
          expect(save.isDisabled()).toBe(false);
        }
        if (mode === "optional clear") {
          await input.fill("");
          expect(input.value()).toBe("");
        }
        await input.click();
        expect(input.isFocused()).toBe(true);
        expect(await page.axeViolations(selector)).toEqual([]);
        let value = mode === "optional clear" ? "" : requested;
        if (mode === "over limit") {
          value = "😀".repeat(field.maximum);
          await input.fill(value);
          expect(input.value()).toBe(value);
        }
        await page.eventually(() => {
          expect(input.attribute("aria-invalid")).toBe("false");
          expect(save.isDisabled()).toBe(false);
        });
        await save.click();
        await state.received;
        await page.eventually(() => {
          expect(save.isDisabled()).toBe(true);
          expect(editable(input.element())).toBe(false);
        });
        expect(state.writes).toEqual([
          {
            method: field.kind === "feedback" ? "PUT" : "POST",
            body: expectedBody(field, value, helpful),
          },
        ]);
        state.release();
        await page.eventually(() => {
          expect(page.locator(receipt).text()).toContain(
            field.kind === "feedback" ? "已送出回饋。" : "已儲存「",
          );
          expect(state.readCount()).toBe(2);
          expect(editable(input.element())).toBe(true);
          expect(input.value()).toBe(
            field.kind === "feedback" || field.kind === "revision" ? value : "",
          );
        });
        expect(await page.axeViolations(receipt)).toEqual([]);
      } finally {
        state.release();
      }
    });
  }
}
