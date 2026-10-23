import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  CHECK_KINDS,
  PLATFORM_GRADER,
  TEST_CASE,
  VERSION,
  WORKSPACE_GRADER,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ viewport: { width: 375, height: 900 }, trace: "retain-on-failure" });

async function inspect(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  for (const review of report.incomplete) {
    expect(review.id).toBe("color-contrast");
    for (const node of review.nodes) {
      expect(node.target).toHaveLength(1);
      expect(node.any.every((check) => check.data?.messageKey === "bgOverlap")).toBe(true);
      const overlapped = page.locator(String(node.target[0]));
      await overlapped.evaluate((element) => element.scrollIntoView({ block: "center" }));
      const verification = await overlapped.evaluate((element) => {
        const colors = Reflect.get(window, "axe").commons.color;
        const style = getComputedStyle(element);
        const foreground = new colors.Color();
        const background = new colors.Color();
        foreground.parseString(style.color);
        background.parseString(style.backgroundColor);
        const range = document.createRange();
        range.selectNodeContents(element);
        const hits = Array.from(range.getClientRects(), (rect) =>
          element.contains(
            document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2),
          ),
        );
        return { ratio: colors.getContrast(background, foreground), alpha: background.alpha, hits };
      });
      await writeFile(
        test.info().outputPath(`${label}-contrast-verification.json`),
        JSON.stringify(verification),
      );
      expect(verification.alpha).toBe(1);
      expect(verification.ratio).toBeGreaterThanOrEqual(4.5);
      expect(verification.hits.length).toBeGreaterThan(0);
      expect(verification.hits.every(Boolean)).toBe(true);
    }
  }
  await page.screenshot({ path: test.info().outputPath(`${label}-viewport.png`) });
}

for (const resource of ["kinds", "graders"] as const) {
  for (const colorScheme of ["light", "dark"] as const) {
    test(`an offline ${resource} retry explains the wait and preserves the criterion in ${colorScheme} mode`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme });
      await stubPlatform(page);
      const path = resource === "kinds" ? "/evaluation/check-kinds" : "/evaluation/graders";
      const what = resource === "kinds" ? "檢查方式" : "評分器版本清單";
      let reads = 0;
      const writes: string[] = [];
      page.on("request", (request) => {
        if (
          request.method() !== "GET" &&
          new URL(request.url()).pathname.startsWith("/test-cases/")
        ) {
          writes.push(request.url());
        }
      });
      await page.route(`**${path}`, (route) => {
        reads++;
        return reads === 1
          ? route.fulfill({ status: 503, json: { error: "unavailable" } })
          : route.fulfill({
              json:
                resource === "kinds"
                  ? CHECK_KINDS
                  : { graders: [PLATFORM_GRADER, WORKSPACE_GRADER] },
            });
      });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
        const draft = page.getByLabel("新增驗收條件", { exact: true });
        await draft.fill("保留這份離線前的驗收條件");
        if (resource === "graders") {
          await page.getByLabel("從哪一種節點開始", { exact: true }).selectOption("prompt_grader");
          await page.getByRole("button", { name: "開始設計", exact: true }).click();
        }
        const palette = page.getByRole("group", { name: "檢查方式與設計", exact: true });
        await expect(palette.getByRole("alert")).toBeVisible();
        expect(reads).toBe(1);
        await page.context().setOffline(true);
        await palette.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect(palette.getByRole("alert")).toHaveCount(0);
        const paused = {
          online: await page.evaluate(() => navigator.onLine),
          reads,
          statuses: await palette.getByRole("status").allTextContents(),
          skeletons: await palette.locator(".loading-skeleton").count(),
        };
        await writeFile(
          test.info().outputPath("palette-offline-state.json"),
          JSON.stringify(paused),
        );
        expect(paused.online).toBe(false);
        await palette.getByRole("status").filter({ hasText: what }).scrollIntoViewIfNeeded();
        await page.screenshot({
          path: test.info().outputPath("palette-offline-proof-viewport.png"),
        });
        const waiting = palette.getByRole("status").filter({ hasText: `等待連線以讀取${what}` });
        await expect(waiting).toContainText("恢復連線後會繼續讀取。");
        await expect(palette.locator(".loading-skeleton")).toHaveCount(0);
        await expect(palette.getByRole("alert")).toHaveCount(0);
        await expect(draft).toHaveValue("保留這份離線前的驗收條件");
        if (resource === "graders") {
          await expect(page.getByLabel("評分器版本", { exact: true })).toBeDisabled();
          await expect(page.getByRole("button", { name: "新增", exact: true })).toBeDisabled();
        }
        expect(reads).toBe(1);
        expect(writes).toEqual([]);
        await inspect(page, "palette-paused");
        await page.context().setOffline(false);
        if (resource === "kinds") {
          await expect(page.getByLabel("從哪一種節點開始", { exact: true })).toBeVisible();
        } else {
          const picker = page.getByLabel("評分器版本", { exact: true });
          await expect(picker).toBeEnabled();
          await picker.selectOption(WORKSPACE_GRADER.version_id);
          await expect(picker).toHaveValue(WORKSPACE_GRADER.version_id);
          await expect(page.getByRole("button", { name: "新增", exact: true })).toBeEnabled();
        }
        await expect(waiting).toHaveCount(0);
        await expect(draft).toHaveValue("保留這份離線前的驗收條件");
        expect(reads).toBe(2);
        expect(writes).toEqual([]);
        await inspect(page, "palette-recovered");
      } finally {
        await page.context().setOffline(false);
      }
    });
  }
}
