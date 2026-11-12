import { expect, test } from "vitest";
import { RUN, RUNS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

for (const retrying of [false, true]) {
  for (const moved of [false, true]) {
    test(`comparison candidate ${retrying ? "retry" : "next page"} explains paused reads and ${moved ? "preserves moved focus" : "continues keyboard navigation"}`, async () => {
      const page = app();
      const rows = Array.from({ length: 51 }, (_, index) => ({
        ...RUNS.runs[1],
        run_id: `44444444-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
      }));
      const offsets: string[] = [];
      const writes = () => page.requests.filter((request) => request.method !== "GET");
      let release!: () => void;
      const resumedRead = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route("**/runs?*", async ({ url }) => {
        const offset = url.searchParams.get("offset") ?? "";
        offsets.push(offset);
        if (retrying && offset === "50")
          return { status: 503, json: { error: "service unavailable" } };
        if (retrying ? offsets.length === 3 : offset === "50") await resumedRead;
        return { json: { runs: offset === "0" ? rows : [rows[50]] } };
      });
      try {
        await page.goto(`/runs/${RUN}/compare`);
        const main = page.locator("main");
        const more = main.getByRole("button", { name: /^(載入更早的試跑紀錄|載入中…|等待連線…)$/ });
        await expect(more).toHaveText("載入更早的試跑紀錄");
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        await field.fill("尚未送出的比較 ID");
        if (retrying) {
          await more.focus();
          await page.press("Enter");
          await expect(main.getByRole("alert")).toContainText("暫時無法讀取可比較的試跑紀錄");
        }
        const retry = main.getByRole("button", { name: "再試一次", exact: true });
        const destination = page
          .getByRole("navigation", { name: "主要導覽" })
          .getByRole("link", { name: "工作台", exact: true });
        await page.setOffline(true);
        await (retrying ? retry : more).focus();
        await page.press("Enter");
        await expect(
          main
            .getByRole("status")
            .filter({ hasText: "等待連線以讀取可比較的試跑紀錄；恢復連線後會繼續讀取。" }),
        ).toBeVisible();
        await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(retry).toHaveCount(0);
        expect(offsets).toEqual(retrying ? ["0", "50"] : ["0"]);
        if (retrying) await expect(page.locator("#run-comparison-heading")).toBeFocused();
        else {
          await expect(more).toHaveText("等待連線…");
          await expect(more).toBeDisabled();
          await expect(main.locator(".download-list > li")).toHaveCount(50);
        }
        if (moved) await destination.focus();
        expect(await page.axeViolations("body")).toEqual([]);
        await page.setOffline(false);
        await page.eventually(() =>
          expect(offsets).toEqual(retrying ? ["0", "50", "0"] : ["0", "50"]),
        );
        await expect(main.getByRole("status").filter({ hasText: "等待連線以讀取" })).toHaveCount(0);
        await expect(main.getByRole("alert")).toHaveCount(0);
        if (retrying)
          await expect(
            main.getByRole("status").filter({ hasText: "載入可比較的試跑紀錄中…" }),
          ).toBeVisible();
        else {
          await expect(more).toHaveText("載入中…");
          await expect(more).toBeDisabled();
        }
        release();
        if (retrying) {
          await expect(more).toHaveText("載入更早的試跑紀錄");
          await expect(more).toBeEnabled();
          await expect(main.locator(".download-list > li")).toHaveCount(50);
        } else {
          await expect(more).toHaveCount(0);
          await expect(main.locator(".download-list > li")).toHaveCount(51);
        }
        if (moved) await expect(destination).toBeFocused();
        else if (retrying) await expect(page.locator("#run-comparison-heading")).toBeFocused();
        else
          await expect(
            main.getByRole("button", { name: new RegExp(`試跑紀錄 ID ${rows[50].run_id}$`) }),
          ).toBeFocused();
        await expect(field).toHaveValue("尚未送出的比較 ID");
        expect(offsets).toEqual(retrying ? ["0", "50", "0"] : ["0", "50"]);
        expect(writes()).toEqual([]);
        expect(await page.axeViolations("body")).toEqual([]);
      } finally {
        release();
        await page.setOffline(false);
      }
    });
  }
}
