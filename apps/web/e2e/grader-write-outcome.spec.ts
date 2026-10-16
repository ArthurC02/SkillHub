import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { writeFile } from "node:fs/promises";
import {
  ME,
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../src/testing/fixtures/platform";
import type { Grader } from "../src/features/evaluation/evaluation.service";
import { stubPlatform } from "./stub";

test.use({ viewport: { width: 375, height: 900 }, trace: "retain-on-failure" });

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

function fields(page: Page, scope: Scope, operation: Operation) {
  const prefix = operation === "create" ? "grader-create" : `grader-${scope.grader.id}`;
  const instructions = page.locator(`#${prefix}-instructions`);
  return {
    instructions,
    reason: page.locator(`#${prefix}-reason`),
    name: page.locator("#grader-create-name"),
    form: page.locator("form").filter({ has: instructions }),
  };
}

async function fillDraft(page: Page, scope: Scope, operation: Operation, draft: Draft) {
  const inputs = fields(page, scope, operation);
  if (operation === "create") await inputs.name.fill(draft.name);
  await inputs.instructions.fill(draft.instructions);
  await inputs.reason.fill(draft.reason);
}

async function inspect(page: Page, target: Locator, label: string) {
  await target.scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await page.screenshot({ path: test.info().outputPath(`${label}.png`) });
}

async function receiveWrites(
  scope: Scope,
  operation: Operation,
  outcome: Outcome,
  state: WriteState,
) {
  const receiver = createServer(async (request, response) => {
    const headers = {
      "Access-Control-Allow-Origin": request.headers.origin ?? "http://localhost:4173",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Allow-Headers": "accept, content-type",
      "Content-Type": "application/json",
    };
    if (request.method === "OPTIONS") {
      response.writeHead(204, headers).end();
      return;
    }
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const draft: Draft = JSON.parse(Buffer.concat(chunks).toString("utf8"));
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
    if (second) {
      response.writeHead(201, headers).end(JSON.stringify(state.latest));
      return;
    }
    if (outcome === "network") {
      response.writeHead(201, { ...headers, "Content-Length": "100" });
      response.write("{", () => response.destroy());
      return;
    }
    response
      .writeHead(outcome === "server" ? 503 : 201, headers)
      .end(outcome === "malformed" ? "{" : JSON.stringify(outcome === "null" ? null : {}));
  });
  await new Promise<void>((resolve, reject) => {
    receiver.once("error", reject);
    receiver.listen(0, "127.0.0.1", resolve);
  });
  return receiver;
}

async function prepare(
  page: Page,
  scope: Scope,
  operation: Operation,
  { endpoint, state }: { endpoint: string; state: WriteState },
) {
  await stubPlatform(page);
  await page.route(`**${scope.path}`, (route) => {
    if (operation === "create" && route.request().method() === "POST")
      return route.continue({ url: endpoint });
    const initial = (platformResponse(scope.path).body as { graders: Grader[] }).graders;
    const graders = !state.latest
      ? initial
      : operation === "create"
        ? [...initial, state.latest]
        : initial.map((grader) => (grader.id === scope.grader.id ? state.latest : grader));
    return route.fulfill({ json: { graders } });
  });
  if (operation === "revise")
    await page.route(`**${scope.path}/${scope.grader.id}/versions`, (route) =>
      route.continue({ url: endpoint }),
    );
  await page.goto(scope.url);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await expect(
    page.getByRole("heading", {
      name: `${scope.grader.name} v${scope.grader.version}`,
      exact: true,
    }),
  ).toBeVisible();
  if (operation === "create")
    await page
      .locator("details > summary")
      .filter({ hasText: /^新增評分器$/ })
      .click();
  else await page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true }).click();
  await fillDraft(page, scope, operation, FIRST);
  await fields(page, scope, operation)
    .form.getByRole("button", {
      name: operation === "create" ? "新增評分器" : "存成新版本",
      exact: true,
    })
    .click();
}

