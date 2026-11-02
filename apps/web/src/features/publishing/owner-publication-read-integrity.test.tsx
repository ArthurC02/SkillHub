import { expect, test } from "vitest";
import { OWN_PUBLICATION, OWN_PUBLISHER, SKILL, VERSION } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

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

const TEXT_FIELDS = "input:not([type]), input[type=text], textarea, [role=textbox]";

function stubOwnerRead(page: App, scenario: Scenario) {
  const reads = { publisher: 0, publication: 0 };
  const writes: unknown[] = [];
  for (const [key, path, valid] of [
    ["publisher", "/me/publisher", OWN_PUBLISHER],
    ["publication", `/skills/${SKILL}/publication`, OWN_PUBLICATION],
  ] as const) {
    page.route(
      (url) => url.pathname.endsWith(path),
      ({ method, body }) => {
        if (method !== "GET") {
          writes.push(body);
          return {
            status: scenario.postStatus,
            json:
              scenario.postStatus === 409
                ? { error: "publisher name is taken", reason: "name_taken" }
                : { error: "upstream unavailable" },
          };
        }
        reads[key] += 1;
        if (key !== scenario.owner.key) return { json: valid };
        if (scenario.phase === "registration" && reads[key] !== 2)
          return { status: 404, json: { error: "no publisher" } };
        return {
          json: reads[key] === (scenario.phase === "initial" ? 1 : 2) ? scenario.invalid : valid,
        };
      },
    );
  }
  return { reads, writes };
}

function regionOf(page: App, owner: Owner) {
  return page.getByRole("region", { name: owner.region, exact: true });
}

function recovered(page: App, scenario: Scenario) {
  const section = regionOf(page, scenario.owner);
  if (scenario.phase === "registration") {
    expect(section.locator(TEXT_FIELDS).value()).toBe("kept-after-invalid-read");
    expect(section.locator("[role=alert]").text()).toBe(
      scenario.postStatus === 409
        ? "這個名稱已經被別人取走了，換一個名稱再試一次。"
        : "尚未確認註冊是否已完成；上面的狀態已重新讀取，請先看過再決定要不要重送。",
    );
  } else {
    expect(section.text()).toContain(
      scenario.owner.entry === "workspace" ? OWN_PUBLISHER.name : "狀態：已發佈",
    );
    expect(section.locator("[role=alert]").count()).toBe(0);
  }
  expect(page.getByRole("heading", { name: scenario.owner.region, exact: true }).isFocused()).toBe(
    true,
  );
}

for (const width of [375, 1280]) {
  for (const scenario of scenarios) {
    test(`owner detail ${scenario.name} at ${width}px`, async () => {
      const page = app();
      const { reads, writes } = stubOwnerRead(page, scenario);
      const { owner } = scenario;
      await page.goto(owner.url);
      const section = regionOf(page, owner);
      const sectionButton = (label: string) => section.locator("button").filter({ hasText: label });
      if (scenario.phase === "refresh") {
        await page.eventually(() =>
          expect(section.text()).toContain(
            owner.entry === "workspace" ? OWN_PUBLISHER.name : "狀態：已發佈",
          ),
        );
        await page.locator('a[href="/policy"]').first().click();
        await page.eventually(() =>
          expect(page.getByRole("heading", { name: "資料保存政策", exact: true }).count()).toBe(1),
        );
        await page.goBack();
      }
      if (scenario.phase === "registration") {
        await page.eventually(() => expect(section.locator(TEXT_FIELDS).count()).toBe(1));
        await section.locator(TEXT_FIELDS).fill("kept-after-invalid-read");
        await section.locator(TEXT_FIELDS).press("Enter");
      }
      await page.eventually(() =>
        expect(section.locator("[role=alert]").text()).toContain(`暫時無法讀取${owner.what}`),
      );
      expect(section.text()).not.toContain("狀態：已發佈");
      expect(section.text()).not.toContain("已撤回");
      expect(section.text()).not.toContain("還沒有註冊發佈者名稱");
      expect(section.locator("[role=textbox], input, textarea").count()).toBe(0);
      expect(sectionButton("發佈 v2").count()).toBe(0);
      expect(sectionButton("撤回").count()).toBe(0);
      expect(await page.axeViolations("main")).toEqual([]);
      const before = { ...reads };
      await sectionButton("再試一次").click();
      await page.eventually(() => recovered(page, scenario));
      expect(reads.publisher - before.publisher).toBe(owner.key === "publisher" ? 1 : 0);
      expect(reads.publication - before.publication).toBe(owner.key === "publication" ? 1 : 0);
      expect(writes).toEqual(
        scenario.phase === "registration" ? [{ name: "kept-after-invalid-read" }] : [],
      );
    });
  }
}
