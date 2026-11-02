import { expect, test } from "vitest";
import {
  ADMIN_AGENTS,
  ADMIN_MODEL_BUDGETS,
  ADMIN_RUN_SETTINGS,
} from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

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

function controlSetting(page: App, setting: (typeof SETTINGS)[number]) {
  let value = setting.original as string;
  let enabled = true;
  let overridden = false;
  let reads = 0;
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
  page.route(`**${setting.read}`, () => {
    reads += 1;
    return { json: snapshot() };
  });
  page.route(`**${setting.write}`, ({ body }) => {
    const sent = body as Record<string, number>;
    writes.push(sent);
    value = String(sent.rounds ?? sent.seconds ?? sent.daily_spend_cap_usd_micros / 1_000_000);
    overridden = true;
    const reply = snapshot() as { agents?: unknown[]; budgets?: unknown[] };
    return { json: reply.agents?.[0] ?? reply.budgets?.[0] ?? reply };
  });
  page.route("**/admin/agents/daily-report/enabled", ({ body }) => {
    enabled = (body as { enabled: boolean }).enabled;
    return {
      json: {
        ...ADMIN_AGENTS.agents[0],
        enabled,
        daily_spend_cap_usd_micros: Number(value) * 1_000_000,
        daily_spend_cap_overridden: overridden,
      },
    };
  });
  return {
    writes,
    reread: async (next: string) => {
      value = next;
      const before = reads;
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
      await page.eventually(() => expect(reads).toBeGreaterThan(before));
    },
  };
}

for (const setting of SETTINGS) {
  test(`observed admin ${setting.kind} draft follows an observed round trip`, async () => {
    const page = app();
    const { writes, reread } = controlSetting(page, setting);
    await page.goto(setting.page);
    await page.eventually(() => expect(page.locator(".auth-name").text()).toBe("tester"));
    const input = page.locator(setting.selector);
    const regionSelector =
      setting.kind === "rounds"
        ? `section:has(> h2#admin-settings-rounds-heading)`
        : `li.download-item:has(${setting.selector})`;
    const region = page.locator(regionSelector);
    await page.eventually(() =>
      expect(region.locator("summary").filter({ hasText: setting.summary }).count()).toBe(1),
    );
    await region.locator("summary").filter({ hasText: setting.summary }).click();
    await input.fill(setting.edited);
    await reread(setting.original);
    expect(input.value()).toBe(setting.edited);
    await reread(setting.changed);
    await page.eventually(() => expect(input.value()).toBe(setting.changed));
    await reread(setting.original);
    await page.eventually(() => expect(region.text()).toContain(setting.fact));
    await page.eventually(() => expect(input.value()).toBe(setting.original));
    await input.fill("");
    await reread(setting.original);
    expect(input.value()).toBe("");
    expect(writes).toEqual([]);
    await input.fill(setting.original);
    await page.locator(setting.note).fill("confirm current value");
    await region.locator("button").filter({ hasText: setting.submit }).first().click();
    await page.eventually(() => expect(page.locator(setting.receipt).isFocused()).toBe(true));
    expect(writes).toEqual([setting.body]);
    expect(await page.axeViolations(regionSelector)).toEqual([]);
  });
}
