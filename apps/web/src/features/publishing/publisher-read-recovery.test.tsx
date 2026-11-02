import { expect, test } from "vitest";
import { OWN_PUBLICATION, OWN_PUBLISHER, SKILL, VERSION } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

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

async function leaveAndReturn(page: ReturnType<typeof app>, change: () => void) {
  await page.locator('a[href="/policy"]').first().click();
  await page.eventually(() => {
    expect(page.pathname()).toBe("/policy");
    expect(page.getByRole("heading", { name: "資料保存政策", exact: true }).count()).toBe(1);
  });
  change();
  await page.goBack();
}

for (const owner of owners) {
  for (const status of [503, 403, 401]) {
    test(`${owner.name} ${status} withdraws cached identity and publication actions and ${status === 401 ? "requires sign in" : "retries its owner"}`, async () => {
      const page = app();
      let failing = false;
      const reads = { publisher: 0, publication: 0 };
      for (const [key, path, body] of [
        ["publisher", "/me/publisher", OWN_PUBLISHER],
        ["publication", `/skills/${SKILL}/publication`, OWN_PUBLICATION],
      ] as const) {
        page.route(`**${path}`, () => {
          reads[key] += 1;
          return failing && path === owner.path
            ? { status, json: { error: "unavailable" } }
            : { json: body };
        });
      }
      await page.goto(owner.url);
      const section = page.getByRole("region", { name: owner.region, exact: true });
      const identity = owner.name === "workspace publisher" ? OWN_PUBLISHER.name : "狀態：已發佈";
      await page.eventually(() => expect(section.text()).toContain(identity));
      await leaveAndReturn(page, () => {
        failing = true;
      });
      if (status === 401)
        await page.eventually(() =>
          expect(section.locator("[role=status]").text()).toContain(`${owner.what}需要登入`),
        );
      else
        await page.eventually(() =>
          expect(section.locator("[role=alert]").text()).toContain(`暫時無法讀取${owner.what}`),
        );
      expect(section.text()).not.toContain("狀態：已發佈");
      expect(section.text()).not.toContain("最新 Release");
      expect(section.text()).not.toContain(OWN_PUBLISHER.name);
      for (const label of ["發佈 v2", "撤回", "註冊"])
        expect(section.locator("button").filter({ hasText: label }).count()).toBe(0);
      if (status === 401) {
        expect(section.locator("button").filter({ hasText: "再試一次" }).count()).toBe(0);
        expect(await page.axeViolations("main")).toEqual([]);
        return;
      }
      const before = { ...reads };
      failing = false;
      await section.locator("button").filter({ hasText: "再試一次" }).click();
      await page.eventually(() => {
        expect(section.text()).toContain(identity);
        expect(section.locator("h2").filter({ hasText: owner.region }).first().isFocused()).toBe(
          true,
        );
      });
      expect(reads.publisher - before.publisher).toBe(owner.path === "/me/publisher" ? 1 : 0);
      expect(reads.publication - before.publication).toBe(owner.path === "/me/publisher" ? 0 : 1);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }
}

for (const owner of owners.filter((candidate) => candidate.name !== "panel publisher")) {
  test(`${owner.name} cached 404 uses confirmed absence rather than old identity`, async () => {
    const page = app();
    let missing = false;
    page.route(`**${owner.path}`, () =>
      missing
        ? { status: 404, json: { error: "not found" } }
        : { json: owner.name === "workspace publisher" ? OWN_PUBLISHER : OWN_PUBLICATION },
    );
    await page.goto(owner.url);
    const section = page.getByRole("region", { name: owner.region, exact: true });
    const identity = owner.name === "workspace publisher" ? OWN_PUBLISHER.name : "狀態：已發佈";
    await page.eventually(() => expect(section.text()).toContain(identity));
    await leaveAndReturn(page, () => {
      missing = true;
    });
    await page.eventually(() => expect(section.locator("input").count()).toBe(1));
    expect(section.text()).not.toContain("狀態：已發佈");
    expect(section.text()).not.toContain("最新 Release");
    expect(section.text()).not.toContain(OWN_PUBLISHER.name);
    expect(section.locator("button").filter({ hasText: "撤回" }).count()).toBe(0);
    expect(section.locator("button").filter({ hasText: "再試一次" }).count()).toBe(0);
  });
}
