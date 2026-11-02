import { expect, test } from "vitest";
import { PREVIEW, SKILL, TARGETS, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const packagePage = `/skills/${SKILL}/package?version=${VERSION}`;
const INCLUDE_TESTS = "包含可散布的測試題與範例資料";

function gate() {
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { held, release };
}

test("a failed preview refresh withdraws packaging permission and retries the selected settings", async () => {
  const page = app();
  let failing = false;
  let builds = 0;
  const { held, release } = gate();
  const settings: string[] = [];
  page.route(`**/skills/${SKILL}/versions/${VERSION}/packaging/preview?*`, async ({ url }) => {
    settings.push(url.searchParams.get("include_test_cases")!);
    if (failing) await held;
    return failing ? { status: 503, json: { error: "service unavailable" } } : { json: PREVIEW };
  });
  page.route(`**/skills/${SKILL}/versions/${VERSION}/packaging`, () => {
    builds += 1;
    failing = true;
    return { status: 409, json: { error: "state changed" } };
  });
  await page.goto(packagePage);
  const build = page.getByRole("button", { name: "建立下載套件", exact: true });
  await expect(build).toBeEnabled();
  await page.getByRole("checkbox", { name: INCLUDE_TESTS, exact: true }).check();
  await expect(build).toBeEnabled();
  await build.click();
  try {
    await page.eventually(() => {
      expect(page.getByRole("alert").filter({ hasText: "套件沒有建立成功" }).count()).toBe(1);
      expect(page.getByRole("status").filter({ hasText: "正在重新讀取打包預覽" }).count()).toBe(1);
      expect(build.isDisabled()).toBe(true);
      expect(page.getByText("這些設定可以打包。", { exact: true }).count()).toBe(0);
    });
  } finally {
    release();
  }
  await page.eventually(() => {
    expect(
      page
        .getByRole("alert")
        .filter({ hasText: /打包.*(?:預覽|服務)/ })
        .count(),
    ).toBe(1);
    expect(build.isDisabled()).toBe(true);
    expect(page.getByText("這些設定可以打包。", { exact: true }).count()).toBe(0);
    expect(
      page.getByText("這個部署沒有設定任何打包目標，所以沒有預覽。", { exact: true }).count(),
    ).toBe(0);
  });
  expect(builds).toBe(1);
  failing = false;
  const retry = page.getByRole("button", { name: "再試一次", exact: true });
  await expect(retry).toHaveCount(1);
  await retry.click();
  await page.eventually(() => {
    expect(build.isDisabled()).toBe(false);
    expect(
      page.getByRole("checkbox", { name: INCLUDE_TESTS, exact: true }).element(),
    ).toHaveProperty("checked", true);
    expect(page.locator("#packaging-preview-heading").isFocused()).toBe(true);
  });
  expect(settings.slice(-2)).toEqual(["true", "true"]);
  expect(builds).toBe(1);
  expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
  expect(await page.axeViolations("main")).toEqual([]);
});

test("target refresh failure cannot expose cached choices or authorize a package and retries without losing the inclusion choice", async () => {
  const page = app();
  let failing = false;
  let reads = 0;
  let previewReads = 0;
  page.route(`**/skills/${SKILL}/versions/${VERSION}/packaging/preview?*`, () => {
    previewReads += 1;
    return { json: PREVIEW };
  });
  const { held, release } = gate();
  page.route("**/packaging/targets", async () => {
    reads += 1;
    if (failing) await held;
    return failing ? { status: 503, json: { error: "service unavailable" } } : { json: TARGETS };
  });
  await page.goto(packagePage);
  const build = page.getByRole("button", { name: "建立下載套件", exact: true });
  await expect(build).toBeEnabled();
  const choice = page.getByRole("radio", { name: new RegExp(TARGETS.targets[1].display_name) });
  await choice.check();
  await expect(build).toBeEnabled();
  await page.getByRole("checkbox", { name: INCLUDE_TESTS, exact: true }).check();
  await expect(build).toBeEnabled();
  const beforeRefresh = previewReads;
  failing = true;
  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  try {
    await page.eventually(() => {
      expect(page.getByRole("status").filter({ hasText: "正在重新讀取打包目標" }).count()).toBe(1);
      expect(build.isDisabled()).toBe(true);
      expect((choice.element() as HTMLInputElement).checked).toBe(true);
      expect(choice.isDisabled()).toBe(true);
      expect(
        page.getByText("正在重新確認打包目標，請等讀取完成後再建立套件。", { exact: true }).count(),
      ).toBe(1);
      expect(page.getByText("這些設定可以打包。", { exact: true }).count()).toBe(0);
    });
  } finally {
    release();
  }
  await page.eventually(() => {
    expect(page.getByRole("alert").filter({ hasText: "打包目標" }).count()).toBe(1);
    expect(page.locator(".packaging-targets").count()).toBe(0);
    expect(build.isDisabled()).toBe(true);
    expect(page.getByText("這些設定可以打包。", { exact: true }).count()).toBe(0);
    expect(
      page
        .getByRole("status")
        .filter({ hasText: /(?:計算這些設定|重新讀取打包預覽)/ })
        .count(),
    ).toBe(0);
  });
  expect(previewReads).toBe(beforeRefresh);
  const before = reads;
  failing = false;
  const retry = page.getByRole("button", { name: "再試一次", exact: true });
  await expect(retry).toHaveCount(1);
  await retry.click();
  await page.eventually(() => {
    expect(page.locator(".packaging-targets").count()).toBe(1);
    expect(build.isDisabled()).toBe(false);
    expect((choice.element() as HTMLInputElement).checked).toBe(true);
    expect(
      (
        page
          .getByRole("checkbox", { name: INCLUDE_TESTS, exact: true })
          .element() as HTMLInputElement
      ).checked,
    ).toBe(true);
    expect(page.locator("#packaging-targets-heading").isFocused()).toBe(true);
  });
  expect(reads).toBe(before + 1);
});

test("a removed target requires an explicit replacement before authorizing another package", async () => {
  const page = app();
  let removed = false;
  page.route("**/packaging/targets", () => ({
    json: { targets: removed ? [TARGETS.targets[0]] : TARGETS.targets },
  }));
  await page.goto(packagePage);
  const build = page.getByRole("button", { name: "建立下載套件", exact: true });
  await expect(build).toBeEnabled();
  await page.getByRole("radio", { name: new RegExp(TARGETS.targets[1].display_name) }).check();
  await expect(build).toBeEnabled();
  await page.getByRole("checkbox", { name: INCLUDE_TESTS, exact: true }).check();
  removed = true;
  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  await page.eventually(() => {
    expect(
      page.getByText("選擇的打包目標已不在可用清單，請重新選擇。", { exact: true }).count(),
    ).toBe(1);
    expect(build.isDisabled()).toBe(true);
    expect(page.locator('input[name="packaging-target"]:checked').count()).toBe(0);
  });
  await page.getByRole("radio", { name: new RegExp(TARGETS.targets[0].display_name) }).check();
  await page.eventually(() => {
    expect(build.isDisabled()).toBe(false);
    expect(
      (
        page
          .getByRole("checkbox", { name: INCLUDE_TESTS, exact: true })
          .element() as HTMLInputElement
      ).checked,
    ).toBe(true);
  });
});
