import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../src/testing/fixtures/platform";
import type { Grader } from "../src/features/evaluation/evaluation.service";
import { stubPlatform } from "./stub";

const SCOPES = [
  { name: "workspace", url: "/lab/graders", path: "/evaluation/graders", grader: WORKSPACE_GRADER },
  {
    name: "platform",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    grader: PLATFORM_GRADER,
  },
] as const;

async function open(page: Page, scope: (typeof SCOPES)[number], width: number) {
  await stubPlatform(page);
  await page.setViewportSize({ width, height: 900 });
  await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
  let latest: Grader = scope.grader;
  const posts: { instructions: string; reason: string }[] = [];
  await page.route(`**${scope.path}`, (route) => {
    const initial = platformResponse(scope.path).body as { graders: Grader[] };
    return route.fulfill({
      json: {
        graders: initial.graders.map((grader) => (grader.id === latest.id ? latest : grader)),
      },
    });
  });
  await page.route(`**${scope.path}/${scope.grader.id}/versions`, (route) => {
    posts.push(route.request().postDataJSON());
    latest = {
      ...latest,
      instructions: "已儲存並整理的評分指示",
      version: latest.version + 1,
      version_id: "22222222-2222-4222-8222-222222222222",
    };
    return route.fulfill({ status: 201, json: latest });
  });
  await page.goto(scope.url);
  await page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true }).click();
  const instructions = page.locator(`#grader-${scope.grader.id}-instructions`);
  const reason = page.locator(`#grader-${scope.grader.id}-reason`);
  await expect(instructions).toBeVisible();
  return {
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
      await page.keyboard.press("Enter");
      await expect(
        page.getByRole("heading", { name: `${latest.name} v${latest.version}`, exact: true }),
      ).toBeVisible();
      const shown = page.locator(".grader-instructions").filter({ hasText: latest.instructions });
      await expect(shown).toBeVisible();
      await expect(shown).toHaveText(latest.instructions);
    },
  };
}

async function inspect(page: Page, info: TestInfo) {
  const report = await new AxeBuilder({ page }).include(".grader-list").analyze();
  await writeFile(info.outputPath("grader-draft-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("grader-draft.png") });
}

for (const scope of SCOPES) {
  for (const width of [375, 1280]) {
    test(`${scope.name} refresh preserves unsent grader fields at ${width}px`, async ({
      page,
    }, info) => {
      const editor = await open(page, scope, width);
      await editor.instructions.fill("我還沒有送出的評分指示");
      await editor.reason.fill("我還沒有送出的變更原因");
      await editor.refresh();
      await expect(editor.instructions).toHaveValue("我還沒有送出的評分指示");
      await expect(editor.reason).toHaveValue("我還沒有送出的變更原因");
      await expect(page.getByRole("button", { name: "存成新版本", exact: true })).toBeEnabled();
      expect(editor.posts).toEqual([]);
      await editor.instructions.scrollIntoViewIfNeeded();
      await inspect(page, info);
    });

    test(`${scope.name} saved grader fields follow later versions at ${width}px`, async ({
      page,
    }, info) => {
      const editor = await open(page, scope, width);
      await editor.instructions.fill("我送出的評分指示");
      await editor.reason.fill("這一版的變更原因");
      await page.getByRole("button", { name: "存成新版本", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(page.locator("#grader-result")).toContainText("已儲存");
      await expect(editor.instructions).toHaveValue("已儲存並整理的評分指示");
      await expect(editor.reason).toHaveValue("");
      expect(editor.posts).toEqual([
        { instructions: "我送出的評分指示", reason: "這一版的變更原因" },
      ]);
      await editor.refresh();
      await expect(editor.instructions).toHaveValue("另一位使用者剛存的新指示");
      await expect(editor.reason).toHaveValue("");
      expect(editor.posts).toHaveLength(1);
      await editor.instructions.scrollIntoViewIfNeeded();
      await inspect(page, info);
    });
  }
}
