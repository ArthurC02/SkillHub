import { expect, test } from "vitest";
import { SKILL, SKILL_VERSIONS, platformResponse } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

for (const status of [403, 503]) {
  test(`file read ${status} withdraws cached package contents and recovers at the same skill`, async () => {
    const page = app();
    let failing = false;
    let reads = 0;
    page.route(`**/api/skills/${SKILL}/files`, (request) => {
      reads += 1;
      return failing
        ? { status, json: { error: "read unavailable" } }
        : { json: platformResponse(request.url.pathname + request.url.search).body };
    });
    await page.goto(`/skills/${SKILL}/files`);
    await expect(page.locator(".skill-files-context")).toHaveCount(1);
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
    failing = true;
    await page.goBack();
    await page.eventually(() => {
      expect(
        page
          .getByText(status === 403 ? "這份套件的檔案目前尚未開放查看" : "儲存的套件目前讀不到")
          .count(),
      ).toBeGreaterThan(0);
      expect(page.locator(".skill-files-context").count()).toBe(0);
      expect(page.locator(".skill-files-layout").count()).toBe(0);
    });
    const before = reads;
    failing = false;
    const retry = page.getByRole("button", { name: "再試一次", exact: true });
    await expect(retry).toHaveCount(1);
    await retry.click();
    await page.eventually(() => {
      expect(page.locator(".skill-files-context").count()).toBe(1);
      expect(page.locator("#skill-files-heading").isFocused()).toBe(true);
    });
    expect(reads).toBe(before + 1);
    expect(page.pathname()).toBe(`/skills/${SKILL}/files`);
  });

  test(`version choices ${status} hide cached names without losing the selected snapshot or package inclusion`, async () => {
    const page = app();
    let failing = false;
    let reads = 0;
    const selectedVersion = SKILL_VERSIONS.versions[1].version_id;
    page.route(`**/skills/${SKILL}/versions`, () => {
      reads += 1;
      return failing ? { status, json: { error: "read unavailable" } } : { json: SKILL_VERSIONS };
    });
    await page.goto(`/skills/${SKILL}/package?version=${selectedVersion}`);
    const disclosure = page.getByText("換一個版本打包，或看這個版本的識別碼", { exact: true });
    await expect(disclosure).toHaveCount(1);
    await disclosure.click();
    const picker = page.getByLabel("小工具 Version", { exact: true });
    const inclusion = page.getByRole("checkbox", {
      name: "包含可散布的測試題與範例資料",
      exact: true,
    });
    await page.eventually(() => {
      expect(picker.locator("option").count()).toBe(SKILL_VERSIONS.versions.length);
      expect(page.getByText("正在打包 v1").count()).toBeGreaterThan(0);
    });
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
    failing = true;
    await page.goBack();
    await expect(disclosure).toHaveCount(1);
    await disclosure.click();
    await page.eventually(() => {
      expect(page.getByRole("alert").filter({ hasText: "版本清單" }).count()).toBe(1);
      expect(picker.isDisabled()).toBe(true);
      expect(picker.locator("option").count()).toBe(1);
      expect(picker.value()).toBe(selectedVersion);
      expect(page.getByText("正在打包 所選版本").count()).toBeGreaterThan(0);
      expect(picker.locator("option").text()).toContain("清單尚未確認");
    });
    await inclusion.check();
    const before = reads;
    failing = false;
    const retry = page.getByRole("button", { name: "再試一次", exact: true });
    await expect(retry).toHaveCount(1);
    await retry.click();
    await page.eventually(() => {
      expect(picker.isDisabled()).toBe(false);
      expect(picker.locator("option").count()).toBe(SKILL_VERSIONS.versions.length);
      expect(picker.value()).toBe(selectedVersion);
      expect(page.getByRole("group", { name: "小工具版本選擇", exact: true }).isFocused()).toBe(
        true,
      );
      expect((inclusion.element() as HTMLInputElement).checked).toBe(true);
    });
    expect(reads).toBe(before + 1);
    expect(new URLSearchParams(page.search()).get("version")).toBe(selectedVersion);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
