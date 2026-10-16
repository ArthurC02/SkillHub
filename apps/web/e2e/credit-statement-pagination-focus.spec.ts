import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { stubPlatform } from "./stub";

const entry = {
  id: "credit-1",
  kind: "grant",
  label: "初始點數紀錄",
  delta_credits: 40,
  estimated: false,
  created_at: "2026-08-17T09:00:00Z",
};

for (const arrival of ["populated", "empty", "error", "more"] as const) {
  for (const moved of [false, true]) {
    test(`credit statement ${arrival} pagination ${moved ? "preserves moved focus" : "continues keyboard focus"}`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const cursors: Array<string | null> = [];
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route("**/me/credits/entries**", async (route) => {
        const cursor = new URL(route.request().url()).searchParams.get("before");
        cursors.push(cursor);
        if (!cursor)
          return route.fulfill({
            json: { entries: [entry], next_before: "credit-1", note: "" },
          });
        if (cursor === "credit-2") return route.fulfill({ json: { entries: [], note: "" } });
        await gate;
        if (arrival === "error")
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        return route.fulfill({
          json: {
            entries:
              arrival === "empty"
                ? []
                : [
                    {
                      ...entry,
                      id: "credit-2",
                      kind: "adjustment",
                      label: "較早的點數紀錄",
                      delta_credits: -25,
                    },
                  ],
            next_before: arrival === "more" ? "credit-2" : undefined,
            note: "",
          },
        });
      });
      try {
        await page.goto("/workspace/account");
        const statement = page.getByRole("region", { name: "點數紀錄", exact: true });
        const rows = statement.locator(".download-list > li");
        const more = statement
          .getByRole("button")
          .filter({ hasText: /^(載入更早的紀錄|載入中…)$/ });
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
        if (arrival === "error")
          await expect(statement.getByRole("alert")).toContainText("尚未讀完");
        await expect(rows).toHaveCount(arrival === "empty" || arrival === "error" ? 1 : 2);
        if (arrival !== "more") await expect(more).toHaveCount(0);
        if (moved) await expect(navigation).toBeFocused();
        else if (arrival === "error")
          await expect(page.locator("#account-credits-heading")).toBeFocused();
        else if (arrival === "more") {
          await expect(more).toBeFocused();
          await page.keyboard.press("Enter");
          await expect(more).toHaveCount(0);
          await expect(rows.filter({ hasText: "較早的點數紀錄" })).toBeFocused();
        } else
          await expect(
            rows.filter({ hasText: arrival === "empty" ? entry.label : "較早的點數紀錄" }),
          ).toBeFocused();
        if (arrival !== "empty" && arrival !== "error")
          await expect(rows.filter({ hasText: "較早的點數紀錄" })).toContainText("-25 點");
        expect(cursors).toEqual(
          arrival === "more" && !moved ? [null, "credit-1", "credit-2"] : [null, "credit-1"],
        );
        expect(writes).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        const report = await new AxeBuilder({ page }).analyze();
        await writeFile(
          test.info().outputPath("credit-pagination-a11y.json"),
          JSON.stringify(report),
        );
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        await page.screenshot({ path: test.info().outputPath("credit-pagination.png") });
      } finally {
        release();
        await writeFile(
          test.info().outputPath("credit-pagination-state.json"),
          JSON.stringify({
            arrival,
            moved,
            cursors,
            writes,
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
