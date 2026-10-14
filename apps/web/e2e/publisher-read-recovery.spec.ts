import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { OWN_PUBLICATION, OWN_PUBLISHER, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const owners = [
  {
    name: "panel publisher",
    url: `/skills/${SKILL}/versions/${VERSION}`,
    region: "發佈",
    what: "發佈者資訊",
    path: "/me/publisher",
  },
  {
    name: "panel publication",
    url: `/skills/${SKILL}/versions/${VERSION}`,
    region: "發佈",
    what: "發佈狀態",
    path: `/skills/${SKILL}/publication`,
  },
  {
    name: "workspace publisher",
    url: "/workspace/downloads",
    region: "發佈者名稱",
    what: "發佈者名稱",
    path: "/me/publisher",
  },
];

for (const owner of owners) {
  for (const status of [503, 403, 401]) {
    test(`${owner.name} ${status} withdraws cached identity and publication actions and ${status === 401 ? "requires sign in" : "retries its owner"}`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let failing = false;
      const reads = { publisher: 0, publication: 0 };
      for (const [key, path, body] of [
        ["publisher", "/me/publisher", OWN_PUBLISHER],
        ["publication", `/skills/${SKILL}/publication`, OWN_PUBLICATION],
      ] as const) {
        await page.route(`**${path}`, (route) => {
          reads[key] += 1;
          return failing && path === owner.path
            ? route.fulfill({ status, json: { error: "unavailable" } })
            : route.fulfill({ json: body });
        });
      }
      await page.goto(owner.url);
      const section = page.getByRole("region", { name: owner.region, exact: true });
      await expect(section).toContainText(
        owner.name === "workspace publisher" ? OWN_PUBLISHER.name : "狀態：已發佈",
      );
      await page.locator('a[href="/policy"]').first().click();
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
      failing = true;
      await page.goBack();
      if (status === 401)
        await expect(section.getByRole("status")).toContainText(`${owner.what}需要登入`);
      else await expect(section.getByRole("alert")).toContainText(`暫時無法讀取${owner.what}`);
      await expect(section).not.toContainText("狀態：已發佈");
      await expect(section).not.toContainText("最新 Release");
      await expect(section.getByText(OWN_PUBLISHER.name, { exact: false })).toHaveCount(0);
      await expect(section.getByRole("button", { name: "發佈 v2", exact: true })).toHaveCount(0);
      await expect(section.getByRole("button", { name: "撤回", exact: true })).toHaveCount(0);
      await expect(section.getByRole("button", { name: "註冊", exact: true })).toHaveCount(0);
      if (status === 401) {
        await expect(section.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
        expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
        return;
      }
      const before = { ...reads };
      failing = false;
      await section.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(section).toContainText(
        owner.name === "workspace publisher" ? OWN_PUBLISHER.name : "狀態：已發佈",
      );
      await expect(section.getByRole("heading", { name: owner.region, exact: true })).toBeFocused();
      expect(reads.publisher - before.publisher).toBe(owner.path === "/me/publisher" ? 1 : 0);
      expect(reads.publication - before.publication).toBe(owner.path === "/me/publisher" ? 0 : 1);
      expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      if (owner.name === "panel publication" && status === 503) {
        await page.screenshot({
          path: test.info().outputPath("publication-recovered-phone.png"),
          fullPage: false,
        });
      }
    });
  }
}

for (const owner of owners.filter((candidate) => candidate.name !== "panel publisher")) {
  test(`${owner.name} cached 404 uses confirmed absence rather than old identity`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let missing = false;
    await page.route(`**${owner.path}`, (route) =>
      missing
        ? route.fulfill({ status: 404, json: { error: "not found" } })
        : route.fulfill({
            json: owner.name === "workspace publisher" ? OWN_PUBLISHER : OWN_PUBLICATION,
          }),
    );
    await page.goto(owner.url);
    const section = page.getByRole("region", { name: owner.region, exact: true });
    await expect(section).toContainText(
      owner.name === "workspace publisher" ? OWN_PUBLISHER.name : "狀態：已發佈",
    );
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    missing = true;
    await page.goBack();
    await expect(
      section.getByRole("textbox", {
        name: owner.name === "workspace publisher" ? "發佈者名稱" : "發佈名稱",
        exact: true,
      }),
    ).toBeVisible();
    await expect(section).not.toContainText("狀態：已發佈");
    await expect(section).not.toContainText("最新 Release");
    await expect(section.getByText(OWN_PUBLISHER.name, { exact: false })).toHaveCount(0);
    await expect(section.getByRole("button", { name: "撤回", exact: true })).toHaveCount(0);
    await expect(section.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
  });
}
