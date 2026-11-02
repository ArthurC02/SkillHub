import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RUN, RUNS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

for (const ending of ["appended", "filtered", "more", "failure"] as const) {
  for (const moved of [false, true]) {
    test(`comparison candidates ${ending} page ${moved ? "preserves moved focus" : "continues keyboard navigation"}`, async () => {
      const page = app();
      const rows = Array.from({ length: 101 }, (_, index) => ({
        ...RUNS.runs[1],
        run_id: `44444444-4444-4444-8444-${String(index + 1).padStart(12, "0")}`,
      }));
      const offsets: string[] = [];
      const writes = () => page.requests.filter((request) => request.method !== "GET");
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route("**/runs?*", async ({ url }) => {
        const offset = url.searchParams.get("offset") ?? "";
        offsets.push(offset);
        expect(url.searchParams.get("limit")).toBe("51");
        expect(url.searchParams.get("test_case_id")).toBe(RUNS.runs[0].test_case_id);
        if (offset === "0") return { json: { runs: rows.slice(0, 51) } };
        await held;
        if (ending === "failure") return { status: 503, json: { error: "service unavailable" } };
        return {
          json: {
            runs:
              ending === "more"
                ? rows.slice(50)
                : ending === "filtered"
                  ? [RUNS.runs[0]]
                  : [rows[50]],
          },
        };
      });
      try {
        await page.goto(`/runs/${RUN}/compare`);
        const main = page.locator("main");
        const more = main.getByRole("button", { name: /^(載入更早的試跑紀錄|載入中…)$/ });
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        await expect(more).toHaveText("載入更早的試跑紀錄");
        await field.fill("尚未送出的比較 ID");
        await more.focus();
        await page.press("Enter");
        const destination = page
          .getByRole("navigation", { name: "主要導覽" })
          .getByRole("link", { name: "工作台", exact: true });
        await page.eventually(() => expect(offsets).toEqual(["0", "50"]));
        await expect(more).toBeDisabled();
        if (moved) await destination.focus();
        release();
        if (ending === "failure")
          await expect(main.getByRole("alert")).toContainText("暫時無法讀取可比較的試跑紀錄");
        else if (ending === "more") {
          await expect(more).toHaveText("載入更早的試跑紀錄");
          await expect(more).toBeEnabled();
          await expect(main.locator(".download-list > li")).toHaveCount(100);
        } else {
          await expect(more).toHaveCount(0);
          await expect(main.locator(".download-list > li")).toHaveCount(
            ending === "filtered" ? 50 : 51,
          );
        }
        if (moved) await expect(destination).toBeFocused();
        else if (ending === "failure")
          await expect(page.locator("#run-comparison-heading")).toBeFocused();
        else if (ending === "more") await expect(more).toBeFocused();
        else
          await expect(
            main.getByRole("button", {
              name: new RegExp(`試跑紀錄 ID ${rows[ending === "filtered" ? 49 : 50].run_id}$`),
            }),
          ).toBeFocused();
        await expect(field).toHaveValue("尚未送出的比較 ID");
        expect(offsets).toEqual(["0", "50"]);
        expect(writes()).toEqual([]);
        expect(await page.axeViolations("body")).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
