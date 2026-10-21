import {
  EvaluationGraderDraftNameMaxLength,
  EvaluationGraderRevisionInstructionsMaxLength,
  SetEvaluationFeedbackBodyCommentMaxLength,
} from "@skillhub/api-client-ts/limits";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type Route, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  EVALUATION,
  PLATFORM_GRADER,
  RUN,
  WORKSPACE_GRADER,
} from "../src/testing/fixtures/platform";
import type { Grader } from "../src/features/evaluation/evaluation.service";
import { stubPlatform } from "./stub";

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

function controls(page: Page, field: Field, helpful: boolean) {
  if (field.kind === "feedback") {
    const form = page.locator(".evaluation-feedback");
    return {
      input: page.locator("#feedback-comment"),
      form,
      save: form.getByRole("button", { name: helpful ? "有幫助" : "沒幫助", exact: true }),
      selector: ".evaluation-feedback",
      receipt: '.evaluation-feedback > p[role="status"]',
    };
  }
  const prefix = field.kind === "revision" ? `grader-${field.scope.grader.id}` : "grader-create";
  const selector = `form:has(#${prefix}-instructions)`;
  const form = page.locator(selector);
  return {
    input: page.locator(`#${prefix}-${field.kind === "name" ? "name" : "instructions"}`),
    form,
    save: form.locator('button[type="submit"]'),
    selector,
    receipt: "#grader-result",
  };
}

async function prepare(page: Page, field: Field) {
  if (field.kind === "feedback") {
    await page.goto(`/runs/${RUN}`);
    return;
  }
  await page.goto(field.scope.url);
  if (field.kind === "revision") {
    await page.getByText(`修改「${field.scope.grader.name}」的評分指示`, { exact: true }).click();
    await page.locator(`#grader-${field.scope.grader.id}-reason`).fill("調整評分依據");
  } else {
    await page
      .locator("details > summary")
      .filter({ hasText: /^新增評分器$/ })
      .click();
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

async function receiveWrites(page: Page, field: Field) {
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
    await page.route(`**/runs/${RUN}/evaluation`, (route) => {
      reads++;
      return route.fulfill({ json: { ...EVALUATION, feedback: saved } });
    });
    await page.route(`**/runs/${RUN}/evaluation/feedback`, async (route) => {
      expect(route.request().method()).toBe("PUT");
      const body = route.request().postDataJSON();
      writes.push({ method: "PUT", body });
      started();
      await held;
      saved = { ...body, submitted_at: "2026-10-22T00:00:00Z" };
      return route.fulfill({ json: { ...EVALUATION, feedback: saved } });
    });
  } else {
    const scope = field.scope;
    let saved: Grader | undefined;
    await page.route(`**${scope.path}`, async (route) => {
      if (route.request().method() !== "GET") {
        expect(field.kind).not.toBe("revision");
        return receive(route);
      }
      reads++;
      const initial =
        scope.name === "platform" ? [PLATFORM_GRADER] : [PLATFORM_GRADER, WORKSPACE_GRADER];
      const graders = !saved
        ? initial
        : field.kind === "revision"
          ? initial.map((grader) => (grader.id === saved!.id ? saved : grader))
          : [...initial, saved];
      return route.fulfill({ json: { graders } });
    });
    await page.route(`**${scope.path}/${scope.grader.id}/versions`, receive);
    async function receive(route: Route) {
      expect(route.request().method()).toBe("POST");
      const body = route.request().postDataJSON();
      writes.push({ method: "POST", body });
      started();
      await held;
      saved = {
        ...scope.grader,
        scope: scope.name,
        id: field.kind === "revision" ? scope.grader.id : "55555555-5555-4555-8555-555555555555",
        version_id: "66666666-6666-4666-8666-666666666666",
        version: field.kind === "revision" ? scope.grader.version + 1 : 1,
        name: body.name ?? scope.grader.name,
        instructions: body.instructions,
        revised_at: "2026-10-22T00:00:00Z",
      };
      return route.fulfill({ status: 201, json: saved });
    }
  }
  return { writes, readCount: () => reads, release, received };
}

async function inspect(page: Page, info: TestInfo, selector: string, label: string) {
  const report = await new AxeBuilder({ page }).include(selector).analyze();
  await writeFile(info.outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${label}.png`) });
}

const scenarios = [
  ...fields.flatMap((field) => ["at limit", "over limit"].map((mode) => ({ field, mode }))),
  { field: feedback, mode: "optional clear" },
];
for (const { field, mode } of scenarios) {
  for (const width of [375, 1280]) {
    test(`${field.name} keeps Unicode ${mode} at ${width}px`, async ({ page }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const state = await receiveWrites(page, field);
      const helpful = width === 375;
      const { input, save, selector, receipt } = controls(page, field, helpful);
      const requested = "😀".repeat(field.maximum + (mode === "over limit" ? 1 : 0));
      try {
        await prepare(page, field);
        await expect(input).toBeEditable();
        await input.click();
        await page.keyboard.press("Control+A");
        await page.keyboard.insertText(requested);
        await expect(input).toHaveValue(requested);
        await expect(input).toHaveAttribute("aria-invalid", String(mode === "over limit"));
        const hintId = await input.getAttribute("aria-describedby");
        expect(hintId).toBeTruthy();
        const hint = page.locator(`#${hintId}`);
        await expect(hint).toContainText(`最多 ${field.maximum} 個字`);
        if (mode === "over limit") {
          await expect(hint).toContainText(`目前 ${field.maximum + 1} 個字`);
          await expect(save).toBeDisabled();
          await expect(save).toHaveAttribute("aria-describedby", hintId!);
          expect(state.writes).toEqual([]);
        } else {
          await expect(save).toBeEnabled();
        }
        if (mode === "optional clear") {
          await page.keyboard.press("Control+A");
          await page.keyboard.press("Backspace");
          await expect(input).toHaveValue("");
        }
        await input.click();
        await expect(input).toBeFocused();
        await expect(input).toBeInViewport({ ratio: 1 });
        await inspect(page, info, selector, "evaluation-input");
        let value = mode === "optional clear" ? "" : requested;
        if (mode === "over limit") {
          await input.click();
          await page.keyboard.press("Backspace");
          value = "😀".repeat(field.maximum);
          await expect(input).toHaveValue(value);
        }
        await expect(input).toHaveAttribute("aria-invalid", "false");
        await expect(save).toBeEnabled();
        await save.click();
        await state.received;
        await expect(save).toBeDisabled();
        await expect(input).not.toBeEditable();
        expect(state.writes).toEqual([
          {
            method: field.kind === "feedback" ? "PUT" : "POST",
            body: expectedBody(field, value, helpful),
          },
        ]);
        state.release();
        await expect(page.locator(receipt)).toContainText(
          field.kind === "feedback" ? "已送出回饋。" : "已儲存「",
        );
        await expect.poll(state.readCount).toBe(2);
        await expect(input).toBeEditable();
        if (field.kind === "feedback" || field.kind === "revision")
          await expect(input).toHaveValue(value);
        else await expect(input).toHaveValue("");
        await inspect(page, info, receipt, "evaluation-receipt");
      } finally {
        state.release();
      }
    });
  }
}
