import { expect, test, vi } from "vitest";
import { OWN_BUNDLE } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

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

function serveBundleReads(kind: "versions" | "overview", invalid: unknown) {
  const reads = { count: 0 };
  const harnessFetch = globalThis.fetch;
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    const pathname = new URL(String(input), "http://127.0.0.1").pathname;
    const isVersions = pathname.endsWith("/me/bundles");
    const isOverview = pathname.endsWith("/me/bundles/overview");
    if (!isVersions && !isOverview) return harnessFetch(input, init);
    const served = isVersions ? versions : overview;
    const watched = (kind === "versions") === isVersions;
    if (watched) reads.count += 1;
    const body = watched && reads.count === 2 ? invalid : served;
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  return reads;
}

for (const width of [375, 1280]) {
  for (const [name, kind, invalid] of invalidResponses) {
    test(`Bundle ${kind} ${name} preserves a draft through recovery at ${width}px`, async () => {
      const page = app();
      const reads = serveBundleReads(kind, invalid);
      await page.goto("/workspace/downloads");
      const section = page.locator("#bundle-workspace");
      const button = (label: string) => section.locator("button").filter({ hasText: label });
      await page.eventually(() =>
        expect(section.locator(".bundle-overview-header").text()).toContain("已發佈"),
      );
      const form = page.getByRole("region", { name: "建立 Bundle", exact: true });
      await form.locator("summary").click();
      const bundleName = form.locator("input").nth(0);
      const version = form.locator("input").nth(1);
      const description = form.locator("textarea");
      await bundleName.fill("kept-bundle");
      await version.fill("1.2.3");
      await description.fill("保留未送出的 Bundle 說明");
      await button("重新整理").click();
      await page.eventually(() =>
        expect(section.locator("[role=alert]").text()).toContain(
          kind === "versions" ? "暫時無法讀取Bundle 清單" : "暫時無法讀取Bundle 發佈概覽",
        ),
      );
      expect(section.text()).not.toContain("尚未發佈。");
      expect(section.text()).not.toContain("已撤回");
      expect(section.text()).not.toContain("目前還沒有 Bundle。");
      expect(form.locator("summary").text()).not.toBe("建立第一個 Bundle");
      expect(bundleName.value()).toBe("kept-bundle");
      expect(version.value()).toBe("1.2.3");
      expect(description.value()).toBe("保留未送出的 Bundle 說明");
      expect(await page.axeViolations("main")).toEqual([]);
      await button("再試一次").click();
      await page.eventually(() => {
        expect(section.locator(".bundle-overview-header").text()).toContain("已發佈");
        expect(section.locator("[role=alert]").count()).toBe(0);
        expect(page.locator("#bundle-workspace-heading").isFocused()).toBe(true);
      });
      expect(bundleName.value()).toBe("kept-bundle");
      expect(version.value()).toBe("1.2.3");
      expect(description.value()).toBe("保留未送出的 Bundle 說明");
      expect(reads.count).toBe(3);
    });
  }
}
