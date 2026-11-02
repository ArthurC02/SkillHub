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
    await page.eventually(() =>
      expect(page.getByRole("alert").text()).toContain("暫時無法讀取 Agent 控制"),
    );
    expect(page.getByRole("navigation", { name: "平台 Agent 工作區" }).text()).toContain(
      "煞車 無法取得",
    );
    expect(page.getByRole("button", { name: "拉下 Agent 煞車", exact: true }).count()).toBe(0);
    expect(page.getByRole("button", { name: "停用 daily-report", exact: true }).count()).toBe(0);
    expect(await page.axeViolations("main")).toEqual([]);
    recover = true;
    await page.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.press("Enter");
    await page.eventually(() => expect(page.getByRole("alert").count()).toBe(0));
    await page.eventually(() =>
      expect(page.locator("#admin-agent-brake-heading").isFocused()).toBe(true),
    );
    await page.eventually(() =>
      expect(
        page.getByRole("button", { name: "停用 daily-report", exact: true }).count(),
      ).toBeGreaterThan(0),
    );
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
          await page.eventually(() => expect(adjust.count()).toBe(1));
          await adjust.click();
          await page.locator("#admin-agent-daily-report-cap").fill("2.00");
        } else {
          const restore = page.getByText("恢復預設上限（$0.2000）", { exact: true });
          await page.eventually(() => expect(restore.count()).toBe(1));
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
        await page.eventually(() =>
          expect(result.text()).toBe(
            "daily-report 的每日花費上限的變更請求已完成；最新上限尚未確認。",
          ),
        );
        expect(result.isFocused()).toBe(true);
        expect(result.attribute("class") ?? "").not.toMatch(/notice-success/);
        await page.eventually(() =>
          expect(page.getByRole("alert").text()).toContain("暫時無法讀取 Agent 控制"),
        );
        expect(page.locator("#admin-agent-daily-report-cap").count()).toBe(0);
        expect(await page.axeViolations("main")).toEqual([]);
        recover = true;
        await page.getByRole("button", { name: "重新整理此 Agent 的上限", exact: true }).focus();
        await page.press("Enter");
        await page.eventually(() =>
          expect(result.text()).toContain(
            action === "restore" ? "目前顯示預設 $0.2000" : "目前顯示營運者設定 $2.0000",
          ),
        );
        expect(result.isFocused()).toBe(true);
        await page.eventually(() => expect(page.getByRole("alert").count()).toBe(0));
        expect(result.text()).not.toContain("another-agent");
        expect(result.text()).not.toContain("$4.0000");
        if (action === "restore") await adjust.click();
        const cap = page.locator("#admin-agent-daily-report-cap");
        await page.eventually(() => expect(cap.count()).toBe(1));
        expect(cap.value()).toBe(action === "restore" ? "0.2" : "2");
        expect(cap.isDisabled()).toBe(false);
        expect(cap.attribute("readonly")).toBeNull();
        expect(await page.axeViolations("main")).toEqual([]);
        const setButton = page.getByRole("button", {
          name: "改 daily-report 的每日上限",
          exact: true,
        });
        if (action === "set") expect(setButton.isDisabled()).toBe(true);
        await cap.fill("3");
        await page.locator("#admin-agent-daily-report-cap-set-note").fill("review latest");
        expect(result.count()).toBe(0);
        expect(setButton.isDisabled()).toBe(false);
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
