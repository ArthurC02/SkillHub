import { expect, test } from "vitest";
import { ADMIN_AGENTS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

const nonReads = (requests: { method: string }[]) =>
  requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));

for (const fault of ["missing list", "null brake"] as const) {
  test(`${fault} Agent read recovers without fabricated controls`, async () => {
    const page = app();
    let recover = false;
    page.route("**/admin/agents", () => ({
      json: recover
        ? ADMIN_AGENTS
        : fault === "missing list"
          ? {}
          : { ...ADMIN_AGENTS, brake: null },
    }));
    await page.goto("/admin/agents");
    await expect(page.getByRole("alert")).toContainText("暫時無法讀取 Agent 控制");
    await expect(page.getByRole("navigation", { name: "平台 Agent 工作區" })).toContainText(
      "煞車 無法取得",
    );
    await expect(page.getByRole("button", { name: "拉下 Agent 煞車", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "停用 daily-report", exact: true })).toHaveCount(
      0,
    );
    expect(await page.axeViolations("main")).toEqual([]);
    recover = true;
    await page.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.press("Enter");
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(page.locator("#admin-agent-brake-heading")).toBeFocused();
    await expect(
      page.getByRole("button", { name: "停用 daily-report", exact: true }),
    ).toBeVisible();
    expect(await page.axeViolations("main")).toEqual([]);
    expect(nonReads(page.requests)).toEqual([]);
  });
}

for (const receipt of ["valid", "null", "empty", "other"] as const) {
  for (const action of ["set", "restore"] as const) {
    test(`${receipt} Agent cap receipt verifies ${action} without replay`, async () => {
      const page = app();
      const errors: string[] = [];
      const onError = (event: ErrorEvent) => errors.push(event.message);
      window.addEventListener("error", onError);
      try {
        const writes: unknown[] = [];
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
        page.route("**/admin/agents", () => ({
          json: { agents: [sent ? (recover ? after : { ...after, enabled: "false" }) : before] },
        }));
        page.route("**/admin/agents/daily-report/spend-cap", ({ body }) => {
          writes.push(body);
          sent = true;
          return {
            json:
              receipt === "valid"
                ? after
                : receipt === "null"
                  ? null
                  : receipt === "empty"
                    ? {}
                    : { ...after, name: "another-agent", daily_spend_cap_usd_micros: 4_000_000 },
          };
        });
        await page.goto("/admin/agents");
        const adjust = page.getByText("調整每日花費上限", { exact: true });
        if (action === "set") {
          await expect(adjust).toHaveCount(1);
          await adjust.click();
          await page.locator("#admin-agent-daily-report-cap").fill("2.00");
        } else {
          const restore = page.getByText("恢復預設上限（$0.2000）", { exact: true });
          await expect(restore).toHaveCount(1);
          await restore.click();
        }
        const note = page.locator(
          `#admin-agent-daily-report-cap-${action === "set" ? "set" : "clear"}-note`,
        );
        await note.fill("verify the cap");
        const submitName =
          action === "set"
            ? "改 daily-report 的每日上限"
            : "把 daily-report 的上限改回預設 $0.2000";
        await page.getByRole("button", { name: submitName, exact: true }).focus();
        await page.press("Enter");
        const result = page.locator("#admin-agent-daily-report-cap-result");
        await expect(result).toHaveText(
          "daily-report 的每日花費上限的變更請求已完成；最新上限尚未確認。",
        );
        await expect(result).toBeFocused();
        expect(result.attribute("class") ?? "").not.toMatch(/notice-success/);
        await expect(page.getByRole("alert")).toContainText("暫時無法讀取 Agent 控制");
        await expect(page.locator("#admin-agent-daily-report-cap")).toHaveCount(0);
        expect(await page.axeViolations("main")).toEqual([]);
        recover = true;
        await page.getByRole("button", { name: "重新整理此 Agent 的上限", exact: true }).focus();
        await page.press("Enter");
        await expect(result).toContainText(
          action === "restore" ? "目前顯示預設 $0.2000" : "目前顯示營運者設定 $2.0000",
        );
        await expect(result).toBeFocused();
        await expect(page.getByRole("alert")).toHaveCount(0);
        await expect(result).not.toContainText("another-agent");
        await expect(result).not.toContainText("$4.0000");
        if (action === "restore") await adjust.click();
        const cap = page.locator("#admin-agent-daily-report-cap");
        await expect(cap).toHaveCount(1);
        await expect(cap).toHaveValue(action === "restore" ? "0.2" : "2");
        await expect(cap).toBeEnabled();
        expect(cap.attribute("readonly")).toBeNull();
        expect(await page.axeViolations("main")).toEqual([]);
        const setButton = page.getByRole("button", {
          name: "改 daily-report 的每日上限",
          exact: true,
        });
        if (action === "set") await expect(setButton).toBeDisabled();
        await cap.fill("3");
        await page.locator("#admin-agent-daily-report-cap-set-note").fill("review latest");
        await expect(result).toHaveCount(0);
        await expect(setButton).toBeEnabled();
        expect(writes).toEqual([
          {
            daily_spend_cap_usd_micros: action === "restore" ? null : 2_000_000,
            note: "verify the cap",
          },
        ]);
        expect(errors).toEqual([]);
      } finally {
        window.removeEventListener("error", onError);
      }
    });
  }
}
