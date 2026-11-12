import { expect, test } from "vitest";
import { RUN, TRACE_GENERAL } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

for (const target of ["header", "feedback"] as const) {
  test(`an uncertain continuation preserves ongoing ${target} input`, async () => {
    const page = app();
    page.route(`**/runs/${RUN}/trace`, () => ({
      json: {
        ...TRACE_GENERAL,
        status: "succeeded",
        questions: [
          { question: "要處理哪個工作表？", why: "確認來源", options: ["Sheet1", "Sheet2"] },
        ],
      },
    }));
    let release!: () => void;
    const response = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: unknown[] = [];
    page.route(`**/runs/${RUN}/continuations`, async ({ body }): Promise<"network-error"> => {
      writes.push(body);
      await response;
      return "network-error";
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(page.locator("#feedback-comment")).toBeVisible();
      const form = page.getByRole("region", { name: "這次試跑需要你回答", exact: true });
      await form.getByRole("radio", { name: "Sheet2", exact: true }).check();
      await form.getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
      await page.eventually(() => expect(writes).toHaveLength(1));
      const input =
        target === "header"
          ? page.locator('input[placeholder="搜尋小工具或描述任務"]')
          : page.locator("#feedback-comment");
      await input.fill("保留輸入");
      release();
      const failure = form.getByRole("alert", { name: "回答送出結果", exact: true });
      await expect(failure).toContainText("無法確認回答是否已送出並開始下一次試跑");
      await expect(input).toBeFocused();
      await expect(failure).not.toBeFocused();
      await input.fill("保留輸入 more");
      await expect(input).toHaveValue("保留輸入 more");
      await expect(form.getByRole("textbox", { name: "你的回答", exact: true })).toHaveValue(
        "Sheet2",
      );
      await expect(
        form.getByRole("button", { name: "核對後仍要重新送出回答", exact: true }),
      ).toBeEnabled();
      expect(writes).toEqual([{ answers: ["Sheet2"] }]);
      expect(page.pathname()).toBe(`/runs/${RUN}`);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      release();
    }
  });
}
