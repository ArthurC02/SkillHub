import { expect, test } from "vitest";
import {
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const scopes = [
  { name: "workspace", url: "/lab/graders", path: "/evaluation/graders", grader: WORKSPACE_GRADER },
  {
    name: "platform",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    grader: PLATFORM_GRADER,
  },
];

const present = (selector: string) => document.querySelectorAll(selector).length;

for (const scope of scopes) {
  for (const status of [503, 408, 429, 0, 401, 403, 404]) {
    test(`${scope.name} grader read ${status} ${[401, 403, 404].includes(status) ? "removes the protected revision draft" : "preserves the revision draft through recovery"}`, async () => {
      const page = app();
      let failing = false;
      let writes = 0;
      let reads = 0;
      page.route(`**${scope.path}`, () => {
        reads += 1;
        if (failing && status === 0) return "network-error";
        return failing
          ? { status, json: { error: "read unavailable" } }
          : { json: platformResponse(scope.path).body };
      });
      page.route(`**${scope.path}/${scope.grader.id}/versions`, () => {
        writes += 1;
        return { status: 503, json: { error: "not submitted" } };
      });
      await page.goto(scope.url);
      const summary = page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true });
      await expect(summary).toHaveCount(1);
      await summary.click();
      const instructionsSelector = `#grader-${scope.grader.id}-instructions`;
      const reasonSelector = `#grader-${scope.grader.id}-reason`;
      const instructions = page.locator(instructionsSelector);
      const reason = page.locator(reasonSelector);
      await instructions.fill("未儲存的評分指示");
      await reason.fill("未送出的修改原因");
      failing = true;
      await page.getByRole("button", { name: "重新整理", exact: true }).click();
      if (status === 401) await expect(page.getByText("評分器需要登入。")).toBeVisible();
      else
        await expect(
          page.getByRole("alert").filter({ hasText: "暫時無法讀取評分器" }),
        ).toBeVisible();
      if ([401, 403, 404].includes(status)) {
        expect(present(instructionsSelector)).toBe(0);
        expect(present(reasonSelector)).toBe(0);
        expect(present(".criterion-list")).toBe(0);
        expect(writes).toBe(0);
        return;
      }
      const list = document.querySelector(".criterion-list");
      expect(present(instructionsSelector)).toBe(1);
      expect(list?.hasAttribute("hidden")).toBe(true);
      expect(list?.getAttribute("inert")).toBe("");
      await expect(page.getByRole("button", { name: "存成新版本", exact: true })).toHaveCount(0);
      expect(writes).toBe(0);
      failing = false;
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(instructions.value()).toBe("未儲存的評分指示");
        expect(reason.value()).toBe("未送出的修改原因");
        expect(page.getByRole("button", { name: "存成新版本", exact: true }).isDisabled()).toBe(
          false,
        );
        expect(page.locator("#grader-list-heading").isFocused()).toBe(true);
      });
      expect(reads).toBe(3);
      expect(writes).toBe(0);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}
