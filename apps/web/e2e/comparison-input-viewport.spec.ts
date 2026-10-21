import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, RUNS, SKILL, platformResponse } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

async function fieldPosition(field: Locator) {
  return field.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    return {
      top: rect.top,
      bottom: rect.bottom,
      exposed:
        rect.top >= document.querySelector(".app-header")!.getBoundingClientRect().bottom &&
        rect.bottom <= innerHeight &&
        document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2) ===
          element,
    };
  });
}

async function paint(page: Page) {
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
}

for (const dependency of ["self", "candidates", "versions"] as const) {
  for (const paused of [false, true]) {
    for (const width of [375, 1280]) {
      test(`${dependency} recovery after ${paused ? "paused" : "pending"} keeps manual comparison visible at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        const original = RUNS.runs.find((run) => run.run_id !== RUN)!;
        const candidates = {
          runs: Array.from({ length: 12 }, (_, index) => ({
            ...original,
            run_id: `7e9b2a00-1111-4111-8111-${String(index + 1).padStart(12, "0")}`,
          })),
        };
        await page.route("**/runs?*", (route) => route.fulfill({ json: candidates }));
        let phase = "ready";
        let release!: () => void;
        const held = new Promise<void>((resolve) => (release = resolve));
        const reads: string[] = [];
        const writes: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        const pattern =
          dependency === "self"
            ? `**/runs/${RUN}`
            : dependency === "candidates"
              ? "**/runs?*"
              : `**/skills/${SKILL}/versions`;
        await page.route(pattern, async (route) => {
          if (route.request().resourceType() === "document") return route.fallback();
          reads.push(phase);
          if (phase === "error")
            return route.fulfill({ status: 503, json: { error: "service unavailable" } });
          if (phase === "held") await held;
          return route.fulfill({
            json:
              new URL(route.request().url()).pathname === "/runs"
                ? candidates
                : platformResponse(route.request().url()).body,
          });
        });
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        const manual = page.getByText("直接輸入試跑紀錄 ID", { exact: true });
        const nav = page.getByRole("navigation", { name: "這個小工具的工作台", exact: true });
        const rows = page.getByRole("button", { name: /^以 v2、/ });
        try {
          await page.goto(`/runs/${RUN}/compare`);
          await expect(rows).toHaveCount(12);
          await expect(rows.first()).toBeInViewport({ ratio: 1 });
          await expect(field).not.toBeVisible();
          await nav.getByRole("link", { name: "總覽", exact: true }).click();
          await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("試跑比較");
          await expect(
            page
              .getByRole("region", { name: "版本", exact: true })
              .getByText(/共\s*2\s*版，最新 v2/),
          ).toBeVisible();
          phase = "error";
          await page.goBack();
          await expect(page.getByRole("alert")).toBeVisible();
          await manual.focus();
          await page.keyboard.press("Enter");
          await field.fill("尚未送出的比較 ID");
          phase = "held";
          if (paused) await setOffline(page, true);
          await page.getByRole("button", { name: "再試一次", exact: true }).focus();
          await page.keyboard.press("Enter");
          await expect(page.locator("#run-comparison-heading")).toBeFocused();
          await expect(page.getByRole("status")).toContainText(paused ? "等待連線" : "載入");
          if (paused) await setOffline(page, false);
          await expect.poll(() => reads.filter((value) => value === "held").length).toBe(1);
          await field.click();
          await field.fill("繼續輸入的比較 ID");
          await paint(page);
          expect((await fieldPosition(field)).exposed).toBe(true);
          release();
          await expect(nav).toBeVisible();
          await expect(rows).toHaveCount(12);
          await expect(page.getByRole("alert")).toHaveCount(0);
          await expect(field).toBeFocused();
          await expect(field).toHaveValue("繼續輸入的比較 ID");
          await paint(page);
          const position = await fieldPosition(field);
          await writeFile(info.outputPath("restored-field.json"), JSON.stringify(position));
          expect(position.exposed).toBe(true);
          await page.screenshot({ path: info.outputPath("restored-input.png") });
          const report = await new AxeBuilder({ page }).analyze();
          await writeFile(info.outputPath("restored-a11y.json"), JSON.stringify(report));
          expect(report.violations).toEqual([]);
          expect(report.incomplete).toEqual([]);
          await field.fill("");
          await expect(field).toBeVisible();
          await field.fill("保留的比較 ID");
          await manual.focus();
          await page.keyboard.press("Enter");
          await expect(field).not.toBeVisible();
          await page.keyboard.press("Enter");
          await expect(field).toBeVisible();
          await expect(field).toHaveValue("保留的比較 ID");
          expect(reads.filter((value) => value === "held")).toHaveLength(1);
          expect(writes).toEqual([]);
        } finally {
          release();
          await setOffline(page, false);
        }
      });
    }
  }
}
