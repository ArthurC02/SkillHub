import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ACTIVITY, PUBLICATION, PUBLISHER, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

type Arrival = "populated" | "empty" | "error" | "more";

async function activityPages(page: Page, arrival: Arrival) {
  await stubPlatform(page);
  const cursors: Array<string | null> = [];
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/me/activity?*", async (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor");
    cursors.push(cursor);
    if (!cursor) return route.fulfill({ json: { ...ACTIVITY, next_cursor: "older-page" } });
    if (cursor === "even-older") return route.fulfill({ json: { ...ACTIVITY, items: [] } });
    await gate;
    if (arrival === "error")
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    return route.fulfill({
      json: {
        ...ACTIVITY,
        ...(arrival === "more" ? { next_cursor: "even-older" } : {}),
        items:
          arrival === "empty"
            ? []
            : ["第一筆較早的試跑", "第二筆較早的試跑"].map((summary, index) => ({
                ...ACTIVITY.items[0],
                source_id: index === 0 ? RUN : "aaaaaaaa-9999-4999-8999-999999999999",
                summary,
                continuation: { kind: "run", run_id: RUN },
              })),
      },
    });
  });
  return { cursors, release };
}

for (const arrival of ["populated", "empty", "error", "more"] as const) {
  for (const moved of [false, true]) {
    test(`activity ${arrival} pagination ${moved ? "preserves moved focus" : "continues keyboard focus"}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const { cursors, release } = await activityPages(page, arrival);
      const rows = page.locator(".activity-list > li");
      try {
        await page.goto("/activity");
        await expect(rows).toHaveCount(3);
        const more = page.getByRole("button", { name: "載入更多活動", exact: true });
        await more.focus();
        await page.keyboard.press("Enter");
        await expect(page.getByRole("button", { name: "載入中…", exact: true })).toBeDisabled();
        const navigation = page
          .getByRole("navigation", { name: "主要導覽", exact: true })
          .getByRole("link", { name: "工作台", exact: true });
        if (moved) await navigation.focus();
        release();
        if (arrival === "error") {
          await expect(page.getByRole("alert")).toContainText("更早的活動讀取失敗");
          await expect(rows).toHaveCount(3);
        } else {
          await expect(rows).toHaveCount(arrival === "empty" ? 3 : 5);
          await expect(
            page
              .getByRole("status")
              .filter({ hasText: arrival === "more" ? "尚有更多活動" : "已載入全部活動" }),
          ).toBeVisible();
        }
        if (moved) await expect(navigation).toBeFocused();
        else if (arrival === "error") await expect(page.locator("#activity-heading")).toBeFocused();
        else if (arrival === "more") {
          await expect(more).toBeFocused();
          await page.keyboard.press("Enter");
          await expect(more).toHaveCount(0);
          await expect(rows).toHaveCount(5);
          await expect(rows.filter({ hasText: "第二筆較早的試跑" })).toBeFocused();
        } else {
          const continuation = rows.filter({
            hasText: arrival === "empty" ? `${PUBLISHER}/${PUBLICATION}` : "第一筆較早的試跑",
          });
          await expect(continuation).toBeFocused();
          expect(
            await continuation.evaluate((element) => {
              const rect = element.getBoundingClientRect();
              const top = document.elementFromPoint(
                rect.x + rect.width / 2,
                rect.y + rect.height / 2,
              );
              return Boolean(top && element.contains(top));
            }),
          ).toBe(true);
          if (arrival === "populated") {
            await page.keyboard.press("Tab");
            await expect(
              continuation.getByRole("link", { name: "PDF Summariser", exact: true }),
            ).toBeFocused();
          }
        }
        expect(cursors).toEqual(
          arrival === "more" && !moved ? [null, "older-page", "even-older"] : [null, "older-page"],
        );
        const report = await new AxeBuilder({ page }).analyze();
        await writeFile(
          test.info().outputPath("pagination-focus-a11y.json"),
          JSON.stringify(report),
        );
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
          375,
        );
        await page.screenshot({ path: test.info().outputPath("pagination-focus-phone.png") });
      } finally {
        release();
        await writeFile(
          test.info().outputPath("pagination-focus-state.json"),
          JSON.stringify({
            arrival,
            moved,
            cursors,
            focus: await page.evaluate(() => ({
              tag: document.activeElement?.tagName,
              id: document.activeElement?.id,
              text: document.activeElement?.textContent?.slice(0, 100),
            })),
          }),
        );
      }
    });
  }
}
