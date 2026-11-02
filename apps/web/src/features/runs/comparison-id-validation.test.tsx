import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { OTHER_RUN, RUN } from "../../testing/fixtures/platform";
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

for (const existing of [false, true]) {
  for (const method of ["keyboard", "pointer"] as const) {
    for (const [blankName, blank] of [
      ["empty", ""],
      ["spaces", "   "],
      ["nonbreaking space", " "],
    ] as const) {
      test(`comparison ID validation with ${blankName} and ${method} ${existing ? "preserves existing results" : "explains an empty submission"}`, async () => {
        const page = app();
        const reads = () =>
          page.requests.filter((request) => request.url.pathname.endsWith("/comparison"));
        const writes = () => page.requests.filter((request) => request.method !== "GET");
        await page.goto(`/runs/${RUN}/compare${existing ? `?against=${OTHER_RUN}` : ""}`);
        const main = page.locator("main");
        const table = main.locator("table").filter({ hasText: "試跑紀錄任務判定與執行狀態對比" });
        if (existing) {
          await expect(table).toBeVisible();
          await main.getByText("換一個要比較的試跑紀錄", { exact: true }).click();
        }
        const originalLocation = page.pathname() + page.search();
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        const submit = async () => {
          if (method === "pointer")
            await main.getByRole("button", { name: "比較", exact: true }).click();
          else {
            await field.focus();
            await page.press("Enter");
          }
        };
        await field.fill(blank);
        await submit();
        await expect(field).toHaveAccessibleDescription(/ID 不能只有空白/);
        const input = field.element() as HTMLInputElement;
        expect(input.validity.valueMissing).toBe(blank === "");
        expect(input.validity.patternMismatch).toBe(blank !== "");
        expect(input.validationMessage).not.toBe("");
        expect(page.pathname() + page.search()).toBe(originalLocation);
        await expect(table).toHaveCount(existing ? 1 : 0);
        expect(reads()).toHaveLength(existing ? 1 : 0);
        expect(await page.axeViolations("main form")).toEqual([]);
        await field.fill(`${blank}${OTHER_RUN}${blank}`);
        await submit();
        await page.eventually(() => {
          expect(page.pathname()).toBe(`/runs/${RUN}/compare`);
          expect(page.search()).toBe(`?against=${OTHER_RUN}`);
        });
        await expect(table).toBeVisible();
        await expect(page.locator("#run-comparison-heading")).toBeFocused();
        expect(reads()).toHaveLength(1);
        expect(reads()[0].url.searchParams.get("against")).toBe(OTHER_RUN);
        expect(writes()).toEqual([]);
      });
    }
  }
}
