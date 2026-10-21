import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, SKILL, platformResponse } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

async function holdDependency(page: Page, pattern: string) {
  const control = { phase: "ready", reads: [] as string[] };
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  const heldStarted = new Promise<void>((resolve) => (started = resolve));
  await page.route(pattern, async (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    control.reads.push(control.phase);
    if (control.phase === "error")
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    if (control.phase === "held") {
      started();
      await held;
    }
    return route.fulfill({ json: platformResponse(route.request().url()).body });
  });
  return { control, release, heldStarted };
}

async function capture(page: Page, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(`${state}-cached-a11y.json`), JSON.stringify(report, null, 2));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}-cached.png`) });
}

for (const dependency of ["self", "candidates", "versions"] as const) {
  for (const paused of [false, true]) {
    for (const moved of [false, true]) {
      test(`cached comparison ${dependency} ${paused ? "paused" : "pending"} retry ${moved ? "preserves continued input" : "retains heading focus"}`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
        await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
        const pattern =
          dependency === "self"
            ? `**/runs/${RUN}`
            : dependency === "candidates"
              ? "**/runs?*"
              : `**/skills/${SKILL}/versions`;
        const what =
          dependency === "self"
            ? "目前這次試跑"
            : dependency === "candidates"
              ? "可比較的試跑紀錄"
              : "候選試跑紀錄的 Version 編號";
        const { control, release, heldStarted } = await holdDependency(page, pattern);
        const writes: string[] = [];
        const comparisons: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
          if (new URL(request.url()).pathname.endsWith("/comparison"))
            comparisons.push(request.url());
        });
        const nav = page.getByRole("navigation", { name: "這個小工具的工作台", exact: true });
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        const retry = page.getByRole("button", { name: "再試一次", exact: true });
        const heading = page.locator("#run-comparison-heading");
        const status = page.getByRole("status").filter({ hasText: `載入${what}中…` });
        try {
          await page.goto(`/runs/${RUN}/compare`);
          await expect(page.locator(".auth-name")).toHaveText("tester");
          await expect(page.getByRole("button", { name: /^以 v2、/ })).toBeVisible();
          await nav.getByRole("link", { name: "總覽", exact: true }).click();
          await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("試跑比較");
          control.phase = "error";
          await page.goBack();
          await expect(page.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
          if (dependency === "self") await expect(nav).toHaveCount(0);
          else await expect(nav).toBeVisible();
          await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
          await field.fill("尚未送出的比較 ID");
          control.phase = "held";
          if (paused) await setOffline(page, true);
          await retry.focus();
          await page.keyboard.press("Enter");
          await expect(heading).toBeFocused();
          if (paused) {
            await expect(
              page
                .getByRole("status")
                .filter({ hasText: `等待連線以讀取${what}；恢復連線後會繼續讀取。` }),
            ).toBeVisible();
            expect(control.reads).not.toContain("held");
          } else {
            await heldStarted;
            await expect(status).toBeVisible();
          }
          await expect(page.getByRole("alert")).toHaveCount(0);
          await expect(retry).toHaveCount(0);
          await expect(field).toHaveValue("尚未送出的比較 ID");
          if (moved) await field.fill("繼續輸入的比較 ID");
          await capture(page, info, paused ? "paused" : "pending");
          if (paused) await setOffline(page, false);
          await heldStarted;
          await expect(status).toBeVisible();
          await expect(retry).toHaveCount(0);
          expect(control.reads.filter((phase) => phase === "held")).toHaveLength(1);
          release();
          await expect(page.getByRole("button", { name: /^以 v2、/ })).toBeVisible();
          await expect(page.locator("[data-loading]")).toHaveCount(0);
          await expect(page.getByRole("alert")).toHaveCount(0);
          await expect(field).toHaveValue(moved ? "繼續輸入的比較 ID" : "尚未送出的比較 ID");
          if (moved) await expect(field).toBeFocused();
          else await expect(heading).toBeFocused();
          expect(control.reads.filter((phase) => phase === "held")).toHaveLength(1);
          expect(writes).toEqual([]);
          expect(comparisons).toEqual([]);
          await capture(page, info, "recovered");
        } finally {
          release();
          await setOffline(page, false);
        }
      });
    }
  }
}
