import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { OWN_PUBLICATIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const invalidResponses: [string, unknown][] = [
  ["missing collection", {}],
  ["null envelope", null],
  ["null row", { publications: [null] }],
  [
    "unknown exposure",
    {
      publications: OWN_PUBLICATIONS.publications.map((publication) => ({
        ...publication,
        catalog_exposure: { state: "future_state" },
      })),
    },
  ],
];

for (const width of [375, 1280]) {
  for (const [name, invalid] of invalidResponses) {
    test(`publication ${name} keeps a Bundle draft through recovery at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      await stubPlatform(page);
      let reads = 0;
      await page.route("**/me/publications", (route) => {
        reads += 1;
        return route.fulfill({
          contentType: "application/json",
          body: JSON.stringify(reads === 2 ? invalid : OWN_PUBLICATIONS),
        });
      });
      await page.goto("/workspace/downloads");
      const overview = page.locator("#skill-publications");
      await expect(overview.locator("li")).toHaveCount(1);
      const form = page.getByRole("region", { name: "建立 Bundle", exact: true });
      await form.locator("summary").click();
      const bundleName = form.getByLabel("名稱", { exact: true });
      const version = form.getByLabel("版本（semver，例如 1.0.0）", { exact: true });
      const description = form.getByLabel("說明", { exact: true });
      await bundleName.fill("kept-bundle");
      await version.fill("1.2.3");
      await description.fill("保留未送出的 Bundle 說明");
      await overview.getByRole("button", { name: "重新整理", exact: true }).click();
      await expect(overview.getByRole("alert")).toContainText("暫時無法讀取小工具發佈清單");
      await expect(overview.locator("li")).toHaveCount(0);
      await expect(overview).not.toContainText("還沒有任何小工具 Publication");
      await expect(bundleName).toHaveValue("kept-bundle");
      await expect(version).toHaveValue("1.2.3");
      await expect(description).toHaveValue("保留未送出的 Bundle 說明");
      const accessibility = await new AxeBuilder({ page }).include("main").analyze();
      expect(accessibility.violations).toEqual([]);
      expect(accessibility.incomplete).toEqual([]);
      await overview.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(overview.locator("li")).toHaveCount(1);
      await expect(overview.getByRole("alert")).toHaveCount(0);
      await expect(page.locator("#publication-overview-title")).toBeFocused();
      await expect(bundleName).toHaveValue("kept-bundle");
      await expect(version).toHaveValue("1.2.3");
      await expect(description).toHaveValue("保留未送出的 Bundle 說明");
      expect(reads).toBe(3);
    });
  }
}
