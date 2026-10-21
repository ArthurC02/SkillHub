import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, OTHER_RUN, SKILL, RUNS, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const source = {
  ...(platformResponse(`http://platform.test/runs/${RUN}`).body as object),
  status: "failed",
  cleanup_status: RUNS.runs[1].cleanup_status,
};

async function stubSource(page: Page, altered: unknown, readable: boolean) {
  const control = { readable, reads: [] as string[] };
  await page.route(`**/runs/${RUN}`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    control.reads.push(control.readable ? "source" : "altered");
    return route.fulfill({ json: control.readable ? source : altered });
  });
  return control;
}

async function capture(page: Page, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(
    info.outputPath(`${state}-source-identity-a11y.json`),
    JSON.stringify(report, null, 2),
  );
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}-source-identity.png`), fullPage: true });
}

async function createVersion(page: Page) {
  await page.route(`**/skills/${SKILL}/versions/from-suggestions`, (route) =>
    route.fulfill({
      status: 201,
      json: {
        skill_id: SKILL,
        version_id: "99999999-1111-4111-8111-111111111111",
        version_number: 3,
        content_hash: "hash-new",
        duplicate: false,
        applied_suggestion_ids: ["s1"],
        rejected_suggestions: [],
      },
    }),
  );
  await page.getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true }).click();
  await expect(
    page.getByRole("link", { name: "以新版本重跑這個測試題", exact: true }),
  ).toBeVisible();
}

test.beforeEach(async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
});

for (const { name, body } of [
  { name: "null", body: null },
  { name: "foreign", body: { ...source, run_id: OTHER_RUN } },
]) {
  for (const phase of ["initial", "applied", "compare"]) {
    test(`a ${name} Run source in ${phase} recovers without substituting or replaying evidence`, async ({
      page,
    }, info) => {
      const control = await stubSource(page, body, phase !== "initial");
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      const nav = page.getByRole("navigation", { name: "這個小工具的工作台", exact: true });
      const apply = page.getByRole("button", {
        name: "以已接受的 1 項建議建立新版本",
        exact: true,
      });
      const rerun = page.getByRole("link", { name: "以新版本重跑這個測試題", exact: true });
      const receipt = page.getByRole("link", { name: "開啟剛建立的版本", exact: true });
      const cleanup = page.locator("#run-trace .badge-row").filter({ hasText: "清理狀態" });
      await page.goto(`/runs/${RUN}${phase === "compare" ? "/compare" : ""}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      if (phase === "compare") {
        await nav.getByRole("link", { name: "總覽", exact: true }).click();
        await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("試跑比較");
        control.readable = false;
        await page.goBack();
      } else if (phase === "applied") {
        await expect(apply).toBeEnabled();
        await createVersion(page);
        control.readable = false;
        await page.getByRole("button", { name: "進階模式", exact: true }).click();
        await page.getByRole("button", { name: "一般模式", exact: true }).click();
      }
      await expect(page.getByRole("alert")).toHaveText(
        `暫時無法讀取${phase === "compare" ? "目前這次試跑" : "這次試跑"}。請再試一次，或稍後再試。`,
      );
      await expect(nav).toHaveCount(0);
      if (phase !== "compare") {
        await expect(page.locator("#run-decision .verdict")).toHaveText("任務判定：未符合");
        await expect(apply).toBeDisabled();
        await expect(page.locator("#suggestion-source-note")).toContainText("試跑來源尚未確認");
        await expect(
          page.getByText("正在讀取這次試跑屬於哪個小工具…", { exact: true }),
        ).toHaveCount(0);
        await expect(cleanup).toHaveCount(0);
        await expect(rerun).toHaveCount(0);
        await expect(page.getByText("這次試跑的測試題草稿已不存在", { exact: false })).toHaveCount(
          0,
        );
        if (phase === "applied") {
          await expect(receipt).toHaveAttribute(
            "href",
            `/skills/${SKILL}/versions/99999999-1111-4111-8111-111111111111`,
          );
          await expect(page.getByText("已建立新版本 #3", { exact: true })).toBeVisible();
        }
      }
      await capture(page, info, "unreadable");
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await expect(retry).toBeFocused();
      control.readable = true;
      await page.keyboard.press("Enter");
      await expect(page.getByRole("alert")).toHaveCount(0);
      await expect(
        page.locator(phase === "compare" ? "#run-comparison-heading" : "#run-heading"),
      ).toBeFocused();
      await expect(nav).toBeVisible();
      if (phase !== "compare") {
        await expect(apply).toBeEnabled();
        await expect(cleanup).toContainText("清理狀態：已清理");
        await expect(page.locator("#run-decision .verdict")).toHaveText("任務判定：未符合");
        if (phase === "applied") await expect(rerun).toBeVisible();
      }
      await capture(page, info, "recovered");
      expect(control.reads).toContain("altered");
      expect(control.reads.at(-1)).toBe("source");
      expect(writes).toEqual(
        phase === "applied"
          ? [expect.stringContaining(`/skills/${SKILL}/versions/from-suggestions`)]
          : [],
      );
    });
  }
}

test("a case-equivalent Run source remains readable with its optional draft absent", async ({
  page,
}, info) => {
  await stubSource(page, { ...source, run_id: RUN.toUpperCase(), test_case_id: undefined }, false);
  const writes: string[] = [];
  page.on("request", (request) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
  });
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await expect(page.getByRole("navigation", { name: "這次試跑的來源", exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true }),
  ).toBeEnabled();
  await expect(page.locator("#run-decision .verdict")).toHaveText("任務判定：未符合");
  await expect(page.getByRole("alert")).toHaveCount(0);
  await capture(page, info, "readable");
  expect(writes).toEqual([]);
});
