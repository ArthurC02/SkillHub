import { expect, test } from "vitest";
import {
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";
import type { Grader } from "../evaluation.service";

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

async function open(scope: (typeof SCOPES)[number]) {
  const page = app();
  let latest: Grader = scope.grader;
  const posts: unknown[] = [];
  page.route(`**${scope.path}`, () => {
    const initial = platformResponse(scope.path).body as { graders: Grader[] };
    return {
      json: {
        graders: initial.graders.map((grader) => (grader.id === latest.id ? latest : grader)),
      },
    };
  });
  page.route(`**${scope.path}/${scope.grader.id}/versions`, ({ body }) => {
    posts.push(body);
    latest = {
      ...latest,
      instructions: "已儲存並整理的評分指示",
      version: latest.version + 1,
      version_id: "22222222-2222-4222-8222-222222222222",
    };
    return { status: 201, json: latest };
  });
  await page.goto(scope.url);
  const summary = page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true });
  await expect(summary).toHaveCount(1);
  await summary.click();
  const instructions = page.locator(`#grader-${scope.grader.id}-instructions`);
  const reason = page.locator(`#grader-${scope.grader.id}-reason`);
  await expect(instructions).toHaveCount(1);
  return {
    page,
    instructions,
    reason,
    posts,
    async refresh() {
      latest = {
        ...latest,
        instructions: "另一位使用者剛存的新指示",
        version: latest.version + 1,
        version_id: "33333333-3333-4333-8333-333333333333",
      };
      await page.getByRole("button", { name: "重新整理", exact: true }).focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(
          page
            .getByRole("heading", { name: `${latest.name} v${latest.version}`, exact: true })
            .count(),
        ).toBeGreaterThan(0);
        expect(
          page.locator(".grader-instructions").filter({ hasText: latest.instructions }).text(),
        ).toBe(latest.instructions);
      });
    },
  };
}

for (const scope of SCOPES) {
  for (const width of [375, 1280]) {
    test(`${scope.name} refresh preserves unsent grader fields at ${width}px`, async () => {
      const editor = await open(scope);
      await editor.instructions.fill("我還沒有送出的評分指示");
      await editor.reason.fill("我還沒有送出的變更原因");
      await editor.refresh();
      await expect(editor.instructions).toHaveValue("我還沒有送出的評分指示");
      await expect(editor.reason).toHaveValue("我還沒有送出的變更原因");
      await expect(
        editor.page.getByRole("button", { name: "存成新版本", exact: true }),
      ).toBeEnabled();
      expect(editor.posts).toEqual([]);
      expect(await editor.page.axeViolations(".grader-list")).toEqual([]);
    });

    test(`${scope.name} saved grader fields follow later versions at ${width}px`, async () => {
      const editor = await open(scope);
      await editor.instructions.fill("我送出的評分指示");
      await editor.reason.fill("這一版的變更原因");
      await editor.page.getByRole("button", { name: "存成新版本", exact: true }).focus();
      await editor.page.press("Enter");
      await editor.page.eventually(() => {
        expect(editor.page.locator("#grader-result").text()).toContain("已儲存");
        expect(editor.instructions.value()).toBe("已儲存並整理的評分指示");
        expect(editor.reason.value()).toBe("");
      });
      expect(editor.posts).toEqual([
        { instructions: "我送出的評分指示", reason: "這一版的變更原因" },
      ]);
      await editor.refresh();
      await editor.page.eventually(() => {
        expect(editor.instructions.value()).toBe("另一位使用者剛存的新指示");
        expect(editor.reason.value()).toBe("");
      });
      expect(editor.posts).toHaveLength(1);
      expect(await editor.page.axeViolations(".grader-list")).toEqual([]);
    });
  }
}
