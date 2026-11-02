import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SKILL, SKILL_B, SKILL_C, skillDetail } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const IDS = [SKILL, SKILL_B, SKILL_C];
const headers = (names: string[]) => ["比較項目", ...names].join(" ");
const writesOf = (page: App) =>
  page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
const comparisonTable = (page: App) => page.getByRole("table", { name: /並排比較/ });
const refreshButton = (page: App) => page.getByRole("button", { name: /更新比較資料|等待連線/ });
const waitingStatus = (page: App) =>
  page.getByRole("status").filter({ hasText: "等待連線以讀取小工具比較資料" });

for (const cached of [false, true]) {
  test(`skill comparison ${cached ? "cached" : "initial"} offline read waits for current columns`, async () => {
    const page = app();
    const oldNames = ["摘要工具", "拆分工具", "清理工具"];
    const currentNames = ["更新摘要工具", "更新拆分工具", "更新清理工具"];
    const reads = [0, 0, 0];
    for (const [index, id] of IDS.entries()) {
      page.route(`**/api/skills/${id}?view=embedded`, () => {
        reads[index]++;
        return {
          json: skillDetail(
            id,
            cached && reads[index] === 1 ? oldNames[index] : currentNames[index],
          ),
        };
      });
    }
    const table = comparisonTable(page);
    const refresh = refreshButton(page);
    const waiting = waitingStatus(page);
    try {
      if (cached) {
        await page.goto(`/compare?ids=${IDS.join(",")}`);
        await expect(table.getByRole("columnheader")).toHaveText(headers(oldNames));
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        await page.setOffline(true);
        await page.goBack();
      } else {
        await page.goto("/compare?ids=");
        await expect(page.getByRole("status").filter({ hasText: "請從首頁" })).toBeVisible();
        await page.getByRole("link", { name: "回到首頁的目錄", exact: true }).click();
        const picks = page.locator(".search-results").getByRole("checkbox");
        await expect(picks).toHaveCount(3);
        for (const index of [0, 1, 2]) {
          await picks.nth(index).focus();
          await picks.nth(index).press(" ");
          await expect(picks.nth(index)).toBeChecked();
        }
        expect(reads).toEqual([0, 0, 0]);
        const comparison = page.getByRole("link", { name: "並排比較這 3 個小工具", exact: true });
        await expect(comparison).toBeVisible();
        await page.setOffline(true);
        await comparison.focus();
        await comparison.press("Enter");
      }
      await expect(page.locator("#skill-comparison-heading")).toBeVisible();
      await expect(waiting).toBeVisible();
      await expect(table).toHaveCount(0);
      await expect(refresh).toHaveAttribute("aria-disabled", "true");
      await refresh.focus();
      await refresh.press("Enter");
      expect(reads).toEqual(cached ? [1, 1, 1] : [0, 0, 0]);
      expect(await page.axeViolations("main")).toEqual([]);
      await page.setOffline(false);
      await expect(table.getByRole("columnheader")).toHaveText(headers(currentNames));
      await expect(waiting).toHaveCount(0);
      await expect(refresh).toHaveAttribute("aria-disabled", "false");
      await expect(refresh).toBeFocused();
      expect(reads).toEqual(cached ? [2, 2, 2] : [1, 1, 1]);
      expect(new URLSearchParams(page.search()).get("ids")).toBe(IDS.join(","));
      expect(writesOf(page)).toEqual([]);
    } finally {
      await page.setOffline(false);
    }
  });
}

test("skill comparison cached paused retry with two readable columns waits for the third", async () => {
  const page = app();
  const names = ["摘要工具", "拆分工具", "清理工具"];
  const reads = [0, 0, 0];
  for (const [index, id] of IDS.entries()) {
    page.route(`**/api/skills/${id}?view=embedded`, () => {
      reads[index]++;
      if (index === 1 && reads[index] === 2)
        return { status: 503, json: { error: "service unavailable" } };
      return { json: skillDetail(id, names[index]) };
    });
  }
  const table = comparisonTable(page);
  const refresh = refreshButton(page);
  try {
    await page.goto(`/compare?ids=${IDS.join(",")}`);
    await expect(table.getByRole("columnheader")).toHaveText(headers(names));
    await page.getByRole("button", { name: "更新比較資料", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("有 1 個小工具讀取失敗");
    await expect(table.getByRole("columnheader")).toHaveText(headers([names[0], names[2]]));
    await page.setOffline(true);
    const retry = page.getByRole("button", { name: "再試一次", exact: true });
    await retry.focus();
    await retry.press("Enter");
    const waiting = waitingStatus(page);
    await expect(waiting).toBeVisible();
    await expect(table).toHaveCount(0);
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(refresh).toHaveAttribute("aria-disabled", "true");
    await refresh.focus();
    await refresh.press("Enter");
    expect(reads).toEqual([2, 2, 2]);
    expect(await page.axeViolations("main")).toEqual([]);
    await page.setOffline(false);
    await expect(table.getByRole("columnheader")).toHaveText(headers(names));
    await expect(refresh).toBeFocused();
    expect(reads).toEqual([2, 3, 2]);
    expect(new URLSearchParams(page.search()).get("ids")).toBe(IDS.join(","));
    expect(writesOf(page)).toEqual([]);
  } finally {
    await page.setOffline(false);
  }
});
