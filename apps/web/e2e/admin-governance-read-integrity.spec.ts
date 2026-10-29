import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_SKILLS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const SKILL = ADMIN_SKILLS.skills[0];
const BAD: [string, unknown][] = [
  ["null", null],
  ["string list", { skills: "" }],
  ["missing takedown", { skills: [{ ...SKILL, takedown_at: undefined }] }],
];

for (const width of [375, 1280]) {
  for (const phase of ["initial", "refresh"]) {
    for (const [name, malformed] of BAD) {
      test(`governance ${name} on ${phase} stays unknown and recovers at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        let body: unknown = phase === "initial" ? malformed : ADMIN_SKILLS;
        let reads = 0;
        const writes: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        await page.route("**/admin/skills?*", (route) => {
          if (route.request().isNavigationRequest()) return route.fallback();
          reads++;
          return route.fulfill({ body: JSON.stringify(body), contentType: "application/json" });
        });
        await page.goto(`/admin/skills?q=${SKILL.skill_id}`);
        const refresh = page.getByRole("button", { name: "重新整理治理狀態", exact: true });
        if (phase === "refresh") {
          await expect(page.getByText("查到 1 筆小工具。", { exact: true })).toBeVisible();
          body = malformed;
          await refresh.click();
        }
        const failure = page.getByRole("alert").filter({ hasText: "暫時無法讀取小工具" });
        await expect(failure).toBeVisible();
        await expect(page.getByText("沒有符合", { exact: false })).toHaveCount(0);
        await expect(page.getByText("治理狀態上次取得於", { exact: false })).toHaveCount(0);
        await expect(page.getByRole("heading", { name: `對「${SKILL.name}」的動作` })).toHaveCount(
          0,
        );
        await expect(page.getByRole("status").filter({ hasText: "未下架" })).toHaveCount(0);
        await expect(page.locator("#admin-skill-lift-takedown")).toHaveCount(0);
        const audit = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(info.outputPath("governance-unreadable-a11y.json"), JSON.stringify(audit));
        expect(audit.violations).toEqual([]);
        expect(audit.incomplete).toEqual([]);
        await page.screenshot({
          path: info.outputPath("governance-unreadable.png"),
          fullPage: true,
        });
        body = { skills: [] };
        await refresh.focus();
        await refresh.press("Enter");
        await expect(
          page.getByText(`沒有符合「${SKILL.skill_id}」的小工具：0 筆。已清除的小工具不會出現。`, {
            exact: true,
          }),
        ).toBeVisible();
        await expect(failure).toHaveCount(0);
        expect(reads).toBe(phase === "initial" ? 2 : 3);
        expect(writes).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({
          path: info.outputPath("governance-recovered.png"),
          fullPage: true,
        });
      });
    }
  }

  test(`governance receipt survives malformed reread at ${width}px`, async ({ page }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    let changed = false;
    let writes = 0;
    await page.route(`**/admin/skills/${SKILL.skill_id}/redistribution`, (route) => {
      writes++;
      changed = true;
      return route.fulfill({ json: { skill_id: SKILL.skill_id, redistribution: "blocked" } });
    });
    await page.route("**/admin/skills?*", (route) => {
      if (route.request().isNavigationRequest()) return route.fallback();
      return route.fulfill({ json: changed ? { skills: "" } : ADMIN_SKILLS });
    });
    await page.goto(`/admin/skills?q=${SKILL.skill_id}`);
    await page.locator("#admin-skill-redistribution summary").click();
    await page.locator("#admin-redistribution-note").fill("禁止再散布");
    await page.locator("#admin-redistribution-value").selectOption("blocked");
    await page.getByRole("button", { name: "送出判定", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("暫時無法讀取小工具");
    await expect(page.locator("#admin-governance-result")).toHaveText(
      "「PDF Summariser」的再散布判定已改為「禁止再散布」。",
    );
    await expect(page.locator("#admin-governance-result")).toBeFocused();
    await expect(page.locator("#admin-skill-redistribution")).toHaveCount(0);
    expect(writes).toBe(1);
    const audit = await new AxeBuilder({ page }).include("main").analyze();
    await writeFile(info.outputPath("governance-receipt-a11y.json"), JSON.stringify(audit));
    expect(audit.violations).toEqual([]);
    expect(audit.incomplete).toEqual([]);
    await page.screenshot({ path: info.outputPath("governance-receipt.png"), fullPage: true });
  });
}
