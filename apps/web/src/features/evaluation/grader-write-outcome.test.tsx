import { act } from "react";
import { expect, test, vi } from "vitest";
import {
  ME,
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../../testing/fixtures/platform";
import { Locator, renderedApp, type App } from "../../testing/app";
import type { Grader } from "./evaluation.service";

const app = renderedApp();

const SCOPES = [
  { name: "workspace", url: "/lab/graders", path: "/evaluation/graders", grader: WORKSPACE_GRADER },
  {
    name: "platform",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    grader: PLATFORM_GRADER,
  },
] as const;
type Scope = (typeof SCOPES)[number];
type Operation = "create" | "revise";
type Outcome = "null" | "incomplete" | "server" | "network" | "malformed";
type Draft = { name: string; instructions: string; reason: string };
type WriteState = { posts: Omit<Draft, "name">[]; latest?: Grader };

const FIRST = { name: "n".repeat(100), instructions: "i".repeat(100), reason: "第一次的變更原因" };
const NEXT = {
  name: "下一份評分器",
  instructions: "下一份未儲存的評分指示",
  reason: "下一份變更原因",
};

function fields(page: App, scope: Scope, operation: Operation) {
  const prefix = operation === "create" ? "grader-create" : `grader-${scope.grader.id}`;
  const instructions = page.locator(`#${prefix}-instructions`);
  const form = new Locator(() => {
    const owner = instructions.all()[0]?.closest("form");
    return owner ? [owner] : [];
  });
  return {
    instructions,
    reason: page.locator(`#${prefix}-reason`),
    name: page.locator("#grader-create-name"),
    form,
    alert: form.locator("[role=alert]"),
    button: (label: string) => form.locator("button").filter({ hasText: label }),
  };
}

async function fillDraft(page: App, scope: Scope, operation: Operation, draft: Draft) {
  const inputs = fields(page, scope, operation);
  if (operation === "create") await inputs.name.fill(draft.name);
  await inputs.instructions.fill(draft.instructions);
  await inputs.reason.fill(draft.reason);
}

function jsonText(status: number, text: string) {
  return new Response(text, { status, headers: { "Content-Type": "application/json" } });
}

function firstAnswer(outcome: Outcome) {
  if (outcome === "network") {
    const truncated = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("{"));
        controller.error(new TypeError("terminated"));
      },
    });
    return new Response(truncated, {
      status: 201,
      headers: { "Content-Type": "application/json" },
    });
  }
  if (outcome === "server") return jsonText(503, "{}");
  if (outcome === "malformed") return jsonText(201, "{");
  return jsonText(201, outcome === "null" ? "null" : "{}");
}

function receiveWrites(scope: Scope, operation: Operation, outcome: Outcome, state: WriteState) {
  const target = operation === "create" ? scope.path : `${scope.path}/${scope.grader.id}/versions`;
  const harnessFetch = globalThis.fetch;
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    const { pathname } = new URL(String(input), "http://127.0.0.1");
    const isWrite = (init?.method ?? "GET").toUpperCase() === "POST" && pathname.endsWith(target);
    if (!isWrite) return harnessFetch(input, init);
    const draft: Draft = JSON.parse(String(init?.body));
    state.posts.push(draft);
    const second = state.posts.length > 1;
    state.latest = {
      ...scope.grader,
      id:
        operation === "revise"
          ? scope.grader.id
          : second
            ? "33333333-3333-4333-8333-333333333333"
            : "44444444-4444-4444-8444-444444444444",
      name: draft.name ?? scope.grader.name,
      instructions: draft.instructions,
      version: operation === "create" ? 1 : scope.grader.version + state.posts.length,
      version_id: second
        ? "55555555-5555-4555-8555-555555555555"
        : "66666666-6666-4666-8666-666666666666",
    };
    return second ? jsonText(201, JSON.stringify(state.latest)) : firstAnswer(outcome);
  });
}

async function prepare(page: App, scope: Scope, operation: Operation, state: WriteState) {
  page.route(
    (url) => url.pathname.endsWith(scope.path),
    ({ method }) => {
      if (method !== "GET") return undefined;
      const initial = (platformResponse(scope.path).body as { graders: Grader[] }).graders;
      const latest = state.latest;
      const graders = !latest
        ? initial
        : operation === "create"
          ? [...initial, latest]
          : initial.map((grader) => (grader.id === scope.grader.id ? latest : grader));
      return { json: { graders } };
    },
  );
  await page.goto(scope.url);
  await page.eventually(() => {
    expect(page.locator(".auth-name").text()).toBe("tester");
    expect(
      page
        .getByRole("heading", {
          name: `${scope.grader.name} v${scope.grader.version}`,
          exact: true,
        })
        .count(),
    ).toBe(1);
  });
  if (operation === "create")
    await page
      .locator("details > summary")
      .filter({ hasText: /^新增評分器$/ })
      .click();
  else await page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true }).click();
  await fillDraft(page, scope, operation, FIRST);
  const inputs = fields(page, scope, operation);
  await expect(inputs.button(operation === "create" ? "新增評分器" : "存成新版本")).toBeEnabled();
  await inputs.reason.press("Enter");
}

