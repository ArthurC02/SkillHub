import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_ROSTERS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const MALFORMED: [string, unknown][] = [
  ["null", null],
  ["string allowlist", { operator_user_ids: [], beta_allowlist: "" }],
  ["invalid operator", { operator_user_ids: ["u-1"], beta_allowlist: [] }],
];

for (const width of [375, 1280]) {
  for (const phase of ["initial", "refresh"]) {
    for (const [name, malformed] of MALFORMED) {
      test(`rosters with ${name} on ${phase} recover without inventing membership at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        let current: unknown =
          phase === "initial"
            ? malformed
            : { ...ADMIN_ROSTERS, beta_allowlist: ["provider|invited-user"] };
        let reads = 0;
        const writes: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        await page.route("**/admin/rosters", (route) => {
          if (route.request().isNavigationRequest()) return route.fallback();
          reads++;
          return route.fulfill({ body: JSON.stringify(current), contentType: "application/json" });
        });
        await page.goto("/admin/rosters");
        if (phase === "refresh") {
          await expect(page.getByText("目前 1 位。", { exact: true })).toBeVisible();
          await expect(page.getByText("provider|invited-user", { exact: true })).toBeVisible();
          current = malformed;
          await page.getByRole("button", { name: "重新整理", exact: true }).click();
        }
        const failure = page.getByRole("alert").filter({ hasText: "暫時無法讀取名冊" });
        await expect(failure).toBeVisible();
        await expect(failure).toContainText(
          phase === "initial" ? "請稍後再試。" : "先前載入的名冊已隱藏。",
        );
        await expect(page.getByRole("heading", { name: "封測准入", exact: true })).toHaveCount(0);
        await expect(page.getByRole("heading", { name: "operator 名冊", exact: true })).toHaveCount(
          0,
        );
        await expect(page.getByText("每一個登入的帳號都算受邀", { exact: false })).toHaveCount(0);
        await expect(page.getByText("provider|invited-user", { exact: true })).toHaveCount(0);
        await expect(page.getByText("目前 0 位。", { exact: true })).toHaveCount(0);
        const report = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(info.outputPath("rosters-unreadable-a11y.json"), JSON.stringify(report));
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        await page.screenshot({ path: info.outputPath("rosters-unreadable.png") });
        current = { operator_user_ids: [], beta_allowlist: [] };
        const retry = page.getByRole("button", { name: "再試一次", exact: true });
        await retry.focus();
        await expect(retry).toBeFocused();
        await retry.press("Enter");
        await expect(page.getByText("每一個登入的帳號都算受邀", { exact: false })).toBeVisible();
        await expect(page.getByText("目前 0 位。", { exact: true })).toBeVisible();
        await expect(failure).toHaveCount(0);
        expect(reads).toBe(phase === "initial" ? 2 : 3);
        expect(writes).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({ path: info.outputPath("rosters-recovered.png") });
      });
    }
  }
}
