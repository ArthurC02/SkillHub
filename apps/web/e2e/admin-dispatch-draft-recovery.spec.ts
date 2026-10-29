import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_DISPATCH } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const DISPATCH = {
  ...ADMIN_DISPATCH,
  halts: [
    ...ADMIN_DISPATCH.halts,
    {
      ...ADMIN_DISPATCH.halts[0],
      target: "node-2",
      source: "orphan_threshold",
      reason: "orphan capacity reached",
      automatic_recovery: true,
      clear_rounds: 1,
    },
  ],
};

for (const width of [375, 1280]) {
  for (const outcome of ["503", "malformed"]) {
    test(`dispatch release draft survives ${outcome} and requires a deliberate confirmation at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const target = width === 375 ? "pool" : "node-2";
      let readMode = "valid";
      let released = false;
      let held: Promise<void> | undefined;
      let finishRead!: () => void;
      const writes: string[] = [];
      const bodies: unknown[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      await page.route("**/admin/dispatch", async (route) => {
        if (route.request().isNavigationRequest()) return route.fallback();
        if (held) await held;
        return route.fulfill(
          readMode === "503"
            ? { status: 503, json: { error: "service unavailable" } }
            : readMode === "malformed"
              ? { json: { dispatching: "false", halts: [] } }
              : {
                  json: released
                    ? {
                        dispatching: target === "pool",
                        halts: DISPATCH.halts.filter((halt) => halt.target !== target),
                      }
                    : DISPATCH,
                },
        );
      });
      await page.route("**/admin/dispatch/halt", (route) => {
        if (route.request().method() !== "DELETE")
          return route.fulfill({ status: 500, json: { error: "unexpected write" } });
        bodies.push(route.request().postDataJSON());
        released = true;
        return route.fulfill({ status: 204, body: "" });
      });
      await page.goto("/admin/dispatch");
      const select = page.locator("#admin-halt-lift-target");
      const note = page.locator("#admin-halt-lift-note");
      const section = page.locator('section[aria-labelledby="admin-dispatch-lift"]');
      const refresh = page.getByRole("button", { name: "重新整理派送狀態", exact: true });
      await select.selectOption(target);
      await note.fill("證據已保存，事故處置紀錄已核對");
      await page.getByRole("button", { name: "恢復派送", exact: true }).click();
      const confirm = page.getByRole("button", { name: "確認恢復派送", exact: true });
      await expect(confirm).toBeEnabled();
      readMode = outcome;
      held = new Promise<void>((resolve) => {
        finishRead = resolve;
      });
      await refresh.click();
      try {
        await expect(select).toBeDisabled();
        await expect(confirm).toBeDisabled();
        await expect(select).toHaveAccessibleDescription(
          "正在重新讀取派送狀態，完成後才能恢復派送。",
        );
        const accessibility = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(
          info.outputPath("dispatch-draft-pending-a11y.json"),
          JSON.stringify(accessibility),
        );
        expect(accessibility.violations).toEqual([]);
        expect(accessibility.incomplete).toEqual([]);
        expect(writes).toEqual([]);
        await select.scrollIntoViewIfNeeded();
        await page.screenshot({ path: info.outputPath("dispatch-draft-pending.png") });
      } finally {
        finishRead();
        held = undefined;
      }
      await expect(page.getByRole("alert")).toContainText("暫時無法讀取派送狀態");
      await expect(section).toHaveCount(1);
      await expect(section).toBeHidden();
      await expect(
        page.getByText("原本的欄位先保留，狀態恢復後再核對。", { exact: false }),
      ).toBeVisible();
      await expect(select).toHaveValue(target);
      await expect(note).toHaveValue("證據已保存，事故處置紀錄已核對");
      await expect(page.getByRole("button", { name: "確認恢復派送", exact: true })).toHaveCount(0);
      await page.screenshot({ path: info.outputPath("dispatch-draft-unreadable.png") });
      readMode = "valid";
      await refresh.click();
      await expect(section).toBeVisible();
      await expect(select).toHaveValue(target);
      await expect(note).toHaveValue("證據已保存，事故處置紀錄已核對");
      await expect(confirm).toBeEnabled();
      expect(writes).toEqual([]);
      await note.scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath("dispatch-draft-restored.png") });
      await confirm.click();
      await expect(page.locator("#admin-dispatch-lift-result")).toContainText(
        "已不在生效中的煞車清單",
      );
      await expect(page.locator("#admin-dispatch-lift-result")).toBeFocused();
      await expect(select).toHaveValue("");
      await expect(select.locator(`option[value="${target}"]`)).toHaveCount(0);
      await expect(
        select.locator(`option[value="${target === "pool" ? "node-2" : "pool"}"]`),
      ).toHaveCount(1);
      await expect(note).toHaveValue("");
      await expect(page.getByRole("button", { name: "恢復派送", exact: true })).toBeDisabled();
      expect(bodies).toEqual([
        target === "pool"
          ? { note: "證據已保存，事故處置紀錄已核對" }
          : { note: "證據已保存，事故處置紀錄已核對", provider: "node-2" },
      ]);
      expect(writes).toHaveLength(1);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    });
  }
}
