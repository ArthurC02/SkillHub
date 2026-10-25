import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import {
  ADMIN_AGENTS,
  ADMIN_MODEL_BUDGETS,
  ADMIN_RUN_SETTINGS,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const SETTINGS = [
  {
    kind: "rounds",
    page: "/admin/settings",
    read: "/admin/settings/runs",
    write: "/admin/settings/runs/continuation-rounds",
    selector: "#admin-settings-rounds-value",
    note: "#admin-settings-rounds-note",
    summary: "調整回答輪數上限",
    submit: "改輪數",
    receipt: "#admin-settings-rounds-result",
    original: "3",
    changed: "5",
    edited: "7",
    fact: "目前：3 輪",
    body: { rounds: 3, note: "confirm current value" },
  },
  {
    kind: "timeout",
    page: "/admin/model-budgets",
    read: "/admin/model-budgets",
    write: "/admin/model-budgets/judge-run",
    selector: "#admin-budget-judge-run-seconds",
    note: "#admin-budget-judge-run-note",
    summary: "調整秒數",
    submit: "改 評估判定 的秒數",
    receipt: "#admin-budget-result",
    original: "90",
    changed: "120",
    edited: "100",
    fact: "目前：90 秒（已調整）",
    body: { seconds: 90, reason: "confirm current value" },
  },
  {
    kind: "cap",
    page: "/admin/agents",
    read: "/admin/agents",
    write: "/admin/agents/daily-report/spend-cap",
    selector: "#admin-agent-daily-report-cap",
    note: "#admin-agent-daily-report-cap-set-note",
    summary: "調整每日花費上限",
    submit: "改 daily-report 的每日上限",
    receipt: "#admin-agent-daily-report-cap-result",
    original: "0.2",
    changed: "1",
    edited: "2.5",
    fact: "每日花費上限 $0.2000",
    body: { daily_spend_cap_usd_micros: 200000, note: "confirm current value" },
  },
] as const;

async function controlSetting(page: Page, setting: (typeof SETTINGS)[number]) {
  let value = setting.original as string;
  let enabled = true;
  let overridden = false;
  const snapshot = () => {
    if (setting.kind === "rounds") {
      return {
        ...ADMIN_RUN_SETTINGS,
        continuation_rounds: { ...ADMIN_RUN_SETTINGS.continuation_rounds, rounds: Number(value) },
      };
    }
    if (setting.kind === "timeout") {
      return {
        budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
          budget.kind === "judge-run" ? { ...budget, seconds: Number(value) } : budget,
        ),
      };
    }
    return {
      agents: [
        {
          ...ADMIN_AGENTS.agents[0],
          daily_spend_cap_usd_micros: Number(value) * 1_000_000,
          daily_spend_cap_overridden: overridden,
          enabled,
        },
      ],
    };
  };
  const writes: Record<string, unknown>[] = [];
  await page.route(`**${setting.read}`, (route) =>
    route.request().isNavigationRequest() ? route.fallback() : route.fulfill({ json: snapshot() }),
  );
  await page.route(`**${setting.write}`, (route) => {
    const body = route.request().postDataJSON();
    writes.push(body);
    value = String(body.rounds ?? body.seconds ?? body.daily_spend_cap_usd_micros / 1_000_000);
    overridden = true;
    const reply = snapshot();
    return route.fulfill({
      json: reply.agents?.[0] ?? reply.budgets?.[0] ?? reply,
    });
  });
  await page.route("**/admin/agents/daily-report/enabled", (route) => {
    enabled = route.request().postDataJSON().enabled;
    return route.fulfill({
      json: {
        ...ADMIN_AGENTS.agents[0],
        enabled,
        daily_spend_cap_usd_micros: Number(value) * 1_000_000,
        daily_spend_cap_overridden: overridden,
      },
    });
  });
  return {
    writes,
    reread: async (next: string) => {
      value = next;
      const read = page.waitForResponse((reply) => reply.url().endsWith(setting.read));
      if (setting.kind === "cap") {
        await page.locator("#admin-agent-daily-report-note").fill("review Agent state");
        await page
          .getByRole("button", { name: `${enabled ? "停用" : "啟用"} daily-report`, exact: true })
          .click();
      } else {
        await page
          .getByRole("button", {
            name: setting.kind === "rounds" ? "重新整理設定" : "重新整理",
            exact: true,
          })
          .click();
      }
      await read;
    },
  };
}

for (const setting of SETTINGS) {
  for (const width of [375, 1280]) {
    test(`observed admin ${setting.kind} draft follows an observed round trip at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const { writes, reread } = await controlSetting(page, setting);
      await page.goto(setting.page);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const input = page.locator(setting.selector);
      const regionSelector =
        setting.kind === "rounds"
          ? `section:has(> h2#admin-settings-rounds-heading)`
          : `li.download-item:has(${setting.selector})`;
      const region = page.locator(regionSelector);
      await region.locator("summary").filter({ hasText: setting.summary }).click();
      await input.fill(setting.edited);
      await reread(setting.original);
      await expect(input).toHaveValue(setting.edited);
      await reread(setting.changed);
      await expect(input).toHaveValue(setting.changed);
      await reread(setting.original);
      await expect(region).toContainText(setting.fact);
      await expect(input).toHaveValue(setting.original);
      await input.fill("");
      await reread(setting.original);
      await expect(input).toHaveValue("");
      expect(writes).toEqual([]);
      await input.fill(setting.original);
      await page.locator(setting.note).fill("confirm current value");
      await region.getByRole("button", { name: setting.submit, exact: true }).click();
      await expect(page.locator(setting.receipt)).toBeFocused();
      await expect(page.locator(setting.receipt)).toBeInViewport({ ratio: 1 });
      expect(writes).toEqual([setting.body]);
      const report = await new AxeBuilder({ page }).include(regionSelector).analyze();
      await writeFile(info.outputPath("admin-draft-a11y.json"), JSON.stringify(report));
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await region.screenshot({ path: info.outputPath("admin-current-draft.png") });
    });
  }
}
