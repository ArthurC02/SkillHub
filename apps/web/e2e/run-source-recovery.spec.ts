import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, SKILL, RUNS, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function capture(page: Page, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(`${state}-source-a11y.json`), JSON.stringify(report, null, 2));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}-source.png`), fullPage: true });
}

async function stubVersionCreation(page: Page) {
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
}

async function expectCreatedVersion(page: Page) {
  await expect(page.getByRole("link", { name: "開啟剛建立的版本", exact: true })).toHaveAttribute(
    "href",
    `/skills/${SKILL}/versions/99999999-1111-4111-8111-111111111111`,
  );
  await expect(page.getByText("已建立新版本 #3", { exact: true })).toBeVisible();
}

for (const status of [403, 503]) {
  for (const phase of ["ready", "applied", "compare"]) {
    test(`${status} Run source recovery preserves ${phase} evidence without stale actions`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let unavailable = false;
      const reads: string[] = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      await page.route(`**/runs/${RUN}`, (route) => {
        if (route.request().resourceType() === "document") return route.fallback();
        reads.push(unavailable ? "unavailable" : "available");
        return route.fulfill(
          unavailable
            ? { status, json: { error: "Run source unavailable" } }
            : {
                json: {
                  ...(platformResponse(route.request().url()).body as object),
                  status: "failed",
                  cleanup_status: RUNS.runs[1].cleanup_status,
                },
              },
        );
      });
      await stubVersionCreation(page);
      const nav = page.getByRole("navigation", { name: "這個小工具的工作台", exact: true });
      const apply = page.getByRole("button", {
        name: "以已接受的 1 項建議建立新版本",
        exact: true,
      });
      const cleanup = page.locator("#run-trace .badge-row").filter({ hasText: "清理狀態" });
      const rerun = page.getByRole("link", { name: "以新版本重跑這個測試題", exact: true });
      const receipt = page.getByRole("link", { name: "開啟剛建立的版本", exact: true });
      const verdict = page.locator("#run-decision .verdict");
      await page.goto(`/runs/${RUN}${phase === "compare" ? "/compare" : ""}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(nav).toBeVisible();
      if (phase === "compare") {
        await nav.getByRole("link", { name: "總覽", exact: true }).click();
        await expect(page).toHaveURL(new RegExp(`/skills/${SKILL}$`));
        await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("試跑比較");
        unavailable = true;
        await page.goBack();
      } else {
        await expect(apply).toBeEnabled();
        await expect(cleanup).toContainText("清理狀態：已清理");
        if (phase === "applied") {
          await apply.click();
          await expect(rerun).toBeVisible();
          expect(writes).toHaveLength(1);
        }
        unavailable = true;
        await page.getByRole("button", { name: "進階模式", exact: true }).click();
        await page.getByRole("button", { name: "一般模式", exact: true }).click();
      }
      await expect(page.getByRole("alert")).toHaveText(
        `暫時無法讀取${phase === "compare" ? "目前這次試跑" : "這次試跑"}。請再試一次，或稍後再試。`,
      );
      await expect(nav).toHaveCount(0);
      if (phase !== "compare") {
        await expect(verdict).toHaveText("任務判定：未符合");
        await expect(cleanup).toHaveCount(0);
        await expect(apply).toBeDisabled();
        await expect(apply).toHaveAttribute("aria-describedby", "suggestion-source-note");
        await expect(page.locator("#suggestion-source-note")).toContainText(
          "試跑來源尚未確認，暫時不能建立新版本。",
        );
        await expect(
          page.getByText("正在讀取這次試跑屬於哪個小工具…", { exact: true }),
        ).toHaveCount(0);
        await expect(rerun).toHaveCount(0);
        await expect(page.getByText("這次試跑的測試題草稿已不存在", { exact: false })).toHaveCount(
          0,
        );
        if (phase === "applied") {
          await expectCreatedVersion(page);
          await expect(
            page.getByText("試跑來源尚未確認，暫時無法提供相同測試題的重跑入口", { exact: false }),
          ).toBeVisible();
        }
      }
      await capture(page, info, "unavailable");
      if (phase !== "compare") {
        await page.getByRole("link", { name: "重新確認試跑來源", exact: true }).focus();
        await page.keyboard.press("Enter");
        await expect(page.locator("#run-heading")).toBeFocused();
      }
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await expect(retry).toBeFocused();
      unavailable = false;
      await page.keyboard.press("Enter");
      await expect(page.getByRole("alert")).toHaveCount(0);
      await expect(
        page.locator(phase === "compare" ? "#run-comparison-heading" : "#run-heading"),
      ).toBeFocused();
      await expect(nav).toBeVisible();
      if (phase !== "compare") {
        await expect(cleanup).toContainText("清理狀態：已清理");
        await expect(apply).toBeEnabled();
        await expect(verdict).toHaveText("任務判定：未符合");
        await expect(page.locator("#suggestion-source-note")).toHaveCount(0);
        if (phase === "applied") {
          await expect(rerun).toBeVisible();
          await expect(receipt).toBeVisible();
        }
      }
      await capture(page, info, "recovered");
      expect(reads).toContain("unavailable");
      expect(reads.at(-1)).toBe("available");
      expect(writes).toEqual(
        phase === "applied"
          ? [expect.stringContaining(`/skills/${SKILL}/versions/from-suggestions`)]
          : [],
      );
    });
  }
}
