import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  ARTIFACT,
  DOWNLOADS,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const datasets = platformResponse(`/test-cases/${TEST_CASE}/datasets`).body as {
  datasets: unknown[];
  total_bytes: number;
};
const lists = [
  {
    name: "test-case datasets",
    url: `/lab/test-cases/${TEST_CASE}?version=${VERSION}`,
    pattern: `**/test-cases/${TEST_CASE}/datasets`,
    section: '[aria-label="測試資料"].test-case-work-panel',
    items: ".file-tree > li",
    empty: "還沒有上傳任何檔案。",
    body: datasets,
    emptyBody: { datasets: [], total_bytes: 0 },
    heading: "test-case-datasets-heading",
    count: datasets.datasets.length,
  },
  {
    name: "deliveries",
    url: `/workspace/downloads?artifact=${ARTIFACT}`,
    pattern: "**/downloads",
    section: "#delivery-history",
    items: ".download-list > .download-item",
    empty: "還沒有打包過任何套件。",
    body: DOWNLOADS,
    emptyBody: { downloads: [] },
    heading: "delivery-history-heading",
    count: DOWNLOADS.downloads.length,
  },
];

for (const list of lists) {
  for (const empty of [false, true]) {
    test(`${list.name} withdraw cached ${empty ? "empty" : "populated"} claims on read failure and recover in place`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let reads = 0;
      await page.route(list.pattern, (route) => {
        if (route.request().resourceType() === "document") return route.fallback();
        reads += 1;
        return reads === 2
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fulfill({ json: empty && reads === 1 ? list.emptyBody : list.body });
      });
      await page.goto(list.url);
      const section = page.locator(list.section);
      if (empty) await expect(section.getByText(list.empty, { exact: false })).toBeVisible();
      else await expect(section.locator(list.items)).toHaveCount(list.count);
      await page.locator('a[href="/policy"]').first().click();
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
      await page.goBack();
      await expect(section.getByRole("alert")).toContainText("暫時無法讀取");
      await expect(section.locator(list.items)).toHaveCount(0);
      await expect(section.getByText(list.empty, { exact: false })).toHaveCount(0);
      await expect(
        section.getByText("這個工作區目前找不到這筆交付紀錄。", { exact: false }),
      ).toHaveCount(0);
      await expect(section.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
      await section.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(section.locator(list.items)).toHaveCount(list.count);
      await expect(page.locator(`#${list.heading}`)).toBeFocused();
      expect(reads).toBe(3);
      if (list.name === "test-case datasets")
        expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
      expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    });
  }
}

test("download history withdraws cached recipients and retries only its record read", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.route("**/downloads", (route) =>
    route.request().resourceType() === "document"
      ? route.fallback()
      : route.fulfill({ json: { downloads: [DOWNLOADS.downloads[0]] } }),
  );
  let reads = 0;
  let failing = false;
  await page.route(`**/downloads/${ARTIFACT}/records`, (route) => {
    reads += 1;
    return failing
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({
          json: {
            records: [{ actor: "Recipient evidence", downloaded_at: "2026-08-17T09:00:00Z" }],
          },
        });
  });
  await page.goto("/workspace/downloads");
  const disclosure = page
    .locator("#delivery-history details")
    .filter({ has: page.locator("summary").filter({ hasText: "誰下載過" }) });
  await disclosure.locator("summary").click();
  await expect(disclosure.getByText("Recipient evidence", { exact: false })).toBeVisible();
  await disclosure.locator("summary").click();
  failing = true;
  await disclosure.locator("summary").click();
  await expect(disclosure.getByRole("alert")).toContainText("逐筆下載紀錄");
  await expect(disclosure.getByText("Recipient evidence", { exact: false })).toHaveCount(0);
  const before = reads;
  failing = false;
  await expect(disclosure.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await disclosure.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(disclosure.getByText("Recipient evidence", { exact: false })).toBeVisible();
  await expect(disclosure.locator("summary")).toBeFocused();
  expect(reads).toBe(before + 1);
});
