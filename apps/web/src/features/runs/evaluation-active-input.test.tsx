import { expect, test } from "vitest";
import { RUN, SKILL, SUGGESTIONS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

for (const action of ["decision", "apply"] as const) {
  for (const target of ["header", "feedback"] as const) {
    test(`a failed ${action} outcome preserves ongoing ${target} input`, async () => {
      const page = app();
      page.route(`**/runs/${RUN}/suggestions`, () => ({
        json: {
          ...SUGGESTIONS,
          suggestions: [
            {
              ...SUGGESTIONS.suggestions[0],
              decision: action === "decision" ? "pending" : "accepted",
            },
          ],
        },
      }));
      let release!: () => void;
      const response = new Promise<void>((resolve) => {
        release = resolve;
      });
      const writes: unknown[] = [];
      page.route(
        action === "decision"
          ? "**/suggestions/s1/decision"
          : `**/skills/${SKILL}/versions/from-suggestions`,
        async ({ body }): Promise<"network-error"> => {
          writes.push(body);
          await response;
          return "network-error";
        },
      );
      try {
        await page.goto(`/runs/${RUN}`);
        await expect(page.locator(".auth-name")).toHaveText("tester");
        const item = page.locator("li.suggestion");
        if (action === "decision")
          await item.getByRole("button", { name: "接受", exact: true }).click();
        else await page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ }).click();
        await page.eventually(() => expect(writes).toHaveLength(1));
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
            ? page.locator('input[placeholder="搜尋小工具或描述任務"]')
            : page.locator("#feedback-comment");
        await input.fill("保留輸入");
        expect(await page.axeViolations("main")).toEqual([]);
        release();
        const alert =
          action === "decision"
            ? item.getByRole("alert")
            : page.getByRole("alert", { name: "建立版本結果", exact: true });
        await expect(alert).toContainText(
          action === "decision" ? "尚未確認「接受」是否已記錄" : "無法確認是否已建立新版本",
        );
        await expect(input).toBeFocused();
        await input.fill("保留輸入 more");
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
        expect(page.pathname()).toBe(`/runs/${RUN}`);
        expect(await page.axeViolations("main")).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
