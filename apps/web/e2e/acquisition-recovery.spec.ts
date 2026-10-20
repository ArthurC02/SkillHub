import AxeBuilder from "@axe-core/playwright";
import { expect, test, type BrowserContext, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  ARTIFACT,
  OWN_BUNDLE,
  PUBLICATION,
  PUBLISHER,
  PUBLIC_BUNDLE_PUBLICATION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { serveStatic, STATIC_KINDS } from "./stub";

const RECEIPT = {
  artifact_id: ARTIFACT,
  file_name: "x".repeat(128) + ".zip",
  size_bytes: 0,
  content_hash: "sha256:another-release",
  expires_at: "2099-01-01T00:00:00Z",
  duplicate: true,
  content_url: `/downloads/${ARTIFACT}/content`,
};
type Kind = "skill" | "public bundle" | "private bundle";

async function stubAcquisition(context: BrowserContext, kind: Kind, reply: unknown, status = 201) {
  const writes: string[] = [];
  await context.route("**/*", (route, request) => {
    if (STATIC_KINDS.has(request.resourceType())) return serveStatic(route);
    const path = new URL(request.url()).pathname;
    if (
      request.method() === "POST" &&
      (path.endsWith("/acquisitions") || path.endsWith("/export"))
    ) {
      writes.push(request.url());
      return route.fulfill({ status, json: reply });
    }
    if (path === "/me/bundles") return route.fulfill({ json: { bundles: [OWN_BUNDLE] } });
    if (path === "/me/bundles/overview")
      return route.fulfill({
        json: { bundles: [{ bundle: "pdf-toolkit", latest_version: OWN_BUNDLE }] },
      });
    if (path === `/publications/${PUBLISHER}/pdf-toolkit`)
      return route.fulfill({ json: PUBLIC_BUNDLE_PUBLICATION });
    const response = platformResponse(request.url());
    return route.fulfill({ status: response.status, json: response.body });
  });
  return {
    writes,
    url:
      kind === "private bundle"
        ? "/workspace/downloads"
        : `/p/${PUBLISHER}/${kind === "skill" ? PUBLICATION : "pdf-toolkit"}`,
    button: kind === "private bundle" ? "匯出 v1.1.0 Plugin" : "下載",
  };
}

async function capture(page: Page) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath("acquisition-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("acquisition-result.png") });
}

for (const kind of ["skill", "public bundle", "private bundle"] as const) {
  for (const damage of ["null", "missing URL"] as const) {
    test(`an uncertain ${kind} acquisition with ${damage} retains context during keyboard verification`, async ({
      page,
      context,
    }) => {
      const { content_url: _contentURL, ...incomplete } = RECEIPT;
      const state = await stubAcquisition(context, kind, damage === "null" ? null : incomplete);
      await page.setViewportSize({ width: 375, height: 900 });
      await page.goto(state.url);
      await page.getByRole("button", { name: state.button, exact: true }).click();
      const alert = page.getByRole("alert").filter({ hasText: "下載結果尚未確認" });
      await expect(alert).toBeFocused();
      await expect(alert).toBeInViewport();
      await expect(page).toHaveURL(new RegExp(state.url + "$"));
      await expect(
        page.getByRole("heading", {
          name:
            kind === "private bundle"
              ? "發佈與交付"
              : kind === "skill"
                ? "PDF Summariser"
                : "pdf-toolkit",
          exact: true,
        }),
      ).toBeVisible();
      expect(state.writes).toHaveLength(1);
      await expect(page.locator('a[href="undefined"]')).toHaveCount(0);
      await capture(page);
      const recovery = page.getByRole("link", { name: "另開交付紀錄核對", exact: true });
      await expect(recovery).toHaveAttribute("href", "/workspace/downloads");
      await expect(recovery).toHaveAttribute("rel", "noopener noreferrer");
      const opened = context.waitForEvent("page");
      await recovery.focus();
      await page.keyboard.press("Enter");
      const verification = await opened;
      await expect(verification).toHaveURL(/\/workspace\/downloads$/);
      await expect(
        verification.getByRole("heading", { name: "交付紀錄", exact: true }),
      ).toBeVisible();
      await verification.close();
      await page.bringToFront();
      await expect(page).toHaveURL(new RegExp(state.url + "$"));
      await expect(alert).toContainText("下載結果尚未確認");
      expect(state.writes).toHaveLength(1);
    });
  }

  test(`a valid ${kind} acquisition keeps a long filename and exact delivery link readable`, async ({
    page,
    context,
  }) => {
    const state = await stubAcquisition(context, kind, RECEIPT);
    await page.setViewportSize({
      width: kind === "public bundle" ? 1280 : 375,
      height: kind === "public bundle" ? 900 : 667,
    });
    await page.goto(state.url);
    await page.getByRole("button", { name: state.button, exact: true }).click();
    const download = page.getByRole("link", { name: "下載 " + RECEIPT.file_name, exact: true });
    await expect(download).toHaveAttribute("href", `/downloads/${ARTIFACT}/content`);
    await expect(page.getByRole("status").filter({ has: download })).toContainText(
      RECEIPT.file_name,
    );
    const history = page.getByRole("link", {
      name: kind === "private bundle" ? "查看這次交付紀錄" : "在交付紀錄查看這一份",
      exact: true,
    });
    await expect(history).toHaveAttribute("href", `/workspace/downloads?artifact=${ARTIFACT}`);
    await download.scrollIntoViewIfNeeded();
    await expect(download).toBeInViewport();
    expect(state.writes).toHaveLength(1);
    await capture(page);
  });
}

test("a foreign acquisition URL offers record verification without an unverified download", async ({
  page,
  context,
}) => {
  const state = await stubAcquisition(context, "public bundle", {
    ...RECEIPT,
    content_url: "/downloads/55555555-5555-4555-8555-555555555555/content",
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(state.url);
  await page.getByRole("button", { name: "下載", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "下載結果尚未確認" })).toBeFocused();
  await expect(page.locator('a[href*="55555555-5555"]')).toHaveCount(0);
  await expect(page.getByRole("link", { name: "另開交付紀錄核對", exact: true })).toBeVisible();
  expect(state.writes).toHaveLength(1);
  await capture(page);
});

for (const [status, message, expected] of [
  [401, "not authenticated", "這個下載需要登入"],
  [
    403,
    "Skill Hub 還在封測：瀏覽與小工具詳情對所有人開放，但複製一份、試跑與下載只開放給受邀的測試者。",
    "還在封測",
  ],
  [409, "作者撤回了這個發佈物，這一頁不再提供它的內容。", "作者撤回了這個發佈物"],
] as const) {
  test(`an acquisition rejected with ${status} retains its definite outcome`, async ({
    page,
    context,
  }) => {
    const state = await stubAcquisition(context, "skill", { error: message }, status);
    await page.setViewportSize({ width: 375, height: 900 });
    await page.goto(state.url);
    await page.getByRole("button", { name: "下載", exact: true }).click();
    await expect(
      page.getByRole(status === 401 ? "status" : "alert").filter({ hasText: expected }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "另開交付紀錄核對", exact: true })).toHaveCount(0);
    expect(state.writes).toHaveLength(1);
    await capture(page);
  });
}

test("an acquisition server error retains publication context and offers verification", async ({
  page,
  context,
}) => {
  const state = await stubAcquisition(context, "skill", { error: "internal error" }, 500);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(state.url);
  await page.getByRole("button", { name: "下載", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "下載結果尚未確認" })).toBeFocused();
  await expect(page.getByRole("link", { name: "另開交付紀錄核對", exact: true })).toBeVisible();
  expect(state.writes).toHaveLength(1);
  await capture(page);
});