async function checkReadOnlyVerification(
  page: Page,
  scope: Scope,
  operation: Operation,
  state: WriteState,
) {
  const inputs = fields(page, scope, operation);
  await expect(inputs.form.getByRole("alert")).toContainText("儲存結果尚未確認");
  await expect(inputs.form.getByRole("alert")).toContainText("請先按「重新整理」核對現有的評分器");
  await expect(inputs.instructions).toHaveValue(FIRST.instructions);
  await expect(inputs.reason).toHaveValue(FIRST.reason);
  await expect(inputs.form.getByRole("alert")).toBeFocused();
  await expect(page.locator("#grader-result")).toHaveCount(0);
  await inspect(page, inputs.form.getByRole("alert"), "unknown-grader-write");
  await fillDraft(page, scope, operation, NEXT);
  await expect(inputs.form.getByRole("alert")).toContainText("儲存結果尚未確認");
  await expect(inputs.form.getByRole("alert")).toContainText(
    operation === "create" ? FIRST.name : scope.grader.name,
  );
  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: `${state.latest!.name} v${state.latest!.version}`,
      exact: true,
    }),
  ).toBeVisible();
  await expect(inputs.instructions).toHaveValue(NEXT.instructions);
  await expect(inputs.reason).toHaveValue(NEXT.reason);
  await expect(inputs.form.getByRole("alert")).toContainText("儲存結果尚未確認");
  expect(state.posts).toHaveLength(1);
}

for (const scope of SCOPES) {
  for (const operation of ["create", "revise"] as const) {
    const extra: Outcome =
      (scope.name === "workspace") === (operation === "create") ? "network" : "malformed";
    for (const outcome of ["null", "incomplete", "server", extra] as const) {
      test(`${scope.name} ${operation} preserves an unknown ${outcome} outcome through verification and explicit resend`, async ({
        page,
      }) => {
        await page.emulateMedia({
          colorScheme: outcome === "null" || outcome === "server" ? "light" : "dark",
        });
        const state: WriteState = { posts: [] };
        const receiver = await receiveWrites(scope, operation, outcome, state);
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
          const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/grader`;
          await prepare(page, scope, operation, { endpoint, state });
          await checkReadOnlyVerification(page, scope, operation, state);
          await page.route("**/me", async (route) => {
            if (blocked) {
              started();
              await gate;
            }
            return route.fulfill({ json: { ...ME, operator: scope.name === "platform" } });
          });
          blocked = true;
          await page.evaluate(() =>
            document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
          );
          await checking;
          const inputs = fields(page, scope, operation);
          const resend = inputs.form.getByRole("button", {
            name: operation === "create" ? "重新送出新增" : "重新送出新版本",
            exact: true,
          });
          await resend.click();
          await expect(
            inputs.form.getByRole("alert").filter({ hasText: "這次操作尚未送出" }),
          ).toBeVisible();
          await expect(
            inputs.form.getByRole("alert").filter({ hasText: "儲存結果尚未確認" }),
          ).toBeVisible();
          expect(state.posts).toHaveLength(1);
          await inspect(
            page,
            inputs.form.getByRole("alert").filter({ hasText: "這次操作尚未送出" }),
            "blocked-grader-resend",
          );
          blocked = false;
          release();
          await expect(page.locator(".auth-name")).toHaveText("tester");
          await resend.click();
          await expect(page.locator("#grader-result")).toContainText(
            `已儲存「${operation === "create" ? NEXT.name : scope.grader.name}」v${operation === "create" ? 1 : scope.grader.version + 2}`,
          );
          await expect(page.locator("#grader-result")).toBeFocused();
          await expect(inputs.form.getByRole("alert")).toHaveCount(0);
          expect(state.posts).toEqual(
            operation === "create"
              ? [FIRST, NEXT]
              : [
                  { instructions: FIRST.instructions, reason: FIRST.reason },
                  { instructions: NEXT.instructions, reason: NEXT.reason },
                ],
          );
          await inspect(page, page.locator("#grader-result"), "confirmed-grader-write");
        } finally {
          release();
          receiver.closeAllConnections();
          await new Promise<void>((resolve, reject) =>
            receiver.close((error) => (error ? reject(error) : resolve())),
          );
        }
      });
    }
  }
}
