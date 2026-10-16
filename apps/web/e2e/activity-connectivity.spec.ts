import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ACTIVITY, RUN } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

function recordWrites(page: Page) {
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  return writes;
}

async function stubActivityReads(
  page: Page,
  mode: "retry" | "populated" | "empty" | "next-page" | "next-page-retry",
) {
  await stubPlatform(page);
  const pagination = mode.startsWith("next-page");
  const cursors: Array<string | null> = [];
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/me/activity?*", async (route) => {
    cursors.push(new URL(route.request().url()).searchParams.get("cursor"));
    if (cursors.length === 1) {
      if (mode === "retry") {
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      }
      return route.fulfill({
        json: {
          ...ACTIVITY,
          items: mode === "empty" ? [] : ACTIVITY.items,
          ...(pagination ? { next_cursor: "older-page" } : {}),
        },
      });
    }
    if (mode === "next-page-retry" && cursors.length === 2) {
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    }
    await gate;
    return route.fulfill({
      json: pagination
        ? {
            ...ACTIVITY,
            items: [
              {
                ...ACTIVITY.items[0],
                source_id: RUN,
                summary: "較早的試跑",
                continuation: { kind: "run", run_id: RUN },
              },
            ],
          }
        : ACTIVITY,
    });
  });
  return { cursors, release };
}

async function inspect(page: Page, state: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(`${state}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
  await page.screenshot({ path: test.info().outputPath(`${state}-phone.png`), fullPage: true });
}

for (const mode of ["retry", "populated", "empty", "next-page", "next-page-retry"] as const) {
  for (const colorScheme of ["light", "dark"] as const) {
    test(`activity ${mode} explains an offline wait and resumes once in ${colorScheme} mode`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 375, height: 900 });
      await page.emulateMedia({ colorScheme });
      const pagination = mode.startsWith("next-page");
      const retry = mode.endsWith("retry");
      const writes = recordWrites(page);
      const { cursors, release } = await stubActivityReads(page, mode);
      const main = page.getByRole("main");
      const rows = main.locator(".activity-list > li");
      const empty = main.getByRole("heading", { name: "目前沒有活動", exact: true });
      const waiting = main.getByRole("status").filter({ hasText: "等待連線以讀取Workspace 活動" });
      try {
        await page.goto("/activity");
        if (mode === "retry") await expect(main.getByRole("alert")).toBeVisible();
        else if (mode === "empty") await expect(empty).toBeVisible();
        else await expect(rows).toHaveCount(3);
        if (mode === "next-page-retry") {
          await main.getByRole("button", { name: "載入更多活動", exact: true }).click();
          await expect(main.getByRole("alert")).toContainText("更早的活動讀取失敗");
        }
        await setOffline(page, true);
        const action = main.getByRole("button", {
          name: retry ? "再試一次" : pagination ? "載入更多活動" : "重新整理",
          exact: true,
        });
        await action.focus();
        await page.keyboard.press("Enter");
        await expect(waiting).toContainText("恢復連線後會繼續讀取。");
        expect(await page.evaluate(() => navigator.onLine)).toBe(false);
        await expect(main.locator(".loading-skeleton")).toHaveCount(0);
        if (mode === "next-page-retry") {
          await expect(main.getByRole("alert")).toContainText("僅保留已載入的紀錄，尚未讀完");
          await expect(
            main.getByRole("button", { name: "重新讀取中…", exact: true }),
          ).toBeDisabled();
        } else await expect(main.getByRole("alert")).toHaveCount(0);
        await expect(empty).toHaveCount(0);
        await expect(rows).toHaveCount(mode === "retry" || mode === "empty" ? 0 : 3);
        const buttons = main.getByRole("button", { name: "等待連線…", exact: true });
        await expect(buttons).toHaveCount(retry ? 0 : pagination ? 2 : 1);
        for (const button of await buttons.all()) await expect(button).toBeDisabled();
        if (mode === "retry" || mode === "empty") {
          await expect(main.locator(".activity-summary")).toHaveCount(0);
        }
        const before = mode === "next-page-retry" ? [null, "older-page"] : [null];
        expect(cursors).toEqual(before);
        expect(writes).toEqual([]);
        await inspect(page, "activity-paused");
        await setOffline(page, false);
        await expect.poll(() => cursors.length).toBe(before.length + 1);
        await expect(empty).toHaveCount(0);
        if (mode === "empty") {
          await expect(main.locator("[data-loading]")).toContainText("載入Workspace 活動中…");
        }
        if (pagination) await expect(rows).toHaveCount(3);
        release();
        await expect(rows).toHaveCount(pagination ? 4 : 3);
        await expect(waiting).toHaveCount(0);
        await expect(main.getByRole("button", { name: "重新整理", exact: true })).toBeEnabled();
        await expect(main.getByRole("button", { name: "載入更多活動", exact: true })).toHaveCount(
          0,
        );
        await expect(main.getByText("已載入全部活動", { exact: true })).toBeVisible();
        expect(cursors).toEqual([...before, pagination ? "older-page" : null]);
        expect(writes).toEqual([]);
        await inspect(page, "activity-recovered");
      } finally {
        release();
        await setOffline(page, false);
        await writeFile(
          test.info().outputPath("activity-connectivity-state.json"),
          JSON.stringify({ mode, colorScheme, cursors, writes, aria: await main.ariaSnapshot() }),
        );
      }
    });
  }
}
