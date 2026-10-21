import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { COMPARISON, OTHER_RUN, RUN, RUNS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const SECOND = "7e9b2a00-1111-4111-8111-000000000002";
const OTHER = RUNS.runs.find((run) => run.run_id === OTHER_RUN)!;
const SOURCE = RUNS.runs.find((run) => run.run_id === RUN)!;

async function capture(page: Page, path: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(path, JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
}

for (const fault of ["missing-run-id", "null-verdict"] as const) {
  for (const continuation of [false, true]) {
    test(`comparison recovers ${fault} on ${continuation ? "a continuation page" : "first read"} without losing its draft`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: continuation ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: continuation ? "dark" : "light" });
      let release!: () => void;
      const held = new Promise<void>((resolve) => (release = resolve));
      let corrupt = true;
      const offsets: number[] = [];
      const comparisons: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      await page.route("**/runs?*", async (route) => {
        const params = new URL(route.request().url()).searchParams;
        expect(params.get("test_case_id")).toBe(SOURCE.test_case_id);
        expect(params.get("limit")).toBe("51");
        const offset = Number(params.get("offset"));
        offsets.push(offset);
        if (continuation && offset === 0)
          return route.fulfill({
            json: { runs: [OTHER, ...Array.from({ length: 50 }, () => SOURCE)] },
          });
        await held;
        const valid = { ...OTHER, run_id: continuation ? SECOND : OTHER_RUN };
        const invalid =
          fault === "missing-run-id"
            ? { ...valid, run_id: undefined }
            : { ...valid, evaluation: null };
        return route.fulfill({ json: { runs: [corrupt ? invalid : valid] } });
      });
      await page.route(`**/runs/${RUN}/comparison?*`, (route) => {
        const against = new URL(route.request().url()).searchParams.get("against")!;
        comparisons.push(against);
        return route.fulfill({
          json: JSON.parse(JSON.stringify(COMPARISON).replaceAll(OTHER_RUN, against)),
        });
      });
      await page.goto(`/runs/${RUN}/compare`);
      await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
      const draft = page.getByLabel("要比較的另一個試跑紀錄 ID");
      await draft.fill("清單恢復前保留的比較 ID");
      const rows = page.locator("main .download-list > .download-item");
      if (continuation) {
        await expect(rows).toHaveCount(1);
        await page.getByRole("button", { name: "載入更早的試跑紀錄", exact: true }).click();
      }
      await expect.poll(() => offsets).toEqual(continuation ? [0, 50] : [0]);
      release();
      const alert = page.getByRole("alert").filter({ hasText: "暫時無法讀取可比較的試跑紀錄" });
      await expect(alert).toBeVisible();
      await expect(page.getByRole("heading", { name: "試跑比較", exact: true })).toBeVisible();
      await expect(draft).toHaveValue("清單恢復前保留的比較 ID");
      await expect(rows).toHaveCount(0);
      await expect(
        page.getByText("這個測試題目前只有這一次試跑紀錄", { exact: false }),
      ).toHaveCount(0);
      await expect(page.getByText("這一頁暫時無法顯示", { exact: true })).toHaveCount(0);
      await expect(
        page.getByRole("link", { name: "另開比較基準的試跑結果", exact: true }),
      ).toHaveAttribute("href", `/runs/${RUN}`);
      expect(comparisons).toEqual([]);
      await capture(page, info.outputPath("unreadable-run-list-a11y.json"));
      await page.screenshot({ path: info.outputPath("unreadable-run-list.png"), fullPage: true });
      corrupt = false;
      const retry = page.getByRole("main").getByRole("button", { name: "再試一次", exact: true });
      await expect(retry).toHaveCount(1);
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect(rows).toHaveCount(1);
      await expect(alert).toHaveCount(0);
      await expect(page.locator("#run-comparison-heading")).toBeFocused();
      await expect(draft).toHaveValue("清單恢復前保留的比較 ID");
      if (continuation) {
        await page.getByRole("button", { name: "載入更早的試跑紀錄", exact: true }).click();
        await expect(rows).toHaveCount(2);
      }
      expect(offsets).toEqual(continuation ? [0, 50, 0, 50] : [0, 0]);
      await capture(page, info.outputPath("recovered-run-list-a11y.json"));
      await rows.last().getByRole("button").focus();
      await page.keyboard.press("Enter");
      const against = continuation ? SECOND : OTHER_RUN;
      await expect(page).toHaveURL(`/runs/${RUN}/compare?against=${against}`);
      await expect(
        page.getByRole("table", { name: "試跑紀錄任務判定與執行狀態對比", exact: true }),
      ).toBeVisible();
      expect(comparisons).toEqual([against]);
      expect(writes).toEqual([]);
    });
  }
}

for (const target of [
  {
    path: "/workspace",
    heading: "繼續推進你的工作",
    what: "需要處理的試跑",
    focus: "workspace-attention-heading",
  },
  {
    path: "/workspace/runs",
    heading: "試跑活動",
    what: "試跑活動",
    focus: "workspace-runs-heading",
  },
]) {
  test(`${target.path} recovers an unreadable Run verdict locally`, async ({ page }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: target.path === "/workspace" ? 375 : 1280, height: 900 });
    let corrupt = true;
    let reads = 0;
    const writes: string[] = [];
    page.on("request", (request) => {
      if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
    });
    await page.route("**/runs?*", (route) => {
      reads += 1;
      return route.fulfill({
        json: { runs: [{ ...OTHER, evaluation: corrupt ? null : OTHER.evaluation }] },
      });
    });
    await page.goto(target.path);
    const alert = page.getByRole("alert").filter({ hasText: `暫時無法讀取${target.what}` });
    await expect(alert).toBeVisible();
    await expect(page.getByRole("heading", { name: target.heading, exact: true })).toBeVisible();
    await expect(page.getByText("這一頁暫時無法顯示", { exact: true })).toHaveCount(0);
    await capture(page, info.outputPath("unreadable-consumer-a11y.json"));
    corrupt = false;
    const retry = page.getByRole("main").getByRole("button", { name: "再試一次", exact: true });
    await expect(retry).toHaveCount(1);
    await retry.focus();
    await page.keyboard.press("Enter");
    await expect(alert).toHaveCount(0);
    await expect(page.locator(`#${target.focus}`)).toBeFocused();
    await expect(
      page.getByRole("main").getByText("任務判定：不符合", { exact: true }),
    ).toBeVisible();
    expect(reads).toBe(2);
    expect(writes).toEqual([]);
    await capture(page, info.outputPath("recovered-consumer-a11y.json"));
  });
}
