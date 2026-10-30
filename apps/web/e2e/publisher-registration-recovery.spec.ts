import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { OWN_PUBLISHER, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const owners = [
  { name: "workspace", url: "/workspace/downloads", region: "發佈者名稱" },
  { name: "version", url: `/skills/${SKILL}/versions/${VERSION}`, region: "發佈" },
];

for (const owner of owners) {
  for (const width of [375, 1280]) {
    for (const outcome of ["refused", "uncertain", "read-error", "registered"] as const) {
      test(`publisher registration ${owner.name} preserves ${outcome} through confirmation at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 900 });
        await stubPlatform(page);
        await page.route("**/me/publications", (route) =>
          route.fulfill({ json: { publications: [] } }),
        );
        let reads = 0;
        let releaseRead!: () => void;
        const reread = new Promise<void>((resolve) => {
          releaseRead = resolve;
        });
        const posts: unknown[] = [];
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.route(`**/skills/${SKILL}/publication`, (route) =>
          route.fulfill({ status: 404, json: { error: "not published" } }),
        );
        await page.route("**/me/publisher", async (route) => {
          if (route.request().method() === "POST") {
            posts.push(route.request().postDataJSON());
            return outcome === "refused"
              ? route.fulfill({
                  status: 409,
                  json: { error: "publisher name is taken", reason: "name_taken" },
                })
              : route.fulfill({ status: 503, json: { error: "upstream unavailable" } });
          }
          reads += 1;
          if (reads === 2) {
            await reread;
            if (outcome === "registered") return route.fulfill({ json: OWN_PUBLISHER });
            if (outcome === "read-error")
              return route.fulfill({ status: 503, json: { error: "unavailable" } });
          }
          return route.fulfill({ status: 404, json: { error: "no publisher" } });
        });
        try {
          await page.goto(owner.url);
          const section = page.getByRole("region", { name: owner.region, exact: true });
          const heading = section.getByRole("heading", { name: owner.region, exact: true });
          const input = section.getByRole("textbox", { name: "發佈者名稱", exact: true });
          await input.fill("kept-namespace");
          await input.press("Enter");
          await expect.poll(() => reads).toBe(2);
          await expect(section.locator("form.publisher-form")).toHaveCount(0);
          await expect(section).not.toContainText("還沒有註冊發佈者名稱");
          await expect(heading).toBeFocused();
          releaseRead();
          if (outcome === "registered") {
            if (owner.name === "workspace") await expect(section).toContainText(OWN_PUBLISHER.name);
            else await expect(section.getByLabel("發佈名稱", { exact: true })).toBeVisible();
            await expect(section.locator("form.publisher-form")).toHaveCount(0);
            await expect(section).not.toContainText("尚未確認註冊");
          } else {
            if (outcome === "read-error") {
              await expect(section.getByRole("alert")).toContainText("暫時無法讀取發佈者");
              await expect(input).toHaveCount(0);
              await expect(section).not.toContainText("還沒有註冊發佈者名稱");
              await section.getByRole("button", { name: "再試一次", exact: true }).click();
            }
            await expect(input).toHaveValue("kept-namespace");
            await expect(section.getByRole("alert")).toHaveText(
              outcome === "refused"
                ? "這個名稱已經被別人取走了，換一個名稱再試一次。"
                : "尚未確認註冊是否已完成；上面的狀態已重新讀取，請先看過再決定要不要重送。",
            );
          }
          await expect(heading).toBeFocused();
          expect(posts).toEqual([{ name: "kept-namespace" }]);
          expect(reads).toBe(outcome === "read-error" ? 3 : 2);
          expect(errors).toEqual([]);
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
          ).toBe(true);
          const report = await new AxeBuilder({ page }).include("main").analyze();
          expect(report.violations).toEqual([]);
          expect(report.incomplete).toEqual([]);
          if (outcome === "read-error") {
            await section.scrollIntoViewIfNeeded();
            await page.screenshot({ path: test.info().outputPath("registration-recovered.png") });
          }
        } finally {
          releaseRead();
        }
      });
    }
  }
}
