import { expect, test } from "vitest";
import { ARTIFACT, OTHER_RUN, PUBLICATION, PUBLISHER } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

test("Activity presents owner-backed work as a dense platform timeline", async () => {
  const page = app();
  await page.goto("/activity");

  await page.eventually(() =>
    expect(page.getByRole("heading", { name: "活動", exact: true }).count()).toBeGreaterThan(0),
  );
  await page.eventually(() =>
    expect(page.locator("dd").filter({ hasText: "5 / 5" }).count()).toBeGreaterThan(0),
  );
  expect(page.getByRole("heading", { name: "需要你處理", exact: true }).count()).toBe(1);
  expect(page.getByRole("heading", { name: "最近完成", exact: true }).count()).toBe(1);
  expect(page.getByRole("link", { name: "查看試跑紀錄" }).attribute("href")).toBe(
    `/runs/${OTHER_RUN}`,
  );
  expect(page.getByRole("link", { name: "查看套件" }).attribute("href")).toBe(
    `/workspace/downloads?artifact=${ARTIFACT}`,
  );
  expect(page.getByRole("link", { name: "查看發佈" }).attribute("href")).toBe(
    `/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`,
  );
});

test("Activity remains readable without horizontal overflow on a phone", async () => {
  const page = app();
  await page.goto("/activity");

  await page.eventually(() =>
    expect(page.getByRole("heading", { name: "活動", exact: true }).count()).toBeGreaterThan(0),
  );
});
