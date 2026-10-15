import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";
import {
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const scopes = [
  { name: "workspace", url: "/lab/graders", path: "/evaluation/graders", grader: WORKSPACE_GRADER },
  {
    name: "platform",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    grader: PLATFORM_GRADER,
  },
];

for (const scope of scopes) {
  for (const status of [503, 408, 429, 0, 401, 403, 404]) {
    test(`${scope.name} grader read ${status} ${[401, 403, 404].includes(status) ? "removes the protected revision draft" : "preserves the revision draft through recovery"}`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let failing = false;
      let writes = 0;
      let reads = 0;
      await page.route(`**${scope.path}`, (route) => {
        if (route.request().resourceType() === "document") return route.fallback();
        reads += 1;
        if (failing && status === 0) return route.abort("failed");
        return failing
          ? route.fulfill({ status, json: { error: "read unavailable" } })
          : route.fulfill({ json: platformResponse(scope.path).body });
      });
      await page.route(`**${scope.path}/${scope.grader.id}/versions`, (route) => {
        writes += 1;
        return route.fulfill({ status: 503, json: { error: "not submitted" } });
      });
      await page.goto(scope.url);
      await page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true }).click();
      const instructions = page.locator(`#grader-${scope.grader.id}-instructions`);
      const reason = page.locator(`#grader-${scope.grader.id}-reason`);
      await instructions.fill("未儲存的評分指示");
      await reason.fill("未送出的修改原因");
      failing = true;
      await page.getByRole("button", { name: "重新整理", exact: true }).click();
      if (status === 401)
        await expect(page.getByText("評分器需要登入。", { exact: false })).toBeVisible();
      else
        await expect(
          page.getByRole("alert").filter({ hasText: "暫時無法讀取評分器" }),
        ).toBeVisible();
      if ([401, 403, 404].includes(status)) {
        await expect(instructions).toHaveCount(0);
        await expect(reason).toHaveCount(0);
        await expect(page.locator(".criterion-list")).toHaveCount(0);
        expect(writes).toBe(0);
        return;
      }
      const list = page.locator(".criterion-list");
      await expect(instructions).toHaveCount(1);
      await expect(list).toBeHidden();
      await expect(list).toHaveAttribute("inert", "");
      await expect(page.getByRole("button", { name: "存成新版本", exact: true })).toHaveCount(0);
      expect(writes).toBe(0);
      if (status === 503)
        await page.screenshot({
          path: testInfo.outputPath("draft-read-failed-phone.png"),
          fullPage: true,
        });
      failing = false;
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(instructions).toBeVisible();
      await expect(instructions).toHaveValue("未儲存的評分指示");
      await expect(reason).toHaveValue("未送出的修改原因");
      await expect(page.getByRole("button", { name: "存成新版本", exact: true })).toBeEnabled();
      await expect(page.locator("#grader-list-heading")).toBeFocused();
      expect(reads).toBe(3);
      expect(writes).toBe(0);
      const report = await new AxeBuilder({ page }).include("main").analyze();
      const path = testInfo.outputPath("recovered-accessibility.json");
      await writeFile(path, JSON.stringify(report));
      await testInfo.attach("recovered-accessibility", {
        path,
        contentType: "application/json",
      });
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      if (status === 503)
        await page.screenshot({
          path: testInfo.outputPath("draft-recovered-phone.png"),
          fullPage: true,
        });
    });
  }
}
