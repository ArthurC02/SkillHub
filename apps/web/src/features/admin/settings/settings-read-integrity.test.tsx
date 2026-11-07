import { expect, test } from "vitest";
import { ADMIN_RUN_SETTINGS } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

function present(selector: string): number {
  return document.querySelectorAll(selector).length;
}

const WRITE_METHODS = new Set(["POST", "PUT", "PATCH", "DELETE"]);

for (const kind of ["judge", "runs"] as const) {
  for (const initial of [true, false]) {
    test(`${kind} settings reject an incomplete ${initial ? "initial" : "refreshed"} read and recover`, async () => {
      const page = app();
      let unavailable = initial;
      const path = kind === "judge" ? "/admin/settings" : "/admin/settings/runs";
      page.route(`**${path}`, () => ({
        json:
          kind === "judge"
            ? {
                judge_panel: { enabled: unavailable ? "false" : false },
                injection_check: { enabled: false },
                reflection_candidates: { count: 1 },
              }
            : {
                ...ADMIN_RUN_SETTINGS,
                tokens_per_minute: {
                  ...ADMIN_RUN_SETTINGS.tokens_per_minute,
                  limit: unavailable ? null : 200000,
                },
              },
      }));
      await page.goto("/admin/settings");
      const current = kind === "judge" ? "關閉：一個判定（預設）" : "目前：試跑的模型金鑰最多";
      const note =
        kind === "judge" ? "#admin-settings-judge-panel-note" : "#admin-settings-run-tpm-note";
      if (!initial) {
        await expect(page.locator("main")).toContainText(current);
        unavailable = true;
        await page.getByRole("button", { name: "重新整理設定", exact: true }).click();
      }
      const what = kind === "judge" ? "平台設定" : "試跑設定";
      await page.eventually(() => {
        expect(page.getByRole("alert").text()).toContain(`暫時無法讀取${what}`);
        expect(page.getByRole("heading", { name: "平台設定", exact: true }).count()).toBe(1);
        expect(present(note)).toBe(0);
        expect(page.locator("main").text()).not.toContain(current);
        expect(page.locator("main").text()).toContain(
          kind === "judge" ? "目前：3 輪" : "關閉：一個判定（預設）",
        );
      });
      expect(await page.axeViolations("main")).toEqual([]);
      unavailable = false;
      await page.getByRole("button", { name: `再讀取${what}`, exact: true }).click();
      await page.eventually(() => {
        expect(page.getByRole("alert").count()).toBe(0);
        expect(page.locator("main").text()).toContain(current);
        expect(present(note)).toBe(1);
      });
      expect(page.requests.filter((r) => WRITE_METHODS.has(r.method))).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}

for (const kind of ["judge", "tokens"] as const) {
  test(`an incomplete successful ${kind} setting receipt waits for verification`, async () => {
    const page = app();
    const judge = kind === "judge";
    const read = judge ? "/admin/settings" : "/admin/settings/runs";
    const write = judge ? "/admin/settings/judge-panel" : "/admin/settings/runs/tokens-per-minute";
    const id = judge ? "admin-settings-judge-panel" : "admin-settings-run-tpm";
    let written = false;
    let recovered = false;
    let reads = 0;
    page.route(`**${read}`, () => {
      reads++;
      if (written && !recovered) return { status: 503, json: { error: "settings unavailable" } };
      return {
        json: judge
          ? {
              judge_panel: { enabled: written },
              injection_check: { enabled: false },
              reflection_candidates: { count: 1 },
            }
          : {
              ...ADMIN_RUN_SETTINGS,
              tokens_per_minute: {
                ...ADMIN_RUN_SETTINGS.tokens_per_minute,
                limit: written ? 300000 : 200000,
              },
            },
      };
    });
    page.route(`**${write}`, () => {
      written = true;
      return { json: {} };
    });
    const writes = () =>
      page.requests.filter((r) => WRITE_METHODS.has(r.method)).map((r) => r.url.pathname);
    await page.goto("/admin/settings");
    await expect(
      page.getByText(judge ? "調整評審團" : "調整每分鐘 tokens 上限", { exact: true }),
    ).toHaveCount(1);
    await page.getByText(judge ? "調整評審團" : "調整每分鐘 tokens 上限", { exact: true }).click();
    if (!judge) await page.locator(`#${id}-value`).fill("300000");
    await page.locator(`#${id}-note`).fill("verify the setting");
    await page
      .getByRole("button", { name: judge ? "開啟評審團" : "改每分鐘上限", exact: true })
      .click();
    const result = page.locator(`#${id}-result`);
    await page.eventually(() => {
      expect(result.text()).toContain("目前設定尚未確認");
      expect(result.attribute("class")).not.toContain("notice-success");
      expect(present(`#${id}-note`)).toBe(0);
      expect(page.getByRole("alert").text()).toContain(
        judge ? "暫時無法讀取平台設定" : "暫時無法讀取試跑設定",
      );
    });
    expect(reads).toBe(2);
    expect(writes()).toEqual([write]);
    expect(await page.axeViolations("main")).toEqual([]);
    recovered = true;
    await page
      .getByRole("button", { name: judge ? "再讀取平台設定" : "再讀取試跑設定", exact: true })
      .click();
    await page.eventually(() => {
      expect(result.attribute("class")).toContain("notice-success");
      expect(result.text()).toContain(
        judge ? "已開啟，需要模型判定的條件" : "已改為每分鐘 300,000 tokens",
      );
      expect(page.getByRole("alert").count()).toBe(0);
    });
    expect(reads).toBe(3);
    expect(writes()).toEqual([write]);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
