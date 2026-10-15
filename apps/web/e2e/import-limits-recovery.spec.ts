import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { IMPORT_LIMITS, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const draft = "https://github.com/example/private-draft";
const imported = {
  shape: "skill",
  skills: [
    {
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 2,
      content_hash: "a".repeat(64),
      path: "skill",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
};

for (const failure of ["server", "network", "malformed"]) {
  for (const source of ["url", "upload"]) {
    test(`${failure} import limits retain the ${source} draft and retry only the read on a phone`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let failed = true;
      let writes = 0;
      await page.route("**/skills/import/limits", (route) => {
        if (!failed) return route.fulfill({ json: IMPORT_LIMITS });
        if (failure === "network") return route.abort("failed");
        if (failure === "malformed")
          return route.fulfill({ json: { allowed_hosts: "github.com" } });
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      });
      await page.route(`**/skills/import/${source}`, (route) => {
        writes += 1;
        return route.fulfill({ status: 201, json: imported });
      });
      await page.goto("/workspace/import");
      const rules = page.getByRole("region", { name: "匯入前檢查", exact: true });
      await expect(rules.getByRole("alert")).toHaveText("無法讀取匯入限制。請重新讀取後再送出。");
      await expect(rules).not.toContainText("來源限");
      await expect(rules.getByRole("status")).toHaveCount(0);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      if (source === "url")
        await page.getByRole("textbox", { name: "URL", exact: true }).fill(draft);
      else {
        await page.getByRole("radio", { name: "上傳 zip", exact: true }).check();
        await page.getByLabel("小工具 zip", { exact: true }).setInputFiles({
          name: "private.zip",
          mimeType: "application/zip",
          buffer: Buffer.from("zip draft"),
        });
      }
      const submit = page.getByRole("button", { name: "開始匯入", exact: true });
      await expect(submit).toBeDisabled();
      await expect(submit).toHaveAttribute("aria-describedby", "import-rules-unavailable");
      await expect(page.locator("#import-rules-unavailable")).toHaveText(
        "匯入限制尚未確認，請先讀取成功後再送出。",
      );
      await page.locator(".import-source form").dispatchEvent("submit");
      expect(writes).toBe(0);
      await rules.getByRole("alert").scrollIntoViewIfNeeded();
      await expect(rules.getByRole("alert")).toBeInViewport({ ratio: 1 });
      await page.screenshot({
        path: test.info().outputPath("import-limits-unavailable.png"),
      });
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      failed = false;
      await rules.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(page.getByRole("heading", { name: "匯入前檢查", exact: true })).toBeFocused();
      await expect(rules).toContainText("zip 最大 10 MB");
      await expect(rules).toContainText("來源限 github.com");
      await expect(rules.getByRole("alert")).toHaveCount(0);
      await expect(submit).toBeEnabled();
      if (source === "url")
        await expect(page.getByRole("textbox", { name: "URL", exact: true })).toHaveValue(draft);
      else
        expect(
          await page
            .getByLabel("小工具 zip", { exact: true })
            .evaluate((input: HTMLInputElement) => input.files?.[0]?.name),
        ).toBe("private.zip");
      expect(writes).toBe(0);
      await submit.click();
      await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
      expect(writes).toBe(1);
    });
  }
}

test("pending import limits keep an editable draft without submitting it", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let release!: () => void;
  let started!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const readStarted = new Promise<void>((resolve) => {
    started = resolve;
  });
  let writes = 0;
  await page.route("**/skills/import/limits", async (route) => {
    started();
    await gate;
    return route.fulfill({ json: IMPORT_LIMITS });
  });
  await page.route("**/skills/import/url", (route) => {
    writes += 1;
    return route.fulfill({ status: 201, json: imported });
  });
  try {
    await page.goto("/workspace/import");
    await readStarted;
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await expect(page.getByText("正在讀取匯入限制…", { exact: true })).toBeVisible();
    const input = page.getByRole("textbox", { name: "URL", exact: true });
    await input.fill(draft);
    await expect(page.getByRole("button", { name: "開始匯入", exact: true })).toBeDisabled();
    await page.locator(".import-source form").dispatchEvent("submit");
    expect(writes).toBe(0);
    release();
    await expect(page.getByRole("button", { name: "開始匯入", exact: true })).toBeEnabled();
    await expect(input).toHaveValue(draft);
    expect(writes).toBe(0);
    await page.getByRole("button", { name: "開始匯入", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
    expect(writes).toBe(1);
  } finally {
    release();
  }
});

test("an offline return withholds cached import limits until the paused read resumes", async ({
  page,
  context,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let writes = 0;
  await page.route("**/skills/import/url", (route) => {
    writes += 1;
    return route.fulfill({ status: 201, json: imported });
  });
  await page.goto("/workspace/import");
  const rules = page.getByRole("region", { name: "匯入前檢查", exact: true });
  await expect(rules).toContainText("zip 最大 10 MB");
  await page.getByRole("link", { name: "Skill Hub", exact: true }).click();
  await expect(page.getByRole("heading", { name: "匯入小工具", exact: true })).toHaveCount(0);
  await expect(page.locator("main h1")).toBeVisible();
  try {
    await context.setOffline(true);
    await page.goBack();
    await expect(rules.getByRole("status")).toHaveText("等待連線以讀取匯入限制…");
    await expect(rules).not.toContainText("zip 最大 10 MB");
    const input = page.getByRole("textbox", { name: "URL", exact: true });
    await input.fill(draft);
    await expect(page.getByRole("button", { name: "開始匯入", exact: true })).toBeDisabled();
    expect(writes).toBe(0);
    await context.setOffline(false);
    await expect(rules).toContainText("zip 最大 10 MB");
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await expect(page.getByRole("button", { name: "開始匯入", exact: true })).toBeEnabled();
    await expect(input).toHaveValue(draft);
    expect(writes).toBe(0);
    await page.getByRole("button", { name: "開始匯入", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
    expect(writes).toBe(1);
  } finally {
    await context.setOffline(false);
  }
});
