import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { IMPORT_LIMITS, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import { setOffline } from "./offline";

test.use({ viewport: { width: 375, height: 900 }, trace: "retain-on-failure" });

const RULES = {
  ...IMPORT_LIMITS,
  max_zip_bytes: 6 << 20,
  max_unpacked_bytes: 48 << 20,
  max_files: 37,
  max_file_bytes: 2 << 20,
  max_path_depth: 5,
};

async function inspect(page: Page, label: string) {
  await page.getByRole("heading", { name: "上傳新版本", exact: true }).scrollIntoViewIfNeeded();
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`${label}.png`) });
}

for (const colorScheme of ["light", "dark"] as const) {
  test(`an offline version upload withholds cached limits and preserves the file in ${colorScheme} mode`, async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme });
    await stubPlatform(page);
    await page.route("**/skills/import/limits", (route) => route.fulfill({ json: RULES }));
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    await expect(page.getByText(/平台限制：zip 最大 6.0 MB/)).toBeVisible();
    await page.getByRole("link", { name: "Skill Hub", exact: true }).click();
    await expect(page.getByRole("heading", { name: "上傳新版本", exact: true })).toHaveCount(0);
    try {
      await setOffline(page, true);
      await page.goBack();
      await expect(page.getByText("等待連線以讀取上傳限制…", { exact: true })).toBeVisible();
      const input = page.getByLabel("小工具 zip", { exact: true });
      await input.setInputFiles({
        name: "offline.zip",
        mimeType: "application/zip",
        buffer: Buffer.from("offline draft"),
      });
      await expect(page.getByText(/平台限制：zip 最大/)).toHaveCount(0);
      await expect(page.getByRole("button", { name: "上傳成新版本", exact: true })).toBeDisabled();
      await inspect(page, "paused-upload-limits");
      await setOffline(page, false);
      await expect(page.getByRole("button", { name: "上傳成新版本", exact: true })).toBeEnabled();
      await expect(page.getByText(/平台限制：zip 最大 6.0 MB/)).toBeVisible();
      await expect(input).toHaveValue(/(?:^|[\\/])offline\.zip$/);
      await inspect(page, "resumed-upload-limits");
    } finally {
      await setOffline(page, false);
    }
  });
}

for (const fault of ["server", "unreadable limits"] as const) {
  for (const reread of [false, true]) {
    for (const colorScheme of ["light", "dark"] as const) {
      test(`version upload retains its file after ${fault} on ${reread ? "reread" : "first read"} in ${colorScheme} mode`, async ({
        page,
      }) => {
        await page.emulateMedia({ colorScheme });
        await stubPlatform(page);
        let fail = !reread;
        let reads = 0;
        let documents = 0;
        const writes: string[] = [];
        page.on("domcontentloaded", () => documents++);
        page.on("request", (request) => {
          if (request.method() !== "GET") writes.push(request.url());
        });
        await page.route("**/skills/import/limits", (route) => {
          reads++;
          return route.fulfill({
            status: fail && fault === "server" ? 503 : 200,
            json: fail
              ? fault === "server"
                ? { error: "service unavailable" }
                : { ...RULES, max_zip_bytes: null }
              : RULES,
          });
        });
        await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
        const section = page.locator("section").filter({
          has: page.getByRole("heading", { name: "上傳新版本", exact: true }),
        });
        const input = page.getByLabel("小工具 zip", { exact: true });
        const submit = page.getByRole("button", { name: "上傳成新版本", exact: true });
        if (reread) {
          await expect(section).toContainText("zip 最大 6.0 MB");
          await expect(submit).toBeEnabled();
          fail = true;
          await page.getByRole("link", { name: "Skill Hub", exact: true }).click();
          await expect(page.getByRole("heading", { name: "上傳新版本", exact: true })).toHaveCount(
            0,
          );
          await page.goBack();
          await expect.poll(() => reads).toBe(2);
        }
        await input.setInputFiles({
          name: "draft.zip",
          mimeType: "application/zip",
          buffer: Buffer.from("retained draft"),
        });
        await expect(section.getByRole("alert")).toContainText("暫時無法讀取上傳限制");
        await expect(submit).toBeDisabled();
        await expect(submit).toHaveAttribute("aria-describedby", /.+/);
        await expect(section).not.toContainText("zip 最大");
        await expect(input).toHaveValue(/(?:^|[\\/])draft\.zip$/);
        await inspect(page, "unconfirmed-upload-limits");
        fail = false;
        await section.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect(submit).toBeEnabled();
        await expect(section.getByRole("heading", { name: "上傳新版本" })).toBeFocused();
        await expect(section).toContainText("平台限制：zip 最大 6.0 MB");
        await expect(section).toContainText("解壓後總量最大 48.0 MB");
        await expect(section).toContainText("最多 37 個檔案");
        await expect(section).toContainText("單一檔案最大 2.0 MB");
        await expect(section).toContainText("路徑最深 5 層");
        await expect(input).toHaveValue(/(?:^|[\\/])draft\.zip$/);
        expect(reads).toBe(reread ? 3 : 2);
        expect(documents).toBe(1);
        expect(writes).toEqual([]);
        await inspect(page, "confirmed-upload-limits");
      });
    }
  }
}
