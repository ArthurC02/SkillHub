import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { VERSION, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  for (const state of ["paused", "fetching"]) {
    for (const focus of ["new member", "edited field"]) {
      test(`Bundle member pagination that is ${state} preserves the draft and focuses the ${focus} at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        const base = (platformResponse("/skills").body as { skills: Record<string, unknown>[] })
          .skills[0];
        const rows = Array.from({ length: 25 }, (_, index) => ({
          ...base,
          skill_id:
            index === 0
              ? base.skill_id
              : `a0000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
          name: `Bundle member ${index + 1}`,
        }));
        let finish!: () => void;
        const held = new Promise<void>((resolve) => {
          finish = resolve;
        });
        const offsets: number[] = [];
        const writes: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        await page.route("**/skills?*", async (route) => {
          const offset = Number(new URL(route.request().url()).searchParams.get("offset"));
          offsets.push(offset);
          if (offset > 0) await held;
          await route.fulfill({
            json: {
              skills: rows.slice(offset, offset + 24),
              limit: 24,
              total: 25,
              truncated: offset === 0,
            },
          });
        });
        await page.setViewportSize({ width, height: 900 });
        await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
        const section = page.locator("#bundle-workspace");
        const form = section.locator(".bundle-form");
        const member = form.locator(`[data-bundle-version="${VERSION}"]`);
        const create = form.getByRole("button", { name: "建立", exact: true });
        const more = section.getByRole("button", { name: "載入更多小工具", exact: true });
        try {
          await expect(member).toHaveValue(VERSION);
          await expect(create).toBeEnabled();
          await form.getByLabel("名稱", { exact: true }).fill("kept-bundle");
          await form.getByLabel("版本（semver，例如 1.0.0）", { exact: true }).fill("1.2.0");
          await form.getByLabel("說明", { exact: true }).fill("保留這段描述");
          if (state === "paused") await page.context().setOffline(true);
          await more.click();
          const status = section.locator("#bundle-members-read-status");
          await expect(create).toBeDisabled();
          await expect(member).toBeDisabled();
          if (state === "paused") await expect(more).toBeDisabled();
          await expect(status).toContainText(
            state === "paused" ? "等待連線" : "載入可加入 Bundle 的版本中",
          );
          await expect(member).toHaveAccessibleDescription(
            state === "paused" ? /等待連線/ : /載入可加入 Bundle 的版本中/,
          );
          await expect(section).not.toContainText("還沒有可以選的小工具");
          const pendingA11y = await new AxeBuilder({ page }).include("#bundle-workspace").analyze();
          await writeFile(info.outputPath("pending-a11y.json"), JSON.stringify(pendingA11y));
          expect(pendingA11y.violations).toEqual([]);
          expect(pendingA11y.incomplete).toEqual([]);
          await page.screenshot({ path: info.outputPath("pending.png") });
          if (focus === "edited field")
            await form.getByLabel("名稱", { exact: true }).fill("continued-bundle");
          await page.context().setOffline(false);
          finish();
          await expect(create).toBeEnabled();
          await expect(member).toBeEnabled();
          await expect(member).toHaveValue(VERSION);
          await expect(form.getByLabel("名稱", { exact: true })).toHaveValue(
            focus === "edited field" ? "continued-bundle" : "kept-bundle",
          );
          await expect(form.getByLabel("說明", { exact: true })).toHaveValue("保留這段描述");
          await expect(status).toHaveCount(0);
          await expect(more).toHaveCount(0);
          await expect(
            focus === "edited field"
              ? form.getByLabel("名稱", { exact: true })
              : form.locator("#bundle-member-a0000000-0000-4000-8000-000000000024"),
          ).toBeFocused();
          expect(offsets).toEqual([0, 24]);
          expect(writes).toEqual([]);
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
          ).toBe(true);
          const recoveredA11y = await new AxeBuilder({ page })
            .include("#bundle-workspace")
            .analyze();
          await writeFile(info.outputPath("recovered-a11y.json"), JSON.stringify(recoveredA11y));
          expect(recoveredA11y.violations).toEqual([]);
          expect(recoveredA11y.incomplete).toEqual([]);
          await page.screenshot({ path: info.outputPath("recovered.png") });
        } finally {
          finish();
          await page.context().setOffline(false);
        }
      });
    }
  }
}
