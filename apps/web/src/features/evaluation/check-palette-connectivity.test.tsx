import { expect, test, vi } from "vitest";
import {
  CHECK_KINDS,
  PLATFORM_GRADER,
  TEST_CASE,
  VERSION,
  WORKSPACE_GRADER,
} from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

vi.mock("./components/CheckFlow", () => ({ CheckFlow: () => null }));

const app = renderedApp();

for (const resource of ["kinds", "graders"] as const) {
  test(`an offline ${resource} retry explains the wait and preserves the criterion`, async () => {
    const page = app();
    const path = resource === "kinds" ? "/evaluation/check-kinds" : "/evaluation/graders";
    const what = resource === "kinds" ? "檢查方式" : "評分器版本清單";
    let reads = 0;
    const writes = () =>
      page.requests.filter(
        (request) => request.method !== "GET" && request.url.pathname.startsWith("/test-cases/"),
      );
    page.route(`**${path}`, () => {
      reads++;
      return reads === 1
        ? { status: 503, json: { error: "unavailable" } }
        : {
            json:
              resource === "kinds" ? CHECK_KINDS : { graders: [PLATFORM_GRADER, WORKSPACE_GRADER] },
          };
    });
    try {
      await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
      const draft = page.getByLabel("新增驗收條件", { exact: true });
      await draft.fill("保留這份離線前的驗收條件");
      if (resource === "graders") {
        await page.getByLabel("從哪一種節點開始", { exact: true }).select("prompt_grader");
        await page.getByRole("button", { name: "開始設計", exact: true }).click();
      }
      const palette = page.getByRole("group", { name: "檢查方式與設計", exact: true });
      await expect(palette.getByRole("alert")).toBeVisible();
      expect(reads).toBe(1);
      await page.setOffline(true);
      await palette.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(palette.getByRole("alert")).toHaveCount(0);
      expect(navigator.onLine).toBe(false);
      const waiting = palette.getByRole("status").filter({ hasText: `等待連線以讀取${what}` });
      await expect(waiting).toContainText("恢復連線後會繼續讀取。");
      await expect(palette.locator(".loading-skeleton")).toHaveCount(0);
      await expect(palette.getByRole("alert")).toHaveCount(0);
      await expect(draft).toHaveValue("保留這份離線前的驗收條件");
      if (resource === "graders") {
        await expect(page.getByLabel("評分器版本", { exact: true })).toBeDisabled();
        await expect(page.getByRole("button", { name: "新增", exact: true })).toBeDisabled();
      }
      expect(reads).toBe(1);
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
      await page.setOffline(false);
      if (resource === "kinds") {
        await expect(page.getByLabel("從哪一種節點開始", { exact: true })).toBeVisible();
      } else {
        const picker = page.getByLabel("評分器版本", { exact: true });
        await expect(picker).toBeEnabled();
        await picker.select(WORKSPACE_GRADER.version_id);
        await expect(picker).toHaveValue(WORKSPACE_GRADER.version_id);
        await expect(page.getByRole("button", { name: "新增", exact: true })).toBeEnabled();
      }
      await expect(waiting).toHaveCount(0);
      await expect(draft).toHaveValue("保留這份離線前的驗收條件");
      expect(reads).toBe(2);
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      await page.setOffline(false);
    }
  });
}
