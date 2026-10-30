import { expect } from "@playwright/test";
import { test } from "./socket-diagnostics";
import { platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  for (const kind of ["judge", "runs"] as const) {
    for (const initial of [true, false]) {
      test(`${kind} ${initial ? "initial" : "cached"} settings recovery keeps keyboard context at ${width}px`, async ({
        page,
      }) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        let unavailable = initial;
        let held = false;
        let started = false;
        let resume: (() => void) | undefined;
        const writes: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        const path = kind === "judge" ? "/admin/settings" : "/admin/settings/runs";
        await page.route(`**${path}`, async (route, request) => {
          if (!["fetch", "xhr"].includes(request.resourceType())) return route.fallback();
          if (unavailable) return route.fulfill({ status: 503, body: "" });
          if (held) {
            started = true;
            await new Promise<void>((resolve) => {
              resume = resolve;
            });
          }
          return route.fulfill({ json: platformResponse(request.url()).body });
        });
        await page.goto("/admin/settings");
        const main = page.getByRole("main");
        const current = main.getByText(kind === "judge" ? "關閉：一個判定（預設）" : "目前：3 輪", {
          exact: kind === "judge",
        });
        if (!initial) {
          await expect(current).toBeVisible();
          unavailable = true;
          await page.getByRole("button", { name: "重新整理設定", exact: true }).click();
        }
        const what = kind === "judge" ? "平台設定" : "試跑設定";
        const retry = page.getByRole("button", { name: `再讀取${what}`, exact: true });
        await expect(page.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
        await retry.focus();
        await expect(retry).toBeFocused();
        unavailable = false;
        await retry.press("Enter");
        await expect(current).toBeVisible();
        const heading =
          kind === "judge"
            ? "#admin-settings-judge-panel-heading"
            : "#admin-settings-rounds-heading";
        await expect(page.locator(initial ? "#main-content" : heading)).toBeFocused();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.keyboard.press("Tab");
        expect(
          await page.evaluate(() =>
            document.querySelector("main")?.contains(document.activeElement),
          ),
        ).toBe(true);

        unavailable = true;
        await page.getByRole("button", { name: "重新整理設定", exact: true }).click();
        await expect(page.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
        unavailable = false;
        held = true;
        const laterControl = page.getByRole("link", { name: "平台 Agent", exact: true });
        try {
          await retry.focus();
          await retry.press("Enter");
          await expect.poll(() => started).toBe(true);
          await expect(page.locator(heading)).toBeFocused();
          await laterControl.focus();
          await expect(laterControl).toBeFocused();
        } finally {
          resume?.();
        }
        await expect(current).toBeVisible();
        await expect(laterControl).toBeFocused();
        expect(writes).toEqual([]);
      });
    }
  }
}
