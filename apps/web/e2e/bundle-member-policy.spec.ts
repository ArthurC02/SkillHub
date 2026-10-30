import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { OWN_BUNDLE, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  for (const missing of ["one member", "all members"]) {
    test(`Bundle redistribution stays unconfirmed with ${missing} absent and recovers through refresh at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      const base = (platformResponse("/skills").body as { skills: object[] }).skills[0];
      const rows = OWN_BUNDLE.members.map((member) => ({
        ...base,
        skill_id: member.skill_id,
        name: member.name,
      }));
      let reads = 0;
      const writes: string[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      await page.route("**/me/bundles", (route) =>
        route.fulfill({ json: { bundles: [OWN_BUNDLE] } }),
      );
      await page.route("**/me/bundles/overview", (route) =>
        route.fulfill({
          json: { bundles: [{ bundle: OWN_BUNDLE.bundle, latest_version: OWN_BUNDLE }] },
        }),
      );
      await page.route("**/skills?*", (route) => {
        reads++;
        const skills = reads > 1 ? rows : missing === "all members" ? [] : rows.slice(0, 1);
        return route.fulfill({
          json: { skills, total: skills.length, limit: 24, truncated: false },
        });
      });
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/workspace/downloads");
      const section = page.locator("#bundle-workspace");
      const article = section.locator(`article[aria-labelledby="bundle-${OWN_BUNDLE.bundle}"]`);
      const publish = article.getByRole("button", {
        name: `首次發佈 v${OWN_BUNDLE.version}`,
        exact: true,
      });
      await expect(publish).toBeDisabled();
      await expect(publish).toHaveAccessibleDescription(/散布需求尚未確認/);
      await expect(article.getByText("散布需求尚未確認", { exact: false })).toBeVisible();
      await expect(section.getByRole("alert")).toHaveCount(0);
      const unknown = await new AxeBuilder({ page })
        .include(`article[aria-labelledby="bundle-${OWN_BUNDLE.bundle}"]`)
        .analyze();
      await writeFile(info.outputPath("unknown-a11y.json"), JSON.stringify(unknown));
      expect(unknown.violations).toEqual([]);
      expect(unknown.incomplete).toEqual([]);
      await publish.scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath("unknown.png") });
      await section.getByRole("button", { name: "重新整理", exact: true }).click();
      await expect(publish).toBeEnabled();
      await expect(article).not.toContainText("散布需求尚未確認");
      expect(reads).toBe(2);
      expect(writes).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      const known = await new AxeBuilder({ page })
        .include(`article[aria-labelledby="bundle-${OWN_BUNDLE.bundle}"]`)
        .analyze();
      await writeFile(info.outputPath("known-a11y.json"), JSON.stringify(known));
      expect(known.violations).toEqual([]);
      expect(known.incomplete).toEqual([]);
      await publish.scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath("known.png") });
    });
  }
}
