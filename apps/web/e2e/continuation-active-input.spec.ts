import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, TRACE_GENERAL } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const target of ["header", "feedback"] as const) {
  test(`an uncertain continuation preserves ongoing ${target} input and reading position`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: target === "header" ? 375 : 1280, height: 900 });
    await page.emulateMedia({ colorScheme: target === "header" ? "light" : "dark" });
    await page.route(`**/runs/${RUN}/trace`, (route) =>
      route.fulfill({
        json: {
          ...TRACE_GENERAL,
          status: "succeeded",
          questions: [
            { question: "要處理哪個工作表？", why: "確認來源", options: ["Sheet1", "Sheet2"] },
          ],
        },
      }),
    );
    let release!: () => void;
    const response = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: unknown[] = [];
    await page.route(`**/runs/${RUN}/continuations`, async (route) => {
      writes.push(route.request().postDataJSON());
      await response;
      return route.abort("failed");
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(page.locator("#feedback-comment")).toBeVisible();
      const form = page.getByRole("region", { name: "這次試跑需要你回答", exact: true });
      await form.getByRole("radio", { name: "Sheet2", exact: true }).check();
      await form.getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
      await expect.poll(() => writes.length).toBe(1);
      const input =
        target === "header"
          ? page.getByPlaceholder("搜尋小工具或描述任務")
          : page.locator("#feedback-comment");
      await input.fill("保留輸入");
      const readingPosition = await page.evaluate(() => scrollY);
      release();
      const failure = form.getByRole("alert", { name: "回答送出結果", exact: true });
      await expect(failure).toContainText("無法確認回答是否已送出並開始下一次試跑");
      await expect(input).toBeFocused();
      await expect.poll(() => page.evaluate(() => scrollY)).toBe(readingPosition);
      await page.keyboard.type(" more");
      await expect(input).toHaveValue("保留輸入 more");
      await expect(failure).not.toBeFocused();
      await expect(form.getByRole("textbox", { name: "你的回答", exact: true })).toHaveValue(
        "Sheet2",
      );
      await expect(
        form.getByRole("button", { name: "核對後仍要重新送出回答", exact: true }),
      ).toBeEnabled();
      expect(writes).toEqual([{ answers: ["Sheet2"] }]);
      expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
      await page.screenshot({ path: test.info().outputPath("continuation-input.png") });
      await page.evaluate(() => scrollTo(0, 0));
      const report = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(
        test.info().outputPath("continuation-input-a11y.json"),
        JSON.stringify(report),
      );
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    } finally {
      release();
    }
  });
}
