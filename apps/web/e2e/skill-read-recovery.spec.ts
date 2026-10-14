import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { SKILL, SKILL_VERSIONS, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const status of [403, 503]) {
  test(`file read ${status} withdraws cached package contents and recovers at the same skill`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let failing = false;
    let reads = 0;
    await page.route(`**/api/skills/${SKILL}/files`, (route) => {
      reads += 1;
      return failing
        ? route.fulfill({ status, json: { error: "read unavailable" } })
        : route.fulfill({ json: platformResponse(route.request().url()).body });
    });
    await page.goto(`/skills/${SKILL}/files`);
    await expect(page.locator(".skill-files-context")).toBeVisible();
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    failing = true;
    await page.goBack();
    await expect(
      page.getByText(status === 403 ? "這份套件的檔案目前尚未開放查看" : "儲存的套件目前讀不到", {
        exact: false,
      }),
    ).toBeVisible();
    await expect(page.locator(".skill-files-context")).toHaveCount(0);
    await expect(page.locator(".skill-files-layout")).toHaveCount(0);
    const before = reads;
    failing = false;
    await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(page.locator(".skill-files-context")).toBeVisible();
    await expect(page.locator("#skill-files-heading")).toBeFocused();
    expect(reads).toBe(before + 1);
    expect(new URL(page.url()).pathname).toBe(`/skills/${SKILL}/files`);
  });

  test(`version choices ${status} hide cached names without losing the selected snapshot or package inclusion`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let failing = false;
    let reads = 0;
    const selectedVersion = SKILL_VERSIONS.versions[1].version_id;
    await page.route(`**/skills/${SKILL}/versions`, (route) => {
      reads += 1;
      return failing
        ? route.fulfill({ status, json: { error: "read unavailable" } })
        : route.fulfill({ json: SKILL_VERSIONS });
    });
    await page.goto(`/skills/${SKILL}/package?version=${selectedVersion}`);
    const disclosure = page.getByText("換一個版本打包，或看這個版本的識別碼", { exact: true });
    await disclosure.click();
    const picker = page.getByLabel("小工具 Version", { exact: true });
    await expect(picker.locator("option")).toHaveCount(SKILL_VERSIONS.versions.length);
    await expect(page.getByText("正在打包 v1", { exact: false })).toBeVisible();
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    failing = true;
    await page.goBack();
    await disclosure.click();
    await expect(page.getByRole("alert").filter({ hasText: "版本清單" })).toBeVisible();
    await expect(picker).toBeDisabled();
    await expect(picker.locator("option")).toHaveCount(1);
    await expect(picker).toHaveValue(selectedVersion);
    await expect(page.getByText("正在打包 所選版本", { exact: false })).toBeVisible();
    await expect(picker.locator("option")).toContainText("清單尚未確認");
    await page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }).check();
    const before = reads;
    failing = false;
    await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(picker).toBeEnabled();
    await expect(picker.locator("option")).toHaveCount(SKILL_VERSIONS.versions.length);
    await expect(picker).toHaveValue(selectedVersion);
    await expect(page.getByRole("group", { name: "小工具版本選擇", exact: true })).toBeFocused();
    await expect(
      page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }),
    ).toBeChecked();
    expect(reads).toBe(before + 1);
    expect(new URL(page.url()).searchParams.get("version")).toBe(selectedVersion);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath("version-picker-recovered-phone.png"),
      fullPage: true,
    });
  });
}
