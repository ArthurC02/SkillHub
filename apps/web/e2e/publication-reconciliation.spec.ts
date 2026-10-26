import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import {
  OWN_PUBLICATION,
  OWN_PUBLICATIONS,
  PUBLISHER,
  SKILL,
  VERSION,
  skillDetail,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

type Scenario = {
  action: "publish" | "delist" | "first";
  held: "own" | "overview" | "write";
  status: number;
  readFailure?: boolean;
};
const scenarios: Scenario[] = [];
for (const action of ["publish", "delist"] as const) {
  for (const held of ["own", "overview"] as const) {
    for (const status of [200, 503]) scenarios.push({ action, held, status });
  }
  scenarios.push({ action, held: "own", status: 503, readFailure: true });
}
for (const status of [201, 503]) scenarios.push({ action: "first", held: "write", status });

for (const width of [375, 1280]) {
  for (const scenario of scenarios) {
    test(`publication ${scenario.action} waits for ${scenario.held} after ${scenario.status}${scenario.readFailure ? " and read failure" : ""} at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      const { release, reads, writes } = await stubPublicationOperation(page, scenario);
      try {
        await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
        const section = page.getByRole("region", { name: "發佈", exact: true });
        const heading = section.getByRole("heading", { name: "發佈", exact: true });
        const attestation = section.getByRole("checkbox", { name: "我有權散布這些內容" });
        const license = section.getByRole("combobox", { name: "授權條款（選填）" });
        if (scenario.action === "first") {
          const input = section.getByRole("textbox", { name: "發佈名稱", exact: true });
          await expect(input).toHaveAccessibleDescription(
            "首次發佈後，這個小工具的發佈名稱就不能更改。",
          );
          await expect(
            section.getByText("首次發佈後，這個小工具的發佈名稱就不能更改。", { exact: true }),
          ).toBeVisible();
          await input.fill("chosen-name");
        } else await expect(section).toContainText("已列入 Catalog");
        await attestation.check();
        await license.selectOption("MIT");
        if (scenario.action === "delist") {
          await section.getByRole("button", { name: "撤回", exact: true }).click();
          await section.getByRole("button", { name: "確認撤回", exact: true }).click();
        } else await section.getByRole("button", { name: "發佈 v2", exact: true }).click();
        await expect.poll(() => writes.length).toBe(1);
        if (scenario.held !== "write")
          await expect.poll(() => reads[scenario.held as "own" | "overview"]).toBe(2);
        await expect(heading).toBeFocused();
        await expect(attestation).toBeDisabled();
        await expect(license).toBeDisabled();
        const submit = section.getByRole("button", {
          name: scenario.action === "delist" ? "發佈 v2" : "送出中…",
          exact: true,
        });
        await expect(submit).toBeDisabled();
        if (scenario.action === "publish")
          await expect(section.getByRole("button", { name: "撤回", exact: true })).toBeDisabled();
        if (scenario.action === "first")
          await expect(
            section.getByRole("textbox", { name: "發佈名稱", exact: true }),
          ).toBeDisabled();
        await expect(section).not.toContainText("上面的狀態已重新讀取");
        if (scenario.action !== "first")
          await expect(
            section.getByRole("status").filter({ hasText: "正在送出或核對發佈狀態" }),
          ).toBeVisible();
        const logout = page.getByRole("button", { name: "登出", exact: true });
        if (scenario.held === "overview") await logout.focus();
        release();
        await assertReconciledPublication(page, scenario, writes);
        const report = await new AxeBuilder({ page }).include("main").analyze();
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        if (scenario.action === "first" || scenario.readFailure) {
          await section.scrollIntoViewIfNeeded();
          await page.screenshot({ path: test.info().outputPath("publication-reconciled.png") });
        }
      } finally {
        release();
      }
    });
  }
}

async function stubPublicationOperation(page: Page, scenario: Scenario) {
  await stubPlatform(page);
  await page.route(`**/api/skills/${SKILL}*`, (route) =>
    route.fulfill({
      json: {
        ...skillDetail(SKILL, "PDF Summariser"),
        redistribution: { value: "generated", label: "平台生成", note: "" },
      },
    }),
  );
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let written = false;
  let published = scenario.action !== "first";
  const reads = { own: 0, overview: 0 };
  const writes: { method: string; body: unknown }[] = [];
  const name = scenario.action === "first" ? "chosen-name" : OWN_PUBLICATION.name;
  const changed = {
    ...OWN_PUBLICATION,
    name,
    address: `/p/${PUBLISHER}/${name}`,
    status: scenario.action === "delist" ? "delisted" : "published",
    availability: {
      value: scenario.action === "delist" ? "delisted" : "available",
      label: "目前狀態",
      note: "目前取得條件。",
    },
    acquisition: { available: scenario.action !== "delist", note: "目前取得條件。" },
  };
  await page.route(`**/skills/${SKILL}/publication`, async (route) => {
    if (route.request().method() !== "GET") {
      writes.push({
        method: route.request().method(),
        body: route.request().postData() ? route.request().postDataJSON() : null,
      });
      if (scenario.held === "write") await gate;
      written = true;
      if (scenario.action === "first") published = scenario.status === 201;
      return route.fulfill({
        status: scenario.status,
        json: scenario.status === 503 ? { error: "upstream unavailable" } : changed,
      });
    }
    reads.own += 1;
    if (written && scenario.held === "own") await gate;
    if (scenario.readFailure && reads.own === 2)
      return route.fulfill({ status: 503, json: { error: "unavailable" } });
    return published
      ? route.fulfill({ json: written ? changed : OWN_PUBLICATION })
      : route.fulfill({ status: 404, json: { error: "not published" } });
  });
  await page.route("**/me/publications", async (route) => {
    reads.overview += 1;
    if (written && scenario.held === "overview") await gate;
    return route.fulfill({
      json: {
        publications: OWN_PUBLICATIONS.publications.map((row) => ({
          ...row,
          ...(written
            ? {
                name,
                address: changed.address,
                status: changed.status,
                acquisition: changed.acquisition,
                catalog_exposure: {
                  state: scenario.action === "delist" ? "not_eligible" : "listed",
                },
              }
            : {}),
        })),
      },
    });
  });

  return { release, reads, writes };
}

async function assertReconciledPublication(
  page: Page,
  scenario: Scenario,
  writes: { method: string; body: unknown }[],
) {
  const section = page.getByRole("region", { name: "發佈", exact: true });
  const heading = section.getByRole("heading", { name: "發佈", exact: true });
  const attestation = section.getByRole("checkbox", { name: "我有權散布這些內容" });
  const license = section.getByRole("combobox", { name: "授權條款（選填）" });
  const logout = page.getByRole("button", { name: "登出", exact: true });
  if (scenario.readFailure) {
    await expect(section.getByRole("alert")).toContainText("暫時無法讀取發佈狀態");
    await expect(section.getByRole("button", { name: "發佈 v2", exact: true })).toHaveCount(0);
    await section.getByRole("button", { name: "再試一次", exact: true }).click();
  }
  await expect(section.getByRole("button", { name: "發佈 v2", exact: true })).toBeEnabled();
  await expect(attestation).toBeEnabled();
  await expect(license).toBeEnabled();
  await expect(scenario.held === "overview" ? logout : heading).toBeFocused();
  if (scenario.status === 503)
    await expect(section.getByRole("alert")).toContainText("上面的狀態已重新讀取");
  else await expect(section.getByRole("alert")).toHaveCount(0);
  if (scenario.action === "first" && scenario.status === 503)
    await expect(section.getByRole("textbox", { name: "發佈名稱", exact: true })).toHaveValue(
      "chosen-name",
    );
  else
    await expect(section.getByRole("status").filter({ hasText: /^狀態：/ })).toContainText(
      scenario.action === "delist" ? "已撤回" : "已發佈",
    );
  expect(writes).toEqual([
    {
      method: scenario.action === "delist" ? "DELETE" : "POST",
      body:
        scenario.action === "delist"
          ? null
          : {
              ...(scenario.action === "first" ? { name: "chosen-name" } : {}),
              version_id: VERSION,
              rights_attested: true,
              declared_license: "MIT",
            },
    },
  ]);
}
