import { expect, test } from "vitest";
import { COMPARISON, OTHER_RUN, RUN } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

for (const intent of ["refresh", "retry", "candidate", "manual"] as const) {
  for (const moved of [false, true]) {
    test(`comparison ${intent} explains paused reads and ${moved ? "preserves moved focus" : "retains decision context"}`, async () => {
      const page = app();
      const reads: string[] = [];
      const writes = () => page.requests.filter((request) => request.method !== "GET");
      page.route(`**/runs/${RUN}/comparison?*`, ({ url }) => {
        reads.push(url.searchParams.get("against") ?? "");
        if (intent === "retry" && reads.length === 1)
          return { status: 503, json: { error: "service unavailable" } };
        return {
          json: {
            ...COMPARISON,
            runs: COMPARISON.runs.map((side) => ({
              ...side,
              final_output:
                intent === "refresh" && reads.length === 1
                  ? "更新前的比較輸出"
                  : "恢復連線後的比較輸出",
            })),
          },
        };
      });
      const picking = intent === "candidate" || intent === "manual";
      await page.goto(`/runs/${RUN}/compare${picking ? "" : `?against=${OTHER_RUN}`}`);
      const main = page.locator("main");
      const table = main.locator("table").filter({ hasText: "試跑紀錄任務判定與執行狀態對比" });
      const refresh = main.getByRole("button", { name: /^(更新比較結果|更新比較中…|等待連線…)$/ });
      const candidate = main.getByRole("button", { name: new RegExp(`試跑紀錄 ID ${OTHER_RUN}$`) });
      if (picking) await expect(candidate).toBeVisible();
      if (intent === "refresh")
        await expect(table.getByText("更新前的比較輸出", { exact: true })).toHaveCount(2);
      if (intent === "manual") {
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        await page.getByLabel("要比較的另一個試跑紀錄 ID").fill(OTHER_RUN);
      }
      const action =
        intent === "retry"
          ? main.getByRole("button", { name: "再試一次", exact: true })
          : intent === "candidate"
            ? candidate
            : intent === "manual"
              ? main.getByRole("button", { name: "比較", exact: true })
              : refresh;
      await expect(action).toBeVisible();
      try {
        await page.setOffline(true);
        await action.focus();
        await page.press("Enter");
        await expect(
          main.getByRole("status").filter({ hasText: "等待連線以讀取比較" }),
        ).toBeVisible();
        await expect(refresh).toHaveText("等待連線…");
        await expect(refresh).toHaveAttribute("aria-disabled", "true");
        await expect(table).toHaveCount(0);
        await expect(main.locator(".verdict")).toHaveCount(0);
        expect(reads).toEqual(picking ? [] : [OTHER_RUN]);
        if (picking || intent === "retry")
          await expect(page.locator("#run-comparison-heading")).toBeFocused();
        else await expect(refresh).toBeFocused();
        const destination = page
          .getByRole("navigation", { name: "主要導覽" })
          .getByRole("link", { name: "工作台", exact: true });
        if (moved) await destination.focus();
        else if (intent === "refresh") await page.press("Enter");
        expect(await page.axeViolations("body")).toEqual([]);
        await page.setOffline(false);
        await expect(table.getByText("恢復連線後的比較輸出", { exact: true })).toHaveCount(2);
        await expect(refresh).toHaveText("更新比較結果");
        await expect(refresh).toHaveAttribute("aria-disabled", "false");
        if (moved) await expect(destination).toBeFocused();
        else if (intent === "refresh") await expect(refresh).toBeFocused();
        else await expect(page.locator("#run-comparison-heading")).toBeFocused();
        expect(reads).toEqual(picking ? [OTHER_RUN] : [OTHER_RUN, OTHER_RUN]);
        expect(writes()).toEqual([]);
        expect(page.pathname()).toBe(`/runs/${RUN}/compare`);
        expect(page.search()).toBe(`?against=${OTHER_RUN}`);
      } finally {
        await page.setOffline(false);
      }
    });
  }
}
