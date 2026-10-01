import { expect, test } from "@playwright/test";
import { ARTIFACT, OTHER_RUN, PUBLICATION, PUBLISHER } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("Activity presents owner-backed work as a dense platform timeline", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/activity");

  await expect(page.getByRole("heading", { level: 1, name: "活動" })).toBeVisible();
  await expect(page.getByRole("definition").filter({ hasText: "5 / 5" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "需要你處理" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 2, name: "最近完成" })).toBeVisible();
  await expect(page.getByRole("link", { name: "查看試跑紀錄" })).toHaveAttribute(
    "href",
    `/runs/${OTHER_RUN}`,
  );
  await expect(page.getByRole("link", { name: "查看套件" })).toHaveAttribute(
    "href",
    `/workspace/downloads?artifact=${ARTIFACT}`,
  );
  await expect(page.getByRole("link", { name: "查看發佈" })).toHaveAttribute(
    "href",
    `/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`,
  );
  await page.screenshot({ path: testInfo.outputPath("activity-desktop.png"), fullPage: true });
});

test("Activity remains readable without horizontal overflow on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto("/activity");

  await expect(page.getByRole("heading", { level: 1, name: "活動" })).toBeVisible();
  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
  await page.screenshot({ path: testInfo.outputPath("activity-phone.png"), fullPage: true });
});
