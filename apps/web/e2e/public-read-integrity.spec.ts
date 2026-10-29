import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";
import { test } from "./socket-diagnostics";
import { writeFile } from "node:fs/promises";
import {
  PUBLIC_BUNDLE_PUBLICATION,
  PUBLIC_PUBLICATION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { serveStatic, STATIC_KINDS } from "./stub";

async function capture(page: Page, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(test.info().outputPath(state + "-public-read-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(state + "-public-read.png") });
}

for (const kind of ["skill", "bundle"] as const) {
  for (const mode of [
    "null",
    "incomplete evidence",
    "availability reread404",
    "availability reread409",
  ] as const) {
    test(`a ${kind} public read with ${mode} withholds unknown content and retries only its read`, async ({
      page,
    }) => {
      const initial = kind === "skill" ? PUBLIC_PUBLICATION : PUBLIC_BUNDLE_PUBLICATION;
      const incomplete =
        kind === "skill"
          ? {
              ...PUBLIC_PUBLICATION,
              release: { ...PUBLIC_PUBLICATION.release, findings: null },
            }
          : {
              ...PUBLIC_BUNDLE_PUBLICATION,
              bundle_release: { ...PUBLIC_BUNDLE_PUBLICATION.bundle_release, findings: null },
            };
      const address = `/publications/${initial.publisher}/${initial.name}`;
      const rechecking = mode.startsWith("availability reread");
      const malformedRead = rechecking ? 2 : 1;
      let reads = 0;
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(new URL(request.url()).pathname);
      });
      await page.route("**/*", (route, request) => {
        if (STATIC_KINDS.has(request.resourceType())) return serveStatic(route);
        const path = new URL(request.url()).pathname;
        if (path === address + "/acquisitions" && request.method() === "POST")
          return route.fulfill({
            status: mode.endsWith("404") ? 404 : 409,
            json: mode.endsWith("404")
              ? { error: "not found" }
              : {
                  error: "作者撤回了這個發佈物，這一頁不再提供它的內容。",
                  reason: "delisted",
                },
          });
        if (path === address) {
          reads++;
          if (reads === malformedRead)
            return route.fulfill({
              json: mode === "incomplete evidence" ? incomplete : null,
            });
          return route.fulfill({ json: initial });
        }
        const response = platformResponse(request.url());
        return route.fulfill({ status: response.status, json: response.body });
      });
      await page.setViewportSize({ width: kind === "skill" ? 375 : 1280, height: 900 });
      await page.goto(initial.address);
      if (rechecking) await page.getByRole("button", { name: "下載", exact: true }).click();
      const main = page.getByRole("main");
      await expect(main.getByRole("heading", { level: 1, name: "公開發佈物" })).toBeVisible();
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取這個發佈物");
      await expect(main).toBeFocused();
      await expect(page.locator(".publication-trust")).toHaveCount(0);
      await expect(main.getByRole("button", { name: "下載", exact: true })).toHaveCount(0);
      await expect(main.getByText("作者已撤回", { exact: true })).toHaveCount(0);
      expect(reads).toBe(malformedRead);
      await capture(page, "failed");
      await main.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(page.locator(".publication-trust")).toBeVisible();
      await expect(main.getByRole("button", { name: "下載", exact: true })).toBeVisible();
      await expect(main.getByRole("alert").filter({ hasText: "暫時無法讀取" })).toHaveCount(0);
      await expect(main).toBeFocused();
      expect(new URL(page.url()).pathname).toBe(initial.address);
      expect(reads).toBe(malformedRead + 1);
      expect(writes).toEqual(rechecking ? [address + "/acquisitions"] : []);
      await capture(page, "recovered");
      if (rechecking) {
        await main.getByRole("button", { name: "下載", exact: true }).focus();
        await page.keyboard.press("Enter");
        await expect(
          main
            .getByRole("alert")
            .filter({ hasText: mode.endsWith("404") ? "下載沒有成功" : "作者撤回" }),
        ).toBeFocused();
        expect(reads).toBe(malformedRead + 2);
        expect(writes).toEqual([address + "/acquisitions", address + "/acquisitions"]);
      }
    });
  }
}
