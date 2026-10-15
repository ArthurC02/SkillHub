import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { SKILL, VERSION, TEST_CASE, RUN, SKILL_VERSIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const [name, body] of [
  ["null list", { versions: null }],
  ["null item", { versions: [null] }],
  ["wrong order", { versions: [...SKILL_VERSIONS.versions].reverse() }],
] as const) {
  test(`a ${name} version response remains unreadable and recovers without claiming latest or no access`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    const writes: string[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.on("request", (request) => {
      if (request.method() !== "GET") writes.push(request.url());
    });
    const cached = name === "wrong order";
    await page.route(`**/skills/${SKILL}/versions`, async (route) => {
      reads += 1;
      if (cached && reads === 1) return route.fulfill({ json: SKILL_VERSIONS });
      if (reads === (cached ? 2 : 1)) return route.fulfill({ json: body });
      await gate;
      return route.fulfill({ json: SKILL_VERSIONS });
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      const history = page.getByRole("region", { name: "版本", exact: true });
      if (cached) {
        await expect(history).toContainText("共 2 版，最新 v2");
        await history.getByRole("button", { name: "重新整理", exact: true }).click();
      }
      await expect(history.getByRole("alert")).toContainText("暫時無法讀取版本歷史");
      await expect(history.getByText("最新", { exact: false })).toHaveCount(0);
      await expect(history.getByText("無權檢視", { exact: false })).toHaveCount(0);
      await history.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => reads).toBe(cached ? 3 : 2);
      await expect(page.locator("#version-history-heading")).toBeFocused();
      await expect(history.getByRole("status")).toContainText("載入版本歷史中");
      release();
      await expect(history).toContainText("共 2 版，最新 v2");
      await expect(history.getByRole("alert")).toHaveCount(0);
      await expect(page.locator("#version-history-heading")).toBeFocused();
      expect(writes).toEqual([]);
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: test.info().outputPath("version-response-recovered.png") });
    } finally {
      release();
    }
  });
}

test("an invalid version number blocks preflight without losing the selected version or starting a Run", async ({
  page,
}) => {
  await stubPlatform(page);
  let reads = 0;
  let summaries = 0;
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
    if (new URL(request.url()).pathname.endsWith("/runs/preflight")) summaries += 1;
  });
  await page.route(`**/skills/${SKILL}/versions`, (route) => {
    reads += 1;
    return route.fulfill({
      json:
        reads === 1
          ? { versions: [{ ...SKILL_VERSIONS.versions[0], version_number: "2" }] }
          : SKILL_VERSIONS,
    });
  });
  await page.goto(
    `/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}&against=${RUN}`,
  );
  const picker = page.getByRole("group", { name: "小工具版本選擇", exact: true });
  await expect(picker.getByRole("alert")).toContainText("暫時無法讀取版本清單");
  await expect(picker.getByRole("combobox")).toHaveValue(VERSION);
  await expect(picker.getByRole("combobox")).toBeDisabled();
  expect(summaries).toBe(0);
  await picker.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }),
  ).toBeEnabled();
  await expect(picker.getByRole("combobox")).toHaveValue(VERSION);
  await expect(picker).toBeFocused();
  expect(new URL(page.url()).searchParams.get("against")).toBe(RUN);
  expect(reads).toBe(2);
  expect(summaries).toBe(1);
  expect(writes).toEqual([]);
});

test("an incomplete embedded version list preserves the Bundle draft and recovers member choices without writing", async ({
  page,
}) => {
  await stubPlatform(page);
  let reads = 0;
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  await page.route("**/me/bundles", (route) => route.fulfill({ json: { bundles: [] } }));
  await page.route(`**/skills/${SKILL}/versions`, (route) => {
    reads += 1;
    return route.fulfill({ json: reads === 1 ? {} : SKILL_VERSIONS });
  });
  await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
  const section = page.locator("#bundle-workspace");
  await expect(section.getByRole("alert")).toContainText("可加入 Bundle 的版本");
  const form = section.locator(".bundle-form");
  await form.getByLabel("名稱", { exact: true }).fill("draft-bundle");
  await form.getByLabel("說明", { exact: true }).fill("保留這段說明");
  await section.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(section.getByRole("alert")).toHaveCount(0);
  await expect(form.getByLabel("名稱", { exact: true })).toHaveValue("draft-bundle");
  await expect(form.getByLabel("說明", { exact: true })).toHaveValue("保留這段說明");
  await expect(form.locator(`[data-bundle-version="${VERSION}"]`)).toHaveValue(VERSION);
  expect(reads).toBe(2);
  expect(writes).toEqual([]);
});