async function checkReadOnlyVerification(
  page: App,
  scope: Scope,
  operation: Operation,
  state: WriteState,
) {
  const inputs = fields(page, scope, operation);
  await page.eventually(() => {
    expect(inputs.alert.text()).toContain("儲存結果尚未確認");
    expect(inputs.alert.text()).toContain("請先按「重新整理」核對現有的評分器");
    expect(inputs.instructions.value()).toBe(FIRST.instructions);
    expect(inputs.reason.value()).toBe(FIRST.reason);
    expect(inputs.alert.isFocused()).toBe(true);
    expect(page.locator("#grader-result").count()).toBe(0);
  });
  expect(await page.axeViolations("main")).toEqual([]);
  await fillDraft(page, scope, operation, NEXT);
  await page.eventually(() => {
    expect(inputs.alert.text()).toContain("儲存結果尚未確認");
    expect(inputs.alert.text()).toContain(operation === "create" ? FIRST.name : scope.grader.name);
  });
  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  await page.eventually(() => {
    expect(
      page
        .getByRole("heading", {
          name: `${state.latest!.name} v${state.latest!.version}`,
          exact: true,
        })
        .count(),
    ).toBe(1);
    expect(inputs.instructions.value()).toBe(NEXT.instructions);
    expect(inputs.reason.value()).toBe(NEXT.reason);
    expect(inputs.alert.text()).toContain("儲存結果尚未確認");
  });
  expect(state.posts).toHaveLength(1);
}

for (const scope of SCOPES) {
  for (const operation of ["create", "revise"] as const) {
    const extra: Outcome =
      (scope.name === "workspace") === (operation === "create") ? "network" : "malformed";
    for (const outcome of ["null", "incomplete", "server", extra] as const) {
      test(`${scope.name} ${operation} preserves an unknown ${outcome} outcome through verification and explicit resend`, async () => {
        const page = app();
        const state: WriteState = { posts: [] };
        receiveWrites(scope, operation, outcome, state);
        let blocked = false;
        let release!: () => void;
        let started!: () => void;
        const gate = new Promise<void>((resolve) => {
          release = resolve;
        });
        const checking = new Promise<void>((resolve) => {
          started = resolve;
        });
        try {
          await prepare(page, scope, operation, state);
          await checkReadOnlyVerification(page, scope, operation, state);
          page.route("**/me", async () => {
            if (blocked) {
              started();
              await gate;
            }
            return { json: { ...ME, operator: scope.name === "platform" } };
          });
          blocked = true;
          await act(async () => {
            document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
          });
          await checking;
          const inputs = fields(page, scope, operation);
          const resend = inputs.button(operation === "create" ? "重新送出新增" : "重新送出新版本");
          await resend.click();
          const notSent = inputs.alert.filter({ hasText: "這次操作尚未送出" });
          await page.eventually(() => {
            expect(notSent.count()).toBe(1);
            expect(inputs.alert.filter({ hasText: "儲存結果尚未確認" }).count()).toBe(1);
          });
          expect(state.posts).toHaveLength(1);
          expect(await page.axeViolations("main")).toEqual([]);
          blocked = false;
          release();
          await expect(page.locator(".auth-name")).toHaveText("tester");
          await resend.click();
          await page.eventually(() => {
            expect(page.locator("#grader-result").text()).toContain(
              `已儲存「${operation === "create" ? NEXT.name : scope.grader.name}」v${operation === "create" ? 1 : scope.grader.version + 2}`,
            );
            expect(page.locator("#grader-result").isFocused()).toBe(true);
            expect(inputs.alert.count()).toBe(0);
          });
          expect(state.posts).toEqual(
            operation === "create"
              ? [FIRST, NEXT]
              : [
                  { instructions: FIRST.instructions, reason: FIRST.reason },
                  { instructions: NEXT.instructions, reason: NEXT.reason },
                ],
          );
          expect(await page.axeViolations("main")).toEqual([]);
        } finally {
          release();
        }
      });
    }
  }
}
