import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { OWN_BUNDLE, SKILL, VERSION, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import type { OwnerBundleSummary } from "../src/features/publishing/publishing.service";

const BUNDLE = { ...OWN_BUNDLE, members: [OWN_BUNDLE.members[0]] };
const OVERVIEW = {
  bundles: [
    {
      bundle: BUNDLE.bundle,
      latest_version: BUNDLE,
      publication: {
        publisher: "acme-tools",
        name: BUNDLE.bundle,
        address: `/p/acme-tools/${BUNDLE.bundle}`,
        status: "published",
        status_changed_at: "2026-09-10T00:00:00Z",
        latest_release: {
          bundle_version: BUNDLE.version,
          content_hash: BUNDLE.content_hash,
          released_at: "2026-09-10T00:00:00Z",
        },
      },
    },
  ],
} satisfies { bundles: OwnerBundleSummary[] };

for (const empty of [false, true]) {
  test(`bundle list failed reread withdraws a cached ${empty ? "empty" : "populated"} result and retries in place`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    let reads = 0;
    await page.route("**/me/bundles", (route) => {
      reads += 1;
      return reads === 2
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({ json: { bundles: empty && reads === 1 ? [] : [BUNDLE] } });
    });
    await page.route("**/me/bundles/overview", (route) => route.fulfill({ json: OVERVIEW }));
    await page.setViewportSize({ width: 375, height: 900 });
    await page.goto("/workspace/downloads");
    const section = page.locator("#bundle-workspace");
    if (empty)
      await expect(section.getByText("目前還沒有 Bundle。", { exact: false })).toBeVisible();
    else
      await expect(
        section.getByRole("heading", { name: BUNDLE.bundle, exact: true }),
      ).toBeVisible();
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    await page.goBack();
    await expect(section.getByRole("alert")).toContainText("暫時無法讀取Bundle 清單");
    await expect(section.getByRole("heading", { name: BUNDLE.bundle, exact: true })).toHaveCount(0);
    await expect(section.getByText("目前還沒有 Bundle。", { exact: false })).toHaveCount(0);
    await expect(section.getByText("建立 Bundle", { exact: true })).toBeVisible();
    await expect(section.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
    await section.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(section.getByRole("heading", { name: BUNDLE.bundle, exact: true })).toBeVisible();
    await expect(page.locator("#bundle-workspace-heading")).toBeFocused();
    expect(reads).toBe(3);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath(`bundle-${empty}-recovered.png`),
      fullPage: true,
    });
  });
}

test("bundle overview failure withdraws the old Release and retries without rereading versions", async ({
  page,
}) => {
  await stubPlatform(page);
  let bundleReads = 0;
  let overviewReads = 0;
  await page.route("**/me/bundles", (route) => {
    bundleReads += 1;
    return route.fulfill({ json: { bundles: [BUNDLE] } });
  });
  await page.route("**/me/bundles/overview", (route) => {
    overviewReads += 1;
    return overviewReads === 2
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: OVERVIEW });
  });
  await page.goto("/workspace/downloads");
  const section = page.locator("#bundle-workspace");
  await expect(section.locator(".bundle-publication")).toBeVisible();
  await page.locator('a[href="/policy"]').first().click();
  await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
  await page.goBack();
  await expect(section.getByRole("alert")).toContainText("Bundle 發佈概覽");
  await expect(section.locator(".bundle-publication")).toHaveCount(0);
  await expect(section.getByText("已發佈", { exact: true })).toHaveCount(0);
  const before = bundleReads;
  await section.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(section.locator(".bundle-publication")).toBeVisible();
  expect(bundleReads).toBe(before);
  expect(overviewReads).toBe(3);
  await expect(page.locator("#bundle-workspace-heading")).toBeFocused();
});

for (const source of ["skills", "detail", "versions"]) {
  test(`bundle ${source} read failure is visible outside the editor and blocks an uninformed publication`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.route("**/me/bundles", (route) => route.fulfill({ json: { bundles: [BUNDLE] } }));
    await page.route("**/me/bundles/overview", (route) => route.fulfill({ json: OVERVIEW }));
    const pattern =
      source === "skills"
        ? "**/skills?*"
        : source === "detail"
          ? `**/api/skills/${SKILL}?*`
          : `**/skills/${SKILL}/versions`;
    let reads = 0;
    await page.route(pattern, (route) => {
      reads += 1;
      return reads === 1
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({ json: platformResponse(route.request().url()).body });
    });
    await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
    const section = page.locator("#bundle-workspace");
    await expect(section.getByRole("alert")).toBeVisible();
    await expect(section.getByRole("alert")).toContainText("可加入 Bundle 的版本");
    expect(
      await section.getByRole("alert").evaluate((element) => element.closest("details") === null),
    ).toBe(true);
    await expect(
      section.getByRole("button", { name: `發佈 v${BUNDLE.version}`, exact: true }),
    ).toBeDisabled();
    await expect(section.getByText("散布需求尚未確認", { exact: false })).toBeVisible();
    const form = section.locator(".bundle-form");
    await form.getByLabel("名稱", { exact: true }).fill("pending-bundle");
    await form.getByLabel("版本（semver，例如 1.0.0）", { exact: true }).fill("1.2.0");
    await form.getByLabel("說明", { exact: true }).fill("保留這段描述");
    await section.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(section.getByRole("alert")).toHaveCount(0);
    await expect(form.getByLabel("名稱", { exact: true })).toHaveValue("pending-bundle");
    await expect(form.getByLabel("說明", { exact: true })).toHaveValue("保留這段描述");
    await expect(form.locator(`[data-bundle-version="${VERSION}"]`)).toHaveValue(VERSION);
    expect(reads).toBe(2);
  });
}
