import { expect, test } from "vitest";
import { OWN_PUBLICATIONS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

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

for (const [name, invalid] of invalidResponses) {
  test(`publication ${name} keeps a Bundle draft through recovery`, async () => {
    const page = app();
    let reads = 0;
    page.route("**/me/publications", () => {
      reads += 1;
      return { json: reads === 2 ? invalid : OWN_PUBLICATIONS };
    });
    await page.goto("/workspace/downloads");
    const overview = page.locator("#skill-publications");
    await expect(overview.locator("li")).toHaveCount(1);
    const form = page.getByRole("region", { name: "建立 Bundle", exact: true });
    await form.locator("summary").click();
    const bundleName = page.getByLabel("名稱", { exact: true });
    const version = page.getByLabel("版本（semver，例如 1.0.0）", { exact: true });
    const description = page.getByLabel("說明", { exact: true });
    await bundleName.fill("kept-bundle");
    await version.fill("1.2.3");
    await description.fill("保留未送出的 Bundle 說明");
    await overview.locator("button").filter({ hasText: "重新整理" }).click();
    await page.eventually(() => {
      expect(overview.locator("[role=alert]").text()).toContain("暫時無法讀取小工具發佈清單");
      expect(overview.locator("li").count()).toBe(0);
      expect(overview.text()).not.toContain("還沒有任何小工具 Publication");
      expect(bundleName.value()).toBe("kept-bundle");
      expect(version.value()).toBe("1.2.3");
      expect(description.value()).toBe("保留未送出的 Bundle 說明");
    });
    expect(await page.axeViolations("main")).toEqual([]);
    await overview.locator("button").filter({ hasText: "再試一次" }).click();
    await page.eventually(() => {
      expect(overview.locator("li").count()).toBe(1);
      expect(overview.locator("[role=alert]").count()).toBe(0);
      expect(page.locator("#publication-overview-title").isFocused()).toBe(true);
      expect(bundleName.value()).toBe("kept-bundle");
      expect(version.value()).toBe("1.2.3");
      expect(description.value()).toBe("保留未送出的 Bundle 說明");
    });
    expect(reads).toBe(3);
  });
}
