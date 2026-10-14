import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { PREVIEW, SKILL, TARGETS, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("a failed preview refresh withdraws packaging permission and retries the selected settings", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let failing = false;
  let builds = 0;
  let releaseRead!: () => void;
  const held = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  const settings: string[] = [];
  await page.route(`**/skills/${SKILL}/versions/${VERSION}/packaging/preview?*`, async (route) => {
    settings.push(new URL(route.request().url()).searchParams.get("include_test_cases")!);
    if (failing) await held;
    return failing
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: PREVIEW });
  });
  await page.route(`**/skills/${SKILL}/versions/${VERSION}/packaging`, (route) => {
    builds += 1;
    failing = true;
    return route.fulfill({ status: 409, json: { error: "state changed" } });
  });
  await page.goto(`/skills/${SKILL}/package?version=${VERSION}`);
  const build = page.getByRole("button", { name: "建立下載套件", exact: true });
  await expect(build).toBeEnabled();
  await page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }).check();
  await expect(build).toBeEnabled();
  await build.click();
  try {
    await expect(page.getByRole("alert").filter({ hasText: "套件沒有建立成功" })).toBeVisible();
    await expect(
      page.getByRole("status").filter({ hasText: "正在重新讀取打包預覽" }),
    ).toBeVisible();
    await expect(build).toBeDisabled();
    await expect(page.getByText("這些設定可以打包。", { exact: true })).toHaveCount(0);
  } finally {
    releaseRead();
  }
  await expect(page.getByRole("alert").filter({ hasText: /打包.*(?:預覽|服務)/ })).toBeVisible();
  await expect(build).toBeDisabled();
  await expect(page.getByText("這些設定可以打包。", { exact: true })).toHaveCount(0);
  await expect(
    page.getByText("這個部署沒有設定任何打包目標，所以沒有預覽。", { exact: true }),
  ).toHaveCount(0);
  expect(builds).toBe(1);
  failing = false;
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(build).toBeEnabled();
  await expect(
    page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }),
  ).toBeChecked();
  await expect(page.locator("#packaging-preview-heading")).toBeFocused();
  expect(settings.slice(-2)).toEqual(["true", "true"]);
  expect(builds).toBe(1);
  expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
});

test("target refresh failure cannot expose cached choices or authorize a package and retries without losing the inclusion choice", async ({
  page,
}) => {
  await stubPlatform(page);
  let failing = false;
  let reads = 0;
  let previewReads = 0;
  await page.route(`**/skills/${SKILL}/versions/${VERSION}/packaging/preview?*`, (route) => {
    previewReads += 1;
    return route.fulfill({ json: PREVIEW });
  });
  let releaseRead!: () => void;
  const held = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  await page.route("**/packaging/targets", async (route) => {
    reads += 1;
    if (failing) await held;
    return failing
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: TARGETS });
  });
  await page.goto(`/skills/${SKILL}/package?version=${VERSION}`);
  const build = page.getByRole("button", { name: "建立下載套件", exact: true });
  await expect(build).toBeEnabled();
  const choice = page.getByRole("radio", { name: new RegExp(TARGETS.targets[1].display_name) });
  await choice.check();
  await expect(build).toBeEnabled();
  await page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }).check();
  await expect(build).toBeEnabled();
  const beforeRefresh = previewReads;
  failing = true;
  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  try {
    await expect(
      page.getByRole("status").filter({ hasText: "正在重新讀取打包目標" }),
    ).toBeVisible();
    await expect(build).toBeDisabled();
    await expect(choice).toBeChecked();
    await expect(choice).toBeDisabled();
    await expect(
      page.getByText("正在重新確認打包目標，請等讀取完成後再建立套件。", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("這些設定可以打包。", { exact: true })).toHaveCount(0);
  } finally {
    releaseRead();
  }
  await expect(page.getByRole("alert").filter({ hasText: "打包目標" })).toBeVisible();
  await expect(page.locator(".packaging-targets")).toHaveCount(0);
  await expect(build).toBeDisabled();
  await expect(page.getByText("這些設定可以打包。", { exact: true })).toHaveCount(0);
  await expect(
    page.getByRole("status").filter({ hasText: /(?:計算這些設定|重新讀取打包預覽)/ }),
  ).toHaveCount(0);
  expect(previewReads).toBe(beforeRefresh);
  const before = reads;
  failing = false;
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(page.locator(".packaging-targets")).toBeVisible();
  await expect(build).toBeEnabled();
  await expect(choice).toBeChecked();
  await expect(
    page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }),
  ).toBeChecked();
  await expect(page.locator("#packaging-targets-heading")).toBeFocused();
  expect(reads).toBe(before + 1);
});

test("a removed target requires an explicit replacement before authorizing another package", async ({
  page,
}) => {
  await stubPlatform(page);
  let removed = false;
  await page.route("**/packaging/targets", (route) =>
    route.fulfill({ json: { targets: removed ? [TARGETS.targets[0]] : TARGETS.targets } }),
  );
  await page.goto(`/skills/${SKILL}/package?version=${VERSION}`);
  const build = page.getByRole("button", { name: "建立下載套件", exact: true });
  await expect(build).toBeEnabled();
  await page.getByRole("radio", { name: new RegExp(TARGETS.targets[1].display_name) }).check();
  await expect(build).toBeEnabled();
  await page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }).check();
  removed = true;
  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  await expect(
    page.getByText("選擇的打包目標已不在可用清單，請重新選擇。", { exact: true }),
  ).toBeVisible();
  await expect(build).toBeDisabled();
  await expect(page.locator('input[name="packaging-target"]:checked')).toHaveCount(0);
  await page.getByRole("radio", { name: new RegExp(TARGETS.targets[0].display_name) }).check();
  await expect(build).toBeEnabled();
  await expect(
    page.getByRole("checkbox", { name: "包含可散布的測試題與範例資料", exact: true }),
  ).toBeChecked();
});
