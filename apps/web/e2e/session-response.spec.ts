import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ME, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const RECEIPT = {
  shape: "skill",
  skills: [
    {
      path: "",
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 1,
      content_hash: "a".repeat(64),
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
};

for (const [name, reply] of [
  ["null", null],
  ["empty object", {}],
  ["string operator", { ...ME, operator: "false" }],
  ["object display name", { ...ME, display_name: { label: "tester" } }],
] as const) {
  for (const phase of ["initial", "foreground", "failed logout"] as const) {
    test(`a ${phase} session with ${name} stays unconfirmed until a valid reread`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 375, height: 900 });
      await stubPlatform(page);
      let invalid = phase === "initial";
      let documents = 0;
      let logouts = 0;
      const imports: string[] = [];
      page.on("request", (request) => {
        if (request.resourceType() === "document") documents += 1;
      });
      await page.route("**/me", (route) => route.fulfill({ json: invalid ? reply : ME }));
      await page.route("**/auth/logout", (route) => {
        logouts += 1;
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      });
      await page.route("**/skills/import/url", (route) => {
        imports.push(route.request().postData()!);
        return route.fulfill({ status: 201, json: RECEIPT });
      });
      await page.goto("/workspace/import");
      const input = page.getByLabel("URL", { exact: true });
      const submit = page.getByRole("button", { name: "開始匯入", exact: true });
      if (phase !== "initial") {
        await expect(page.locator(".auth-name")).toHaveText("tester");
        await expect(submit).toBeEnabled();
      } else {
        await expect(
          page.getByRole("button", { name: "重新確認登入狀態", exact: true }),
        ).toBeVisible();
      }
      await input.fill("https://github.com/example/unsent");
      if (phase !== "initial") {
        invalid = true;
        if (phase === "failed logout")
          await page.getByRole("button", { name: "登出", exact: true }).click();
        else
          await page.evaluate(() =>
            document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
          );
      }
      const retry = page.getByRole("button", { name: "重新確認登入狀態", exact: true });
      await expect(retry).toBeVisible();
      await expect(page.locator("header").getByRole("alert")).toContainText(
        phase === "failed logout" ? "未能確認登出結果" : "登入狀態暫時無法確認",
      );
      await expect(
        page.getByRole("heading", { level: 1, name: "匯入小工具", exact: true }),
      ).toBeVisible();
      await expect(submit).toBeDisabled();
      await expect(submit).toHaveAccessibleDescription(
        "登入狀態尚未確認；請先在頁首確認，再送出匯入。",
      );
      await expect(
        page.locator("header").getByRole("link", { name: "後台", exact: true }),
      ).toHaveCount(0);
      await page
        .locator(".import-source form")
        .evaluate((form: HTMLFormElement) => form.requestSubmit());
      expect(imports).toEqual([]);
      await expect(input).toHaveValue("https://github.com/example/unsent");
      expect(documents).toBe(1);
      expect(logouts).toBe(phase === "failed logout" ? 1 : 0);
      await page.evaluate(() => scrollTo(0, 0));
      const report = await new AxeBuilder({ page })
        .include("main")
        .include(".app-header")
        .analyze();
      await writeFile(test.info().outputPath("session-response-a11y.json"), JSON.stringify(report));
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await page.screenshot({
        path: test.info().outputPath("session-unconfirmed-phone.png"),
        fullPage: true,
      });
      invalid = false;
      await retry.click();
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(retry).toHaveCount(0);
      await expect(submit).toBeEnabled();
      await expect(input).toHaveValue("https://github.com/example/unsent");
      expect(documents).toBe(1);
      expect(imports).toEqual([]);
      await submit.click();
      await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
      expect(imports).toEqual(['{"url":"https://github.com/example/unsent"}']);
      await expect(
        page.getByRole("link", { name: "開啟 v1 並繼續驗證", exact: true }),
      ).toHaveAttribute("href", `/skills/${SKILL}/versions/${VERSION}`);
    });
  }
}
