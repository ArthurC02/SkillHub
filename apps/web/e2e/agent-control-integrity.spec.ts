import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { test } from "./socket-diagnostics";
import { ADMIN_AGENTS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function capture(page: Page, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(`${state}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}.png`) });
}

for (const width of [375, 1280]) {
  for (const fault of ["missing list", "null brake"] as const) {
    test(`${fault} Agent read recovers without fabricated controls at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      let recover = false;
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      await page.route("**/admin/agents", (route, request) => {
        if (request.resourceType() === "document") return route.fallback();
        return route.fulfill({
          json: recover
            ? ADMIN_AGENTS
            : fault === "missing list"
              ? {}
              : { ...ADMIN_AGENTS, brake: null },
        });
      });
      await page.goto("/admin/agents");
      await expect(page.getByRole("alert")).toContainText("暫時無法讀取 Agent 控制");
      await expect(page.getByRole("navigation", { name: "平台 Agent 工作區" })).toContainText(
        "煞車 無法取得",
      );
      await expect(page.getByRole("button", { name: "拉下 Agent 煞車", exact: true })).toHaveCount(
        0,
      );
      await expect(
        page.getByRole("button", { name: "停用 daily-report", exact: true }),
      ).toHaveCount(0);
      await capture(page, info, "unreadable");
      recover = true;
      await page.getByRole("button", { name: "再試一次", exact: true }).press("Enter");
      await expect(page.getByRole("alert")).toHaveCount(0);
      await expect(page.locator("#admin-agent-brake-heading")).toBeFocused();
      await expect(
        page.getByRole("button", { name: "停用 daily-report", exact: true }),
      ).toBeVisible();
      await capture(page, info, "recovered");
      expect(writes).toEqual([]);
    });
  }

  for (const receipt of ["valid", "null", "empty", "other"] as const) {
    for (const action of ["set", "restore"] as const) {
      test(`${receipt} Agent cap receipt verifies ${action} without replay at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        const errors: string[] = [];
        const writes: unknown[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        const before = {
          ...ADMIN_AGENTS.agents[0],
          daily_spend_cap_usd_micros: action === "set" ? 2_000_000 : 1_000_000,
          daily_spend_cap_overridden: true,
        };
        const after =
          action === "restore"
            ? ADMIN_AGENTS.agents[0]
            : { ...before, daily_spend_cap_usd_micros: 2_000_000 };
        let sent = false;
        let recover = false;
        await page.route("**/admin/agents", (route, request) => {
          if (request.resourceType() === "document") return route.fallback();
          return route.fulfill({
            json: { agents: [sent ? (recover ? after : { ...after, enabled: "false" }) : before] },
          });
        });
        await page.route("**/admin/agents/daily-report/spend-cap", (route, request) => {
          writes.push(request.postDataJSON());
          sent = true;
          return route.fulfill({
            json:
              receipt === "valid"
                ? after
                : receipt === "null"
                  ? null
                  : receipt === "empty"
                    ? {}
                    : { ...after, name: "another-agent", daily_spend_cap_usd_micros: 4_000_000 },
          });
        });
        await page.goto("/admin/agents");
        if (action === "set") {
          await page.getByText("調整每日花費上限", { exact: true }).click();
          await page.locator("#admin-agent-daily-report-cap").fill("2.00");
        } else {
          await page.getByText("恢復預設上限（$0.2000）", { exact: true }).click();
        }
        const note = page.locator(
          `#admin-agent-daily-report-cap-${action === "set" ? "set" : "clear"}-note`,
        );
        await note.fill("verify the cap");
        await page
          .getByRole("button", {
            name:
              action === "set"
                ? "改 daily-report 的每日上限"
                : "把 daily-report 的上限改回預設 $0.2000",
            exact: true,
          })
          .press("Enter");
        const result = page.locator("#admin-agent-daily-report-cap-result");
        await expect(result).toHaveText(
          "daily-report 的每日花費上限的變更請求已完成；最新上限尚未確認。",
        );
        await expect(result).toBeFocused();
        await expect(result).not.toHaveClass(/notice-success/);
        await expect(page.getByRole("alert")).toContainText("暫時無法讀取 Agent 控制");
        await expect(page.locator("#admin-agent-daily-report-cap")).toBeHidden();
        await capture(page, info, "unverified");
        recover = true;
        await page
          .getByRole("button", { name: "重新整理此 Agent 的上限", exact: true })
          .press("Enter");
        await expect(result).toContainText(
          action === "restore" ? "目前顯示預設 $0.2000" : "目前顯示營運者設定 $2.0000",
        );
        await expect(result).toBeFocused();
        await expect(page.getByRole("alert")).toHaveCount(0);
        await expect(result).not.toContainText("another-agent");
        await expect(result).not.toContainText("$4.0000");
        if (action === "restore") await page.getByText("調整每日花費上限", { exact: true }).click();
        await expect(page.locator("#admin-agent-daily-report-cap")).toHaveValue(
          action === "restore" ? "0.2" : "2",
        );
        await expect(page.locator("#admin-agent-daily-report-cap")).toBeEditable();
        await capture(page, info, "verified");
        if (action === "set") {
          await expect(
            page.getByRole("button", { name: "改 daily-report 的每日上限", exact: true }),
          ).toBeDisabled();
        }
        await page.locator("#admin-agent-daily-report-cap").fill("3");
        await page.locator("#admin-agent-daily-report-cap-set-note").fill("review latest");
        await expect(result).toHaveCount(0);
        await expect(
          page.getByRole("button", { name: "改 daily-report 的每日上限", exact: true }),
        ).toBeEnabled();
        expect(writes).toEqual([
          {
            daily_spend_cap_usd_micros: action === "restore" ? null : 2_000_000,
            note: "verify the cap",
          },
        ]);
        expect(errors).toEqual([]);
      });
    }
  }
}
