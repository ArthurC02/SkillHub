import AxeBuilder from "@axe-core/playwright";
import { expect } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { test } from "./socket-diagnostics";
import { ADMIN_RUN_SETTINGS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  for (const kind of ["judge", "runs"] as const) {
    for (const initial of [true, false]) {
      test(`${kind} settings reject an incomplete ${initial ? "initial" : "refreshed"} read and recover at ${width}px`, async ({
        page,
      }) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        let unavailable = initial;
        const writes: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        const path = kind === "judge" ? "/admin/settings" : "/admin/settings/runs";
        await page.route(`**${path}`, (route, request) => {
          if (!["fetch", "xhr"].includes(request.resourceType())) return route.fallback();
          return route.fulfill({
            json:
              kind === "judge"
                ? { judge_panel: { enabled: unavailable ? "false" : false } }
                : {
                    ...ADMIN_RUN_SETTINGS,
                    tokens_per_minute: {
                      ...ADMIN_RUN_SETTINGS.tokens_per_minute,
                      limit: unavailable ? null : 200000,
                    },
                  },
          });
        });
        await page.goto("/admin/settings");
        const current = kind === "judge" ? "關閉：一個判定（預設）" : "目前：試跑的模型金鑰最多";
        if (!initial) {
          await expect(page.getByRole("main")).toContainText(current);
          unavailable = true;
          await page.getByRole("button", { name: "重新整理設定", exact: true }).click();
        }
        const what = kind === "judge" ? "平台設定" : "試跑設定";
        await expect(page.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
        await expect(
          page.getByRole("heading", { level: 1, name: "平台設定", exact: true }),
        ).toBeVisible();
        await expect(
          page.locator(
            kind === "judge" ? "#admin-settings-judge-panel-note" : "#admin-settings-run-tpm-note",
          ),
        ).toHaveCount(0);
        await expect(page.getByRole("main")).not.toContainText(current);
        await expect(page.getByRole("main")).toContainText(
          kind === "judge" ? "目前：3 輪" : "關閉：一個判定（預設）",
        );
        const failed = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(
          test.info().outputPath("settings-unreadable-a11y.json"),
          JSON.stringify(failed),
        );
        expect(failed.violations).toEqual([]);
        expect(failed.incomplete).toEqual([]);
        unavailable = false;
        await page.getByRole("button", { name: `再讀取${what}`, exact: true }).click();
        await expect(page.getByRole("alert")).toHaveCount(0);
        await expect(page.getByRole("main")).toContainText(current);
        await expect(
          page.locator(
            kind === "judge" ? "#admin-settings-judge-panel-note" : "#admin-settings-run-tpm-note",
          ),
        ).toHaveCount(1);
        expect(writes).toEqual([]);
        const recovered = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(
          test.info().outputPath("settings-recovered-a11y.json"),
          JSON.stringify(recovered),
        );
        expect(recovered.violations).toEqual([]);
        expect(recovered.incomplete).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({ path: test.info().outputPath("settings-recovered.png") });
      });
    }
  }
}

for (const width of [375, 1280]) {
  for (const kind of ["judge", "tokens"] as const) {
    test(`an incomplete successful ${kind} setting receipt waits for verification at ${width}px`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      const judge = kind === "judge";
      const read = judge ? "/admin/settings" : "/admin/settings/runs";
      const write = judge
        ? "/admin/settings/judge-panel"
        : "/admin/settings/runs/tokens-per-minute";
      const id = judge ? "admin-settings-judge-panel" : "admin-settings-run-tpm";
      let written = false;
      let recovered = false;
      let reads = 0;
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method()))
          writes.push(new URL(request.url()).pathname);
      });
      await page.route(`**${read}`, (route, request) => {
        if (!["fetch", "xhr"].includes(request.resourceType())) return route.fallback();
        reads++;
        if (written && !recovered)
          return route.fulfill({ status: 503, json: { error: "settings unavailable" } });
        return route.fulfill({
          json: judge
            ? { judge_panel: { enabled: written } }
            : {
                ...ADMIN_RUN_SETTINGS,
                tokens_per_minute: {
                  ...ADMIN_RUN_SETTINGS.tokens_per_minute,
                  limit: written ? 300000 : 200000,
                },
              },
        });
      });
      await page.route(`**${write}`, (route) => {
        written = true;
        return route.fulfill({ json: {} });
      });
      await page.goto("/admin/settings");
      await page
        .getByText(judge ? "調整評審團" : "調整每分鐘 tokens 上限", { exact: true })
        .click();
      if (!judge) await page.locator(`#${id}-value`).fill("300000");
      await page.locator(`#${id}-note`).fill("verify the setting");
      await page
        .getByRole("button", { name: judge ? "開啟評審團" : "改每分鐘上限", exact: true })
        .click();
      await expect(page.locator(`#${id}-result`)).toContainText("目前設定尚未確認");
      await expect(page.locator(`#${id}-result`)).not.toHaveClass(/notice-success/);
      await expect(page.locator(`#${id}-note`)).toHaveCount(0);
      await expect(page.getByRole("alert")).toContainText(
        judge ? "暫時無法讀取平台設定" : "暫時無法讀取試跑設定",
      );
      expect(reads).toBe(2);
      expect(writes).toEqual([write]);
      const unknown = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(
        test.info().outputPath("setting-unverified-a11y.json"),
        JSON.stringify(unknown),
      );
      expect(unknown.violations).toEqual([]);
      expect(unknown.incomplete).toEqual([]);
      recovered = true;
      await page
        .getByRole("button", { name: judge ? "再讀取平台設定" : "再讀取試跑設定", exact: true })
        .click();
      await expect(page.locator(`#${id}-result`)).toHaveClass(/notice-success/);
      await expect(page.locator(`#${id}-result`)).toContainText(
        judge ? "已開啟，需要模型判定的條件" : "已改為每分鐘 300,000 tokens",
      );
      await expect(page.getByRole("alert")).toHaveCount(0);
      expect(reads).toBe(3);
      expect(writes).toEqual([write]);
      const verified = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(
        test.info().outputPath("setting-verified-a11y.json"),
        JSON.stringify(verified),
      );
      expect(verified.violations).toEqual([]);
      expect(verified.incomplete).toEqual([]);
    });
  }
}
