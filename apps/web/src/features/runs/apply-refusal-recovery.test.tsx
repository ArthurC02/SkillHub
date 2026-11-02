import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RUN, SKILL } from "../../testing/fixtures/platform";
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

for (const [name, rejected] of [
  ["object list", { length: 1 }],
  ["null entry", [null]],
  ["unknown reason", [{ suggestion_id: "s1", blocked_reason: "future", message: "原因未知。" }]],
  ["empty reasons", []],
] as const) {
  test(`a refused apply with ${name} retains its confirmed refusal and recovers reasons without another write`, async () => {
    const page = app();
    let writes = 0;
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(`**/skills/${SKILL}/versions/from-suggestions`, () => {
      writes += 1;
      return {
        status: 422,
        json: {
          error: "not one of the suggestions could be applied, so no version was created",
          rejected_suggestions: rejected,
        },
      };
    });
    page.route("**/suggestions/s1/diff", async () => {
      reads += 1;
      if (reads === 1) return { status: 503, json: { error: "service unavailable" } };
      await gate;
      return {
        json: { target_path: "SKILL.md", applicable: false, blocked_reason: "target_changed" },
      };
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const region = page.getByRole("region", { name: "改善建議", exact: true });
      const apply = region.getByRole("button", {
        name: "以已接受的 1 項建議建立新版本",
        exact: true,
      });
      await expect(apply).toHaveCount(1);
      await apply.click();
      const failure = region.getByRole("alert", { name: "建立版本結果", exact: true });
      await expect(failure).toContainText("沒有一項建議可以套用，所以沒有建立新版本。");
      await expect(failure).toContainText("逐項拒絕原因未完整讀取");
      await expect(failure.getByRole("list")).toHaveCount(0);
      await expect(failure).not.toContainText("undefined");
      await region.getByRole("button", { name: "查看差異", exact: true }).first().click();
      const diff = region.getByRole("region", { name: "建議差異", exact: true });
      await expect(diff.getByRole("alert")).toContainText("暫時無法讀取差異");
      const retry = diff.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() => expect(reads).toBe(2));
      await expect(diff).toBeFocused();
      expect(writes).toBe(1);
      release();
      await expect(diff).toContainText("目前無法套用：目標檔案已經和建議產生當時不同");
      await expect(diff.getByRole("alert")).toHaveCount(0);
      await expect(diff.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
      await expect(diff).toBeFocused();
      await expect(failure).toContainText("逐項拒絕原因未完整讀取");
      expect(writes).toBe(1);
      expect(reads).toBe(2);
      expect(page.pathname()).toBe(`/runs/${RUN}`);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
    }
  });
}
