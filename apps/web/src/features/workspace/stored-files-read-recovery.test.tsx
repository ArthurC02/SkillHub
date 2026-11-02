import { expect, test } from "vitest";
import {
  ARTIFACT,
  DOWNLOADS,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

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
    empty: "目前沒有可顯示的套件。",
    body: DOWNLOADS,
    emptyBody: { downloads: [] },
    heading: "delivery-history-heading",
    count: DOWNLOADS.downloads.length,
  },
];

for (const list of lists) {
  for (const empty of [false, true]) {
    test(`${list.name} withdraw cached ${empty ? "empty" : "populated"} claims on read failure and recover in place`, async () => {
      const page = app();
      let reads = 0;
      page.route(list.pattern, () => {
        reads += 1;
        return reads === 2
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: empty && reads === 1 ? list.emptyBody : list.body };
      });
      await page.goto(list.url);
      const section = page.locator(list.section);
      if (empty) await expect(section.getByText(list.empty)).toBeVisible();
      else await expect(section.locator(list.items)).toHaveCount(list.count);
      await page.locator('a[href="/policy"]').first().click();
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
      await page.goBack();
      await page.eventually(() => {
        expect(section.getByRole("alert").text()).toContain("暫時無法讀取");
        expect(section.locator(list.items).count()).toBe(0);
        expect(section.getByText(list.empty).count()).toBe(0);
        expect(section.getByText("這個工作區目前找不到這筆交付紀錄。").count()).toBe(0);
        expect(section.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(1);
      });
      await section.getByRole("button", { name: "再試一次", exact: true }).click();
      await page.eventually(() => {
        expect(section.locator(list.items).count()).toBe(list.count);
        expect(page.locator(`#${list.heading}`).isFocused()).toBe(true);
      });
      expect(reads).toBe(3);
      if (list.name === "test-case datasets")
        expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}

test("download history withdraws cached recipients and retries only its record read", async () => {
  const page = app();
  page.route("**/downloads", () => ({ json: { downloads: [DOWNLOADS.downloads[0]] } }));
  let reads = 0;
  let failing = false;
  page.route(`**/downloads/${ARTIFACT}/records`, () => {
    reads += 1;
    return failing
      ? { status: 503, json: { error: "service unavailable" } }
      : {
          json: {
            records: [{ actor: "Recipient evidence", downloaded_at: "2026-08-17T09:00:00Z" }],
          },
        };
  });
  await page.goto("/workspace/downloads");
  const disclosure = page.locator("#delivery-history details").filter({ hasText: "誰下載過" });
  await expect(disclosure).toHaveCount(1);
  const summary = disclosure.locator("summary");
  await summary.click();
  await expect(disclosure.getByText("Recipient evidence")).toBeVisible();
  await summary.click();
  failing = true;
  await summary.click();
  await page.eventually(() => {
    expect(disclosure.getByRole("alert").text()).toContain("逐筆下載紀錄");
    expect(disclosure.getByText("Recipient evidence").count()).toBe(0);
  });
  const before = reads;
  failing = false;
  const retry = disclosure.getByRole("button", { name: "再試一次", exact: true });
  await expect(retry).toHaveCount(1);
  await retry.click();
  await page.eventually(() => {
    expect(disclosure.getByText("Recipient evidence").count()).toBeGreaterThan(0);
    expect(summary.isFocused()).toBe(true);
  });
  expect(reads).toBe(before + 1);
});
