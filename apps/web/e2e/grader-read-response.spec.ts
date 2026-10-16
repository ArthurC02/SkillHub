import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ viewport: { width: 375, height: 900 }, trace: "retain-on-failure" });

async function inspect(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await page.screenshot({ path: test.info().outputPath(`${label}.png`), fullPage: true });
}

for (const scope of [
  {
    name: "workspace",
    url: "/lab/graders",
    path: "/evaluation/graders",
    grader: WORKSPACE_GRADER,
    heading: "評分器",
  },
  {
    name: "platform",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    grader: PLATFORM_GRADER,
    heading: "平台評分器",
  },
]) {
  for (const malformed of ["collection", "instructions"] as const) {
    for (const reread of [false, true]) {
      test(`${scope.name} graders with missing ${malformed} on ${reread ? "reread" : "first read"} recover locally`, async ({
        page,
      }) => {
        await stubPlatform(page);
        let reads = 0;
        let recovered = false;
        let documents = 0;
        const writes: string[] = [];
        page.on("domcontentloaded", () => documents++);
        page.on("request", (request) => {
          if (request.method() !== "GET") writes.push(request.url());
        });
        await page.route(`**${scope.path}`, (route) => {
          reads++;
          return route.fulfill({
            json:
              recovered || (reread && reads === 1)
                ? platformResponse(scope.path).body
                : malformed === "collection"
                  ? {}
                  : { graders: [{ ...scope.grader, instructions: null }] },
          });
        });
        await page.goto(scope.url);
        const instructions = page.locator(`#grader-${scope.grader.id}-instructions`);
        const reason = page.locator(`#grader-${scope.grader.id}-reason`);
        if (reread) {
          await page.locator("summary").filter({ hasText: "新增評分器" }).click();
          await page.locator("#grader-create-name").fill("未送出的評分器");
          await page.locator("#grader-create-instructions").fill("未送出的新指示");
          await page.locator("#grader-create-reason").fill("未送出的新增原因");
          await page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true }).click();
          await instructions.fill("未送出的修訂指示");
          await reason.fill("未送出的修訂原因");
          await page.getByRole("button", { name: "重新整理", exact: true }).click();
        }
        await expect(
          page.getByRole("alert").filter({ hasText: "暫時無法讀取評分器" }),
        ).toBeVisible();
        await expect(
          page.getByRole("heading", { level: 1, name: scope.heading, exact: true }),
        ).toBeVisible();
        await expect(page.getByRole("button", { name: "重新載入頁面", exact: true })).toHaveCount(
          0,
        );
        await expect(page.getByRole("heading", { level: 3 })).toHaveCount(0);
        await expect(page.getByText("還沒有評分器。", { exact: true })).toHaveCount(0);
        if (reread) {
          await expect(instructions).toHaveValue("未送出的修訂指示");
          await expect(page.locator(".grader-list")).toBeHidden();
          await expect(page.locator(".grader-list")).toHaveAttribute("inert", "");
          await expect(page.locator("#grader-create-name")).toHaveValue("未送出的評分器");
        }
        expect(reads).toBe(reread ? 2 : 1);
        expect(writes).toEqual([]);
        await inspect(page, "unreadable-graders");
        recovered = true;
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect(page.getByRole("alert")).toHaveCount(0);
        await expect(
          page.getByRole("heading", {
            name: `${scope.grader.name} v${scope.grader.version}`,
            exact: true,
          }),
        ).toBeVisible();
        await expect(page.locator("#grader-list-heading")).toBeFocused();
        if (reread) {
          await expect(instructions).toBeVisible();
          await expect(instructions).toHaveValue("未送出的修訂指示");
          await expect(reason).toHaveValue("未送出的修訂原因");
          await expect(page.locator("#grader-create-name")).toHaveValue("未送出的評分器");
          await expect(page.locator("#grader-create-instructions")).toHaveValue("未送出的新指示");
          await expect(page.locator("#grader-create-reason")).toHaveValue("未送出的新增原因");
        }
        expect(reads).toBe(reread ? 3 : 2);
        expect(documents).toBe(1);
        expect(writes).toEqual([]);
        await inspect(page, "recovered-graders");
      });
    }
  }
}
