import { expect, test } from "vitest";
import { IMPORT_LIMITS, SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";
import { chooseZipWithoutNativeValidation } from "../../../testing/files";

const app = renderedApp();

const RULES = {
  ...IMPORT_LIMITS,
  max_zip_bytes: 6 << 20,
  max_unpacked_bytes: 48 << 20,
  max_files: 37,
  max_file_bytes: 2 << 20,
  max_path_depth: 5,
};

const UPLOAD_HEADING = "上傳新版本";
const SUBMIT = "上傳成新版本";

function chosenName(page: App) {
  return (page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement).files?.[0]
    ?.name;
}

function uploadSection(page: App) {
  return page.locator("aside").locator("section").filter({ hasText: UPLOAD_HEADING });
}

test("an offline version upload withholds cached limits and preserves the file", async () => {
  const page = app();
  page.route("**/skills/import/limits", () => ({ json: RULES }));
  await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
  await expect(page.getByText(/平台限制：zip 最大 6 MB/)).toHaveCount(1);
  await page.getByRole("link", { name: "Skill Hub", exact: true }).click();
  await expect(page.getByRole("heading", { name: UPLOAD_HEADING, exact: true })).toHaveCount(0);
  try {
    await page.setOffline(true);
    await page.goBack();
    await expect(page.getByText("等待連線以讀取上傳限制…", { exact: true })).toHaveCount(1);
    await chooseZipWithoutNativeValidation(page, "offline.zip", "offline draft");
    const submit = page.getByRole("button", { name: SUBMIT, exact: true });
    await page.eventually(() => {
      expect(page.getByText(/平台限制：zip 最大/).count()).toBe(0);
      expect(submit.isDisabled()).toBe(true);
    });
    expect(await page.axeViolations("main")).toEqual([]);
    await page.setOffline(false);
    await page.eventually(() => {
      expect(submit.isDisabled()).toBe(false);
      expect(page.getByText(/平台限制：zip 最大 6 MB/).count()).toBe(1);
    });
    expect(chosenName(page)).toBe("offline.zip");
    expect(await page.axeViolations("main")).toEqual([]);
  } finally {
    await page.setOffline(false);
  }
});

for (const fault of ["server", "unreadable limits"] as const) {
  for (const reread of [false, true]) {
    test(`version upload retains its file after ${fault} on ${reread ? "reread" : "first read"}`, async () => {
      const page = app();
      let fail = !reread;
      let reads = 0;
      page.route("**/skills/import/limits", () => {
        reads++;
        if (!fail) return { json: RULES };
        return fault === "server"
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: { ...RULES, max_zip_bytes: null } };
      });
      await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
      const section = uploadSection(page);
      const submit = page.getByRole("button", { name: SUBMIT, exact: true });
      if (reread) {
        await page.eventually(() => {
          expect(section.text()).toContain("zip 最大 6 MB");
          expect(submit.isDisabled()).toBe(false);
        });
        fail = true;
        await page.getByRole("link", { name: "Skill Hub", exact: true }).click();
        await expect(page.getByRole("heading", { name: UPLOAD_HEADING, exact: true })).toHaveCount(
          0,
        );
        await page.goBack();
        await page.eventually(() => expect(reads).toBe(2));
      }
      await expect(section).toHaveCount(1);
      await chooseZipWithoutNativeValidation(page, "draft.zip", "retained draft");
      await page.eventually(() => {
        expect(section.getByRole("alert").text()).toContain("暫時無法讀取上傳限制");
        expect(submit.isDisabled()).toBe(true);
        expect(submit.attribute("aria-describedby")).toMatch(/.+/);
        expect(section.text()).not.toContain("zip 最大");
      });
      expect(chosenName(page)).toBe("draft.zip");
      expect(await page.axeViolations("main")).toEqual([]);
      fail = false;
      await section.getByRole("button", { name: "再試一次", exact: true }).click();
      await page.eventually(() => {
        expect(submit.isDisabled()).toBe(false);
        expect(section.getByRole("heading", { name: UPLOAD_HEADING }).isFocused()).toBe(true);
        expect(section.text()).toContain("平台限制：zip 最大 6 MB");
        expect(section.text()).toContain("解壓後總量最大 48 MB");
        expect(section.text()).toContain("最多 37 個檔案");
        expect(section.text()).toContain("單一檔案最大 2 MB");
        expect(section.text()).toContain("路徑最深 5 層");
      });
      expect(chosenName(page)).toBe("draft.zip");
      expect(reads).toBe(reread ? 3 : 2);
      expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}
