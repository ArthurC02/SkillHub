import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { OWN_PUBLICATION, OWN_PUBLISHER, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const owners = [
  {
    name: "workspace publisher",
    entry: "workspace",
    url: "/workspace/downloads",
    region: "發佈者名稱",
    what: "發佈者名稱",
    key: "publisher",
    path: "/me/publisher",
  },
  {
    name: "version publisher",
    entry: "version",
    url: `/skills/${SKILL}/versions/${VERSION}`,
    region: "發佈",
    what: "發佈者資訊",
    key: "publisher",
    path: "/me/publisher",
  },
  {
    name: "version publication",
    entry: "version",
    url: `/skills/${SKILL}/versions/${VERSION}`,
    region: "發佈",
    what: "發佈狀態",
    key: "publication",
    path: `/skills/${SKILL}/publication`,
  },
] as const;
type Owner = (typeof owners)[number];
type Scenario = {
  name: string;
  owner: Owner;
  phase: "initial" | "refresh" | "registration";
  invalid: unknown;
  postStatus?: number;
};
const scenarios: Scenario[] = [];
for (const owner of owners) {
  scenarios.push({
    name: owner.name + " invalid initial read",
    owner,
    phase: "initial",
    invalid: owner.key === "publisher" ? null : {},
  });
  scenarios.push({
    name: owner.name + " invalid cached refresh",
    owner,
    phase: "refresh",
    invalid: owner.key === "publisher" ? null : { ...OWN_PUBLICATION, status: "future" },
  });
  if (owner.key === "publisher")
    for (const postStatus of [409, 503])
      scenarios.push({
        name: owner.name + " invalid registration lookup after " + postStatus,
        owner,
        phase: "registration",
        invalid: null,
        postStatus,
      });
}
scenarios.push({
  name: "publication unknown initial status",
  owner: owners[2],
  phase: "initial",
  invalid: { ...OWN_PUBLICATION, status: "future" },
});

for (const width of [375, 1280]) {
  for (const scenario of scenarios) {
    test(`owner detail ${scenario.name} at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const { reads, writes, ready } = await stubOwnerRead(page, scenario);
      const { owner } = scenario;
      await Promise.all([ready, page.goto(owner.url)]);
      const section = page.getByRole("region", { name: owner.region, exact: true });
      if (scenario.phase === "refresh") {
        await expect(section).toContainText(
          owner.entry === "workspace" ? OWN_PUBLISHER.name : "狀態：已發佈",
        );
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        await Promise.all([ownerReadResponse(page, owner.path), page.goBack()]);
      }
      if (scenario.phase === "registration") {
        await section
          .getByRole("textbox", { name: "發佈者名稱", exact: true })
          .fill("kept-after-invalid-read");
        await Promise.all([
          ownerReadResponse(page, owner.path),
          section.getByRole("button", { name: "註冊", exact: true }).click(),
        ]);
      }
      await expect(section.getByRole("alert")).toContainText(`暫時無法讀取${owner.what}`);
      await expect(section).not.toContainText("狀態：已發佈");
      await expect(section).not.toContainText("已撤回");
      await expect(section).not.toContainText("還沒有註冊發佈者名稱");
      await expect(section.getByRole("textbox")).toHaveCount(0);
      await expect(section.getByRole("button", { name: "發佈 v2", exact: true })).toHaveCount(0);
      await expect(section.getByRole("button", { name: "撤回", exact: true })).toHaveCount(0);
      const accessibility = await new AxeBuilder({ page }).include("main").analyze();
      expect(accessibility.violations).toEqual([]);
      expect(accessibility.incomplete).toEqual([]);
      if (scenario.phase === "initial") {
        await section.scrollIntoViewIfNeeded();
        await page.screenshot({ path: test.info().outputPath("owner-read-unknown.png") });
      }
      const before = { ...reads };
      await Promise.all([
        ownerReadResponse(page, owner.path),
        section.getByRole("button", { name: "再試一次", exact: true }).click(),
      ]);
      await assertOwnerRecovered(page, scenario);
      expect(reads.publisher - before.publisher).toBe(owner.key === "publisher" ? 1 : 0);
      expect(reads.publication - before.publication).toBe(owner.key === "publication" ? 1 : 0);
      expect(writes).toEqual(
        scenario.phase === "registration" ? [{ name: "kept-after-invalid-read" }] : [],
      );
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    });
  }
}

function ownerReadResponse(page: Page, path: string) {
  return page.waitForResponse(
    (reply) => reply.request().method() === "GET" && new URL(reply.url()).pathname.endsWith(path),
  );
}

async function stubOwnerRead(page: Page, scenario: Scenario) {
  await stubPlatform(page);
  const reads = { publisher: 0, publication: 0 };
  const writes: unknown[] = [];
  for (const [key, path, valid] of [
    ["publisher", "/me/publisher", OWN_PUBLISHER],
    ["publication", `/skills/${SKILL}/publication`, OWN_PUBLICATION],
  ] as const) {
    await page.route(`**${path}`, (route) => {
      if (route.request().method() !== "GET") {
        expect(scenario.phase).toBe("registration");
        expect(key).toBe("publisher");
        expect(route.request().method()).toBe("POST");
        writes.push(route.request().postDataJSON());
        return route.fulfill({
          status: scenario.postStatus,
          json:
            scenario.postStatus === 409
              ? { error: "publisher name is taken", reason: "name_taken" }
              : { error: "upstream unavailable" },
        });
      }
      reads[key] += 1;
      if (key !== scenario.owner.key) return route.fulfill({ json: valid });
      if (scenario.phase === "registration" && reads[key] !== 2)
        return route.fulfill({ status: 404, json: { error: "no publisher" } });
      return route.fulfill({
        json: reads[key] === (scenario.phase === "initial" ? 1 : 2) ? scenario.invalid : valid,
      });
    });
  }
  const ready = Promise.all([
    ownerReadResponse(page, "/me/publisher"),
    ...(scenario.owner.entry === "version"
      ? [ownerReadResponse(page, `/skills/${SKILL}/publication`)]
      : []),
  ]);
  return { reads, writes, ready };
}

async function assertOwnerRecovered(page: Page, scenario: Scenario) {
  const section = page.getByRole("region", { name: scenario.owner.region, exact: true });
  if (scenario.phase === "registration") {
    await expect(section.getByRole("textbox", { name: "發佈者名稱", exact: true })).toHaveValue(
      "kept-after-invalid-read",
    );
    await expect(section.getByRole("alert")).toHaveText(
      scenario.postStatus === 409
        ? "這個名稱已經被別人取走了，換一個名稱再試一次。"
        : "尚未確認註冊是否已完成；上面的狀態已重新讀取，請先看過再決定要不要重送。",
    );
  } else {
    await expect(section).toContainText(
      scenario.owner.entry === "workspace" ? OWN_PUBLISHER.name : "狀態：已發佈",
    );
    await expect(section.getByRole("alert")).toHaveCount(0);
  }
  await expect(
    section.getByRole("heading", { name: scenario.owner.region, exact: true }),
  ).toBeFocused();
}
