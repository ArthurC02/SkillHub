import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type Locator } from "@playwright/test";
import { SKILL, VERSION, SKILL_VERSIONS, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const NEXT = "abcdef01-2345-4789-abcd-0123456789ab";
const FIRST_FILE = `${"package".repeat(35)}.zip`;
const SAVED = {
  skill_id: SKILL,
  version_id: NEXT,
  version_number: 3,
  content_hash: "sha256:next",
  duplicate: false,
  findings: { errors: [], warnings: [], infos: [] },
};
const CURRENT = {
  versions: [
    {
      version_id: NEXT,
      version_number: 3,
      content_hash: "sha256:next",
      created_at: "2026-10-15T00:00:00Z",
    },
    ...SKILL_VERSIONS.versions,
  ],
};
async function openUpload(page: Page) {
  await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  const input = page.getByLabel("小工具 zip", { exact: true });
  await input.setInputFiles({
    name: FIRST_FILE,
    mimeType: "application/zip",
    buffer: Buffer.from("first bytes"),
  });
  return input;
}

async function verifyNextFile(input: Locator) {
  expect(
    await input.evaluate(async (element) => {
      const file = (element as HTMLInputElement).files?.[0];
      return { name: file?.name, content: file ? await file.text() : undefined };
    }),
  ).toEqual({ name: "next.zip", content: "next bytes" });
}

for (const source of ["skill", "versions"]) {
  test(`a confirmed version upload survives a failed ${source} reread and retains its next file through offline recovery`, async ({
    page,
    context,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let uploaded = false;
    let recovered = false;
    let posts = 0;
    let rereads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    async function rejectReread() {
      rereads += 1;
      await gate;
      return { status: 503, json: { error: "service unavailable" } };
    }
    await page.route(`**/api/skills/${SKILL}`, async (route) =>
      route.fulfill(
        uploaded && !recovered && source === "skill"
          ? await rejectReread()
          : { json: platformResponse(route.request().url()).body },
      ),
    );
    await page.route(`**/skills/${SKILL}/versions`, async (route) => {
      if (route.request().method() === "POST") {
        posts += 1;
        uploaded = true;
        return route.fulfill({ status: 201, json: SAVED });
      }
      return route.fulfill(
        uploaded && !recovered && source === "versions"
          ? await rejectReread()
          : { json: uploaded ? CURRENT : SKILL_VERSIONS },
      );
    });
    try {
      const input = await openUpload(page);
      await page.getByRole("button", { name: "上傳成新版本", exact: true }).click();
      const status = page.getByRole("status").filter({ hasText: "已存成 v3" });
      await expect(status).toBeVisible();
      const result = page.getByRole("link", { name: "開啟 v3 並繼續驗證", exact: true });
      await expect(result).toHaveAttribute("href", `/skills/${SKILL}/versions/${NEXT}`);
      await expect(result).toBeFocused();
      const position = await result.evaluate((element) => ({
        top: element.getBoundingClientRect().top,
        header: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
      }));
      expect(position.top).toBeGreaterThanOrEqual(position.header);
      await expect.poll(() => rereads).toBe(1);
      await expect(page.locator("#version-facts-title")).toHaveText("v2，是否最新尚未確認");
      await input.setInputFiles({
        name: "next.zip",
        mimeType: "application/zip",
        buffer: Buffer.from("next bytes"),
      });
      await input.focus();
      const submit = page.getByRole("button", { name: "上傳成新版本", exact: true });
      await expect(submit).toBeDisabled();
      await page
        .locator(".version-upload")
        .evaluate((form) =>
          form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
        );
      release();
      await expect(page.getByRole("alert")).toContainText(
        source === "skill" ? "暫時無法讀取這個小工具" : "暫時無法讀取版本脈絡",
      );
      await expect(input).toBeFocused();
      await expect(status).toContainText(`這次上傳的檔案：${FIRST_FILE}`);
      await expect(submit).toBeDisabled();
      await page
        .locator(".version-upload")
        .evaluate((form) =>
          form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
        );
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: test.info().outputPath("version-upload-retained.png") });
      await result.click();
      await expect(page).toHaveURL(new RegExp(`/skills/${SKILL}/versions/${NEXT}$`));
      await expect(input).toBeVisible();
      await verifyNextFile(input);
      recovered = true;
      await context.setOffline(true);
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(page.getByRole("button", { name: /^(再試一次|重新讀取中…)$/ })).toBeDisabled();
      await expect(
        page.getByRole("status").filter({ hasText: "等待連線以確認小工具與版本脈絡" }),
      ).toHaveText("等待連線以確認小工具與版本脈絡；恢復連線後會繼續讀取。");
      expect(await page.evaluate(() => navigator.onLine)).toBe(false);
      await expect(submit).toBeDisabled();
      expect(
        await input.evaluate((element) => {
          const file = (element as HTMLInputElement).files?.[0];
          return { name: file?.name, size: file?.size };
        }),
      ).toEqual({ name: "next.zip", size: 10 });
      await context.setOffline(false);
      await expect(
        page.getByRole("heading", { name: "PDF Summariser v3", exact: true }),
      ).toBeVisible();
      await expect(submit).toBeEnabled();
      await verifyNextFile(input);
      await expect(status).toContainText(`這次上傳的檔案：${FIRST_FILE}`);
      await expect(page.getByRole("main")).toBeFocused();
      expect(posts).toBe(1);
    } finally {
      release();
      await context.setOffline(false);
    }
  });
}

for (const [source, status] of [
  ["skill", 401],
  ["skill", 410],
  ["versions", 401],
  ["versions", 403],
  ["versions", 404],
  ["versions", 200],
] as const) {
  test(`a ${source} reread returning ${status} removes protected cached upload controls`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let uploaded = false;
    let posts = 0;
    const endpoint = source === "skill" ? `/api/skills/${SKILL}` : `/skills/${SKILL}/versions`;
    await page.route(`**${endpoint}`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(
        uploaded
          ? {
              status,
              json:
                status === 200
                  ? { versions: [] }
                  : { error: status === 401 ? "not authenticated" : "not found" },
            }
          : { json: platformResponse(route.request().url()).body },
      );
    });
    await page.route(`**/skills/${SKILL}/versions`, (route) => {
      if (route.request().method() !== "POST") return route.fallback();
      uploaded = true;
      posts += 1;
      return route.fulfill({ status: 201, json: SAVED });
    });
    const input = await openUpload(page);
    await page.getByRole("button", { name: "上傳成新版本", exact: true }).click();
    if (status === 401) await expect(page.getByText("需要登入。", { exact: false })).toBeVisible();
    else if (status === 200) await expect(page.getByRole("alert")).toContainText("無權檢視");
    else if (status === 410)
      await expect(page.getByRole("alert")).toContainText("這個小工具已從目錄下架");
    else await expect(page.getByRole("alert")).toContainText("暫時無法讀取版本脈絡");
    await expect(input).toHaveCount(0);
    await expect(page.getByRole("link", { name: "開啟 v3 並繼續驗證", exact: true })).toHaveCount(
      0,
    );
    expect(posts).toBe(1);
  });
}
