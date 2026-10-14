import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { SKILL, VERSION, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const own = platformResponse("/skills").body as { skills: Array<Record<string, unknown>> };
const firstPage = Array.from({ length: 24 }, (_, index) => ({
  ...own.skills[0],
  skill_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
  name: `Earlier asset ${index + 1}`,
}));
const target = { ...own.skills[0], name: "Target asset", summary: "The requested asset" };

async function pagedAssets(page: Page) {
  const offsets: number[] = [];
  await page.route("**/skills?*", (route) => {
    const offset = Number(new URL(route.request().url()).searchParams.get("offset"));
    offsets.push(offset);
    return route.fulfill({
      json: {
        skills: offset === 0 ? firstPage : [target],
        total: 25,
        limit: 24,
        truncated: offset === 0,
      },
    });
  });
  await page.route("**/skills", (route) => {
    offsets.push(0);
    return route.fulfill({ json: { skills: firstPage, total: 25, limit: 24, truncated: true } });
  });
  return offsets;
}

test("test-case asset paging avoids an ownership conclusion and retains the draft before finding the requested asset", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  const offsets = await pagedAssets(page);
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await expect(page.locator("#tc-skill option")).toHaveCount(25);
  await expect(page.getByText("這個小工具不在你的工作區。", { exact: false })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "載入更多小工具", exact: true })).toBeVisible();
  await page.getByLabel("名稱", { exact: true }).fill("尚未送出的測試題");
  await page.getByLabel("User Prompt", { exact: true }).fill("保留任務提示");
  await page.getByRole("button", { name: "載入更多小工具", exact: true }).click();
  await expect(page.locator("#tc-skill")).toHaveValue(SKILL);
  await expect(page.getByLabel("名稱", { exact: true })).toHaveValue("尚未送出的測試題");
  await expect(page.getByLabel("User Prompt", { exact: true })).toHaveValue("保留任務提示");
  expect(offsets).toEqual([0, 24]);
  await expect(page.getByRole("button", { name: "載入更多小工具", exact: true })).toHaveCount(0);
  expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("test-case-asset-paging.png"),
    fullPage: true,
  });
});

test("Bundle asset paging reaches the requested version without replacing typed identity fields", async ({
  page,
}) => {
  await stubPlatform(page);
  const offsets = await pagedAssets(page);
  const details = platformResponse(`/api/skills/${SKILL}`).body as object;
  const versionList = platformResponse(`/skills/${SKILL}/versions`).body as {
    versions: Array<Record<string, unknown>>;
  };
  await page.route("**/api/skills/*?view=embedded", (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-1);
    return route.fulfill({
      json: { ...details, skill_id: id, name: id === SKILL ? target.name : "Earlier asset" },
    });
  });
  await page.route("**/skills/*/versions", (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-2);
    return route.fulfill({
      json: {
        versions:
          id === SKILL
            ? versionList.versions
            : [
                {
                  ...versionList.versions[0],
                  version_id: `20000000-0000-4000-8000-${id?.slice(-12)}`,
                },
              ],
      },
    });
  });
  await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
  const section = page.locator("#bundle-workspace");
  const form = section.locator(".bundle-form");
  await expect(form.locator("select")).toHaveCount(24);
  await expect(form.getByText("無法確認要接續的 Version", { exact: false })).toHaveCount(0);
  await form.getByLabel("名稱", { exact: true }).fill("draft-bundle");
  await form.getByLabel("版本（semver，例如 1.0.0）", { exact: true }).fill("1.2.0");
  await section.getByRole("button", { name: "載入更多小工具", exact: true }).click();
  await expect(form.locator(`[data-bundle-version="${VERSION}"]`)).toHaveValue(VERSION);
  await expect(form.getByLabel("名稱", { exact: true })).toHaveValue("draft-bundle");
  await expect(form.getByLabel("版本（semver，例如 1.0.0）", { exact: true })).toHaveValue("1.2.0");
  expect(offsets).toEqual([0, 24]);
  await expect(section.getByRole("button", { name: "載入更多小工具", exact: true })).toHaveCount(0);
});

test("a failed asset page preserves loaded choices and the draft until that page is recovered", async ({
  page,
}) => {
  await stubPlatform(page);
  const offsets = await pagedAssets(page);
  let failed = false;
  await page.route("**/skills?*", (route) => {
    const offset = Number(new URL(route.request().url()).searchParams.get("offset"));
    if (offset === 24 && !failed) {
      failed = true;
      offsets.push(offset);
      return route.fulfill({ status: 503, json: { code: "unavailable", message: "Unavailable" } });
    }
    return route.fallback();
  });
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await expect(page.locator("#tc-skill option")).toHaveCount(25);
  await page.getByLabel("名稱", { exact: true }).fill("保留草稿");
  await page.getByLabel("User Prompt", { exact: true }).fill("保留提示");
  await page.getByRole("button", { name: "載入更多小工具", exact: true }).click();
  await expect(
    page.getByText("目前無法確認可用的小工具，請先重新讀取清單。", { exact: true }),
  ).toBeVisible();
  await expect(page.locator("#tc-skill option")).toHaveCount(25);
  await expect(page.getByText("這個小工具不在你的工作區。", { exact: false })).toHaveCount(0);
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.locator("#tc-skill")).toHaveValue(SKILL);
  await expect(page.getByLabel("名稱", { exact: true })).toHaveValue("保留草稿");
  await expect(page.getByLabel("User Prompt", { exact: true })).toHaveValue("保留提示");
  expect(offsets).toEqual([0, 24, 24]);
  await expect(page.locator("#create-test-case-heading")).toBeFocused();
});

test("the reference picker distinguishes a partial asset search from absence and reaches an older match", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.route("**/me", (route) =>
    route.fulfill({
      json: {
        ...(platformResponse(route.request().url()).body as object),
        features: { generate_skill: true, creation_skill: false },
      },
    }),
  );
  const offsets = await pagedAssets(page);
  await page.route("**/api/skills/search?*", (route) =>
    route.fulfill({
      json: {
        query: "Target",
        results: [],
        no_results: true,
        limit: 20,
        truncated: false,
        degraded: false,
        partial_index: false,
        filtered_out: false,
      },
    }),
  );
  await page.goto("/workspace/creations");
  await page.getByText("附一張流程圖，或指定要參考的小工具（都是選填）", { exact: true }).click();
  await page.getByLabel("搜尋要參考的小工具", { exact: true }).fill("Target");
  await expect(page.getByRole("heading", { name: "資產庫中的小工具", exact: true })).toBeVisible();
  await expect(page.getByText("資產庫裡沒有符合項目。", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "載入更多小工具", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "載入更多小工具", exact: true }).click();
  await expect(page.getByRole("checkbox", { name: /Target asset/ })).toBeVisible();
  await page.getByRole("checkbox", { name: /Target asset/ }).check();
  await expect(page.getByRole("list", { name: "已選的參考小工具", exact: true })).toContainText(
    "Target asset",
  );
  expect(offsets).toEqual([0, 24]);
});
