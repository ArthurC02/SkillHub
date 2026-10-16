import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_AUDIT_LOG } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const firstId = "dddddddd-dddd-4ddd-8ddd-000000000001";
const nextId = "dddddddd-dddd-4ddd-8ddd-000000000002";
const firstCursor = "2026-09-10T08:00:00Z_50";
const nextCursor = "2026-09-09T08:00:00Z_100";

async function inspect(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`${label}.png`) });
}

type Arrival = "populated" | "empty" | "more" | 503 | 401 | 403 | 404;

async function auditPages(page: Page, arrival: Arrival) {
  await stubPlatform(page);
  const cursors: Array<string | null> = [];
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  let release!: () => void;
  let releaseRecovery!: () => void;
  let recovered = false;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const recovery = new Promise<void>((resolve) => {
    releaseRecovery = resolve;
  });
  await page.route(/\/admin\/audit-log\?/, async (route) => {
    const url = new URL(route.request().url());
    expect(url.searchParams.get("limit")).toBe("50");
    const cursor = url.searchParams.get("cursor");
    cursors.push(cursor);
    if (!cursor)
      return route.fulfill({
        json: {
          events: [{ ...ADMIN_AUDIT_LOG.events[0], resource_id: firstId }],
          next_cursor: firstCursor,
        },
      });
    if (cursor === nextCursor) return route.fulfill({ json: { events: [] } });
    await gate;
    if (typeof arrival === "number" && !recovered)
      return route.fulfill({ status: arrival, json: { error: "controlled audit read" } });
    if (recovered) await recovery;
    return route.fulfill({
      json: {
        events: arrival === "empty" ? [] : [{ ...ADMIN_AUDIT_LOG.events[1], resource_id: nextId }],
        next_cursor: arrival === "more" ? nextCursor : undefined,
      },
    });
  });
  return {
    cursors,
    writes,
    release,
    releaseRecovery,
    recover: () => {
      recovered = true;
    },
  };
}

for (const arrival of ["populated", "empty", "more", 503, 401, 403, 404] as const) {
  for (const moved of [false, true]) {
    test(`audit ${arrival} pagination ${moved ? "preserves moved focus" : "continues keyboard focus"}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const { cursors, writes, release, releaseRecovery, recover } = await auditPages(
        page,
        arrival,
      );
      try {
        await page.goto("/admin/audit-log");
        const rows = page.locator("tbody tr");
        const more = page
          .getByRole("button")
          .filter({ hasText: /^(載入更多|載入中…|重試載入更多)$/ });
        await expect(rows).toHaveCount(1);
        await more.focus();
        await page.keyboard.press("Enter");
        await expect(more).toHaveText("載入中…");
        await expect(more).toBeDisabled();
        const navigation = page
          .getByRole("navigation", { name: "主要導覽", exact: true })
          .getByRole("link", { name: "工作台", exact: true });
        if (moved) await navigation.focus();
        release();
        if (arrival === 503) {
          await expect(page.getByRole("alert")).toContainText("已載入的紀錄仍可查看");
          await expect(rows).toHaveCount(1);
          await expect(more).toHaveText("重試載入更多");
          if (!moved) {
            await expect(more).toBeFocused();
            await page.screenshot({ path: test.info().outputPath("audit-pagination-failed.png") });
            recover();
            await page.keyboard.press("Enter");
            await expect(more).toBeDisabled();
            releaseRecovery();
            await expect(rows).toHaveCount(2);
            await expect(more).toHaveCount(0);
            await expect(rows.filter({ hasText: nextId })).toBeFocused();
            await expect(page.getByRole("alert")).toHaveCount(0);
          }
        } else if (typeof arrival === "number") {
          if (arrival === 401) {
            await expect(
              page.getByRole("status").filter({ hasText: "動作紀錄需要登入。" }),
            ).toBeVisible();
            await expect(page.getByRole("link", { name: "使用 GitHub 登入" })).toBeVisible();
          } else await expect(page.getByRole("alert").first()).toBeVisible();
          await expect(rows).toHaveCount(0);
          await expect(page.getByText(firstId, { exact: true })).toHaveCount(0);
          await expect(more).toHaveCount(0);
          if (!moved)
            await expect(page.getByRole("heading", { level: 1, name: "動作紀錄" })).toBeFocused();
        } else {
          await expect(rows).toHaveCount(arrival === "empty" ? 1 : 2);
          if (arrival !== "more") await expect(more).toHaveCount(0);
          if (!moved && arrival === "more") {
            await expect(more).toBeFocused();
            await page.keyboard.press("Enter");
            await expect(more).toHaveCount(0);
            await expect(rows.filter({ hasText: nextId })).toBeFocused();
          } else if (!moved)
            await expect(
              rows.filter({ hasText: arrival === "empty" ? firstId : nextId }),
            ).toBeFocused();
        }
        if (moved) await expect(navigation).toBeFocused();
        expect(cursors).toEqual(
          arrival === 503 && !moved
            ? [null, firstCursor, firstCursor]
            : arrival === "more" && !moved
              ? [null, firstCursor, nextCursor]
              : [null, firstCursor],
        );
        expect(writes).toEqual([]);
        await inspect(page, "audit-pagination");
      } finally {
        release();
        releaseRecovery();
        await writeFile(
          test.info().outputPath("audit-pagination-state.json"),
          JSON.stringify({
            arrival,
            moved,
            cursors,
            writes,
            focus: await page.evaluate(() => ({
              tag: document.activeElement?.tagName,
              text: document.activeElement?.textContent?.slice(0, 100),
            })),
          }),
        );
      }
    });
  }
}
