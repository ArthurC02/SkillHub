import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { OWN_BUNDLE } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const publication = {
  publisher: "acme-tools",
  name: "pdf-toolkit",
  address: "/p/acme-tools/pdf-toolkit",
  status: "published",
  status_changed_at: "2026-09-10T00:00:00Z",
};
const row = { bundle: OWN_BUNDLE.bundle, latest_version: OWN_BUNDLE, publication };
const overview = { bundles: [row] };
const versions = { bundles: [OWN_BUNDLE] };
const invalidResponses: [string, "versions" | "overview", unknown][] = [
  ["missing collection", "versions", {}],
  ["null envelope", "versions", null],
  ["null row", "versions", { bundles: [null] }],
  ["missing members", "versions", { bundles: [{ ...OWN_BUNDLE, members: undefined }] }],
  ["missing collection", "overview", {}],
  ["null envelope", "overview", null],
  ["null row", "overview", { bundles: [null] }],
  ["null publication", "overview", { bundles: [{ ...row, publication: null }] }],
  [
    "unknown status",
    "overview",
    { bundles: [{ ...row, publication: { ...publication, status: "future" } }] },
  ],
];

for (const width of [375, 1280]) {
  for (const [name, kind, invalid] of invalidResponses) {
    test(`Bundle ${kind} ${name} preserves a draft through recovery at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      await stubPlatform(page);
      let reads = 0;
      const current = kind === "versions" ? versions : overview;
      await page.route("**/me/bundles", (route) => {
        if (kind !== "versions") return route.fulfill({ json: versions });
        reads += 1;
        return route.fulfill({
          contentType: "application/json",
          body: JSON.stringify(reads === 2 ? invalid : current),
        });
      });
      await page.route("**/me/bundles/overview", (route) => {
        if (kind !== "overview") return route.fulfill({ json: overview });
        reads += 1;
        return route.fulfill({
          contentType: "application/json",
          body: JSON.stringify(reads === 2 ? invalid : current),
        });
      });
      await page.goto("/workspace/downloads");
      const section = page.locator("#bundle-workspace");
      await expect(section.locator(".bundle-overview-header")).toContainText("已發佈");
      const form = page.getByRole("region", { name: "建立 Bundle", exact: true });
      await form.locator("summary").click();
      const bundleName = form.getByLabel("名稱", { exact: true });
      const version = form.getByLabel("版本（semver，例如 1.0.0）", { exact: true });
      const description = form.getByLabel("說明", { exact: true });
      await bundleName.fill("kept-bundle");
      await version.fill("1.2.3");
      await description.fill("保留未送出的 Bundle 說明");
      await section.getByRole("button", { name: "重新整理", exact: true }).click();
      await expect(section.getByRole("alert")).toContainText(
        kind === "versions" ? "暫時無法讀取Bundle 清單" : "暫時無法讀取Bundle 發佈概覽",
      );
      await expect(section).not.toContainText("尚未發佈。");
      await expect(section).not.toContainText("已撤回");
      await expect(section).not.toContainText("目前還沒有 Bundle。");
      await expect(form.locator("summary")).not.toHaveText("建立第一個 Bundle");
      await expect(bundleName).toHaveValue("kept-bundle");
      await expect(version).toHaveValue("1.2.3");
      await expect(description).toHaveValue("保留未送出的 Bundle 說明");
      const report = await new AxeBuilder({ page }).include("main").analyze();
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await section.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(section.locator(".bundle-overview-header")).toContainText("已發佈");
      await expect(section.getByRole("alert")).toHaveCount(0);
      await expect(page.locator("#bundle-workspace-heading")).toBeFocused();
      await expect(bundleName).toHaveValue("kept-bundle");
      await expect(version).toHaveValue("1.2.3");
      await expect(description).toHaveValue("保留未送出的 Bundle 說明");
      expect(reads).toBe(3);
    });
  }
}
