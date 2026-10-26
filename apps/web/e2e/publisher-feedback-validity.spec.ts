import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const owners = [
  { name: "workspace", url: "/workspace/downloads", region: "發佈者名稱" },
  { name: "version", url: `/skills/${SKILL}/versions/${VERSION}`, region: "發佈" },
];
const outcomes = ["name_taken", "future_reason", "__proto__", "constructor", "uncertain"];

for (const owner of owners) {
  for (const width of [375, 1280]) {
    for (const outcome of outcomes) {
      test(`publisher feedback ${owner.name} ${outcome} protects the submitted draft at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 900 });
        await stubPlatform(page);
        await page.route("**/me/publications", (route) =>
          route.fulfill({ json: { publications: [] } }),
        );
        let releasePost!: () => void;
        const response = new Promise<void>((resolve) => {
          releasePost = resolve;
        });
        const posts: unknown[] = [];
        const errors: string[] = [];
        let reads = 0;
        page.on("pageerror", (error) => errors.push(error.message));
        await page.route(`**/skills/${SKILL}/publication`, (route) =>
          route.fulfill({ status: 404, json: { error: "not published" } }),
        );
        await page.route("**/me/publisher", async (route) => {
          if (route.request().method() === "GET") {
            reads += 1;
            return route.fulfill({ status: 404, json: { error: "no publisher" } });
          }
          posts.push(route.request().postDataJSON());
          await response;
          return outcome === "uncertain"
            ? route.fulfill({ status: 503, json: { error: "upstream unavailable" } })
            : route.fulfill({
                status: 409,
                json: { error: "publisher registration refused", reason: outcome },
              });
        });
        try {
          await page.goto(owner.url);
          const section = page.getByRole("region", { name: owner.region, exact: true });
          const input = section.getByRole("textbox", { name: "發佈者名稱", exact: true });
          await input.fill("submitted-name");
          await section.getByRole("button", { name: "註冊", exact: true }).click();
          await expect(
            section.getByRole("button", { name: "註冊中…", exact: true }),
          ).toBeDisabled();
          await expect(input).toBeDisabled();
          await expect.poll(() => posts).toEqual([{ name: "submitted-name" }]);
          releasePost();
          await expect(input).toBeEnabled();
          await expect(input).toHaveValue("submitted-name");
          const alert = section.getByRole("alert");
          const sentence =
            outcome === "name_taken"
              ? "這個名稱已經被別人取走了，換一個名稱再試一次。"
              : outcome === "uncertain"
                ? "尚未確認註冊是否已完成；上面的狀態已重新讀取，請先看過再決定要不要重送。"
                : "註冊沒有成功，可以再試一次。";
          await expect(alert).toHaveText(sentence);
          await input.fill("new-name");
          if (outcome === "uncertain") await expect(alert).toHaveText(sentence);
          else await expect(alert).toHaveCount(0);
          await expect(input).toHaveValue("new-name");
          expect(posts).toEqual([{ name: "submitted-name" }]);
          await expect.poll(() => reads).toBe(2);
          expect(errors).toEqual([]);
          await expect(page.getByText("這一頁暫時無法顯示", { exact: false })).toHaveCount(0);
          const report = await new AxeBuilder({ page }).include("main").analyze();
          expect(report.violations).toEqual([]);
          expect(report.incomplete).toEqual([]);
          expect(
            await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
          ).toBe(true);
          if (outcome === "__proto__") {
            await section.scrollIntoViewIfNeeded();
            await page.screenshot({
              path: test.info().outputPath("publisher-feedback-edited.png"),
            });
          }
        } finally {
          releasePost();
        }
      });
    }
  }
}
