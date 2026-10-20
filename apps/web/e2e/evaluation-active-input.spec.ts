import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, SKILL, SUGGESTIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function capture(page: Page, state: string) {
  await page.screenshot({ path: test.info().outputPath(state + "-evaluation-input.png") });
  await page.evaluate(() => scrollTo(0, 0));
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(
    test.info().outputPath(state + "-evaluation-input-a11y.json"),
    JSON.stringify(report),
  );
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
}

for (const action of ["decision", "apply"] as const) {
  for (const target of ["header", "feedback"] as const) {
    test(`a failed ${action} outcome preserves ongoing ${target} input`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: target === "header" ? 375 : 1280, height: 900 });
      await page.emulateMedia({ colorScheme: target === "header" ? "light" : "dark" });
      await page.route(`**/runs/${RUN}/suggestions`, (route) =>
        route.fulfill({
          json: {
            ...SUGGESTIONS,
            suggestions: [
              {
                ...SUGGESTIONS.suggestions[0],
                decision: action === "decision" ? "pending" : "accepted",
              },
            ],
          },
        }),
      );
      let release!: () => void;
      const response = new Promise<void>((resolve) => {
        release = resolve;
      });
      const writes: unknown[] = [];
      await page.route(
        action === "decision"
          ? "**/suggestions/s1/decision"
          : `**/skills/${SKILL}/versions/from-suggestions`,
        async (route) => {
          writes.push(route.request().postDataJSON());
          await response;
          return route.abort("failed");
        },
      );
      try {
        await page.goto(`/runs/${RUN}`);
        await expect(page.locator(".auth-name")).toHaveText("tester");
        const item = page.locator("li.suggestion");
        if (action === "decision")
          await item.getByRole("button", { name: "接受", exact: true }).click();
        else await page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ }).click();
        await expect.poll(() => writes.length).toBe(1);
        if (action === "apply") {
          await expect(
            page.getByRole("status").filter({ hasText: "正在建立新版本…" }),
          ).toBeVisible();
          await expect(
            page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ }),
          ).toHaveAccessibleDescription("正在建立新版本…");
        }
        const input =
          target === "header"
            ? page.getByPlaceholder("搜尋小工具或描述任務")
            : page.locator("#feedback-comment");
        await input.fill("保留輸入");
        await capture(page, "pending");
        release();
        const alert =
          action === "decision"
            ? item.getByRole("alert")
            : page.getByRole("alert", { name: "建立版本結果", exact: true });
        await expect(alert).toContainText(
          action === "decision" ? "尚未確認「接受」是否已記錄" : "無法確認是否已建立新版本",
        );
        await expect(input).toBeFocused();
        await page.keyboard.type(" more");
        await expect(input).toHaveValue("保留輸入 more");
        await expect(alert).not.toBeFocused();
        if (action === "decision") {
          await expect(item).toContainText("上次讀取：尚未決定");
          await expect(
            page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ }),
          ).toBeDisabled();
          expect(writes).toEqual([{ decision: "accepted" }]);
        } else {
          await expect(
            page.getByRole("button", { name: "重新確認建議套用狀態", exact: true }),
          ).toBeVisible();
          expect(writes).toEqual([{ evaluation_id: "eval-2", suggestion_ids: ["s1"] }]);
        }
        expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
        await capture(page, "failed");
      } finally {
        release();
      }
    });
  }
}
