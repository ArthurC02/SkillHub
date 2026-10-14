import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  ACTIVITY,
  OWN_PUBLICATIONS,
  PUBLICATION,
  PUBLISHER,
  RUN,
  RUNS,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const status of [401, 403, 404]) {
  test(`activity pagination ${status} withholds previously loaded records`, async ({ page }) => {
    await stubPlatform(page);
    let firstReads = 0;
    let nextReads = 0;
    await page.route("**/me/activity?*", (route) => {
      if (new URL(route.request().url()).searchParams.has("cursor")) {
        nextReads += 1;
        return route.fulfill({
          status,
          json: {
            error:
              status === 401 ? "not authenticated" : status === 403 ? "forbidden" : "not found",
          },
        });
      }
      firstReads += 1;
      return route.fulfill({
        json: firstReads === 1 ? { ...ACTIVITY, next_cursor: "older-page" } : ACTIVITY,
      });
    });
    await page.goto("/activity");
    await expect(page.locator(".activity-list > li")).toHaveCount(3);
    await page.getByRole("button", { name: "載入更多活動" }).click();
    if (status === 401) {
      await expect(
        page.getByRole("status").filter({ hasText: "Workspace 活動需要登入" }),
      ).toBeVisible();
      await expect(page.getByRole("button", { name: "再試一次" })).toHaveCount(0);
    } else {
      await expect(page.getByRole("alert")).toContainText("暫時無法讀取Workspace 活動");
    }
    await expect(page.locator(".activity-list, .activity-summary")).toHaveCount(0);
    await expect(page.getByText("以下僅保留已載入的紀錄", { exact: false })).toHaveCount(0);
    if (status !== 401) {
      await page.getByRole("button", { name: "再試一次" }).click();
      await expect(page.locator(".activity-list > li")).toHaveCount(3);
      expect(firstReads).toBe(2);
      expect(nextReads).toBe(1);
      await expect(page.locator("#activity-heading")).toBeFocused();
    }
  });
}

const HOME_CREATION = {
  id: "88888888-8888-4888-8888-888888888888",
  revision: 1,
  state: "waiting_input",
  snapshot: {
    messages: [],
    brief: "尚待確認的創作",
    brief_confirmed: false,
    acceptance_criteria: [],
    diagram_understanding: "",
    diagram_confirmed: false,
    references: [],
    pending_action: "",
    budget_credits: 1300,
    reserved_credits: 0,
    usage_unknown: false,
    steps: 1,
    tool_calls: 0,
  },
  created_at: "2026-10-14T00:00:00Z",
  updated_at: "2026-10-14T00:00:00Z",
  expires_at: "2099-01-01T00:00:00Z",
  deadline: "2099-01-01T00:00:00Z",
};

for (const empty of [false, true]) {
  test(`activity withdraws a cached ${empty ? "empty" : "populated"} result after a failed reread`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    await page.route("**/me/activity?*", (route) => {
      reads += 1;
      return reads === 2
        ? route.fulfill({
            status: 503,
            json: {
              complete: false,
              unavailable_sources: ["evaluation", "publishing"],
              error: "service unavailable",
            },
          })
        : route.fulfill({ json: empty && reads === 1 ? { ...ACTIVITY, items: [] } : ACTIVITY });
    });
    await page.goto("/activity");
    await expect(page.getByRole("definition").filter({ hasText: "5 / 5" })).toBeVisible();
    const refresh = page.getByRole("button", { name: "重新整理", exact: true });
    await refresh.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("alert")).toContainText("Evaluation、Publishing");
    await expect(page.locator(".activity-summary")).toHaveCount(0);
    await expect(page.locator(".activity-list")).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "目前沒有活動", exact: true })).toHaveCount(0);
    await expect(page.locator("#activity-heading")).toBeFocused();
    await page.getByRole("button", { name: "再試一次" }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator(".activity-list > li")).toHaveCount(3);
    await expect(page.getByRole("alert")).toHaveCount(0);
    expect(reads).toBe(3);
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath(`activity-${empty}-recovered-phone.png`),
      fullPage: true,
    });
  });
}

test("an activity next-page failure retains the loaded page and retries its cursor", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let firstReads = 0;
  let nextReads = 0;
  await page.route("**/me/activity?*", (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor");
    if (!cursor) {
      firstReads += 1;
      return route.fulfill({ json: { ...ACTIVITY, next_cursor: "older-page" } });
    }
    expect(cursor).toBe("older-page");
    nextReads += 1;
    return nextReads === 1
      ? route.fulfill({
          status: 503,
          json: {
            complete: false,
            unavailable_sources: ["evaluation", "publishing"],
            error: "service unavailable",
          },
        })
      : route.fulfill({
          json: {
            ...ACTIVITY,
            items: [
              {
                ...ACTIVITY.items[0],
                source_id: RUN,
                summary: "較早的試跑",
                continuation: { kind: "run", run_id: RUN },
              },
            ],
          },
        });
  });
  await page.goto("/activity");
  await expect(page.locator(".activity-list > li")).toHaveCount(3);
  await page.getByRole("button", { name: "載入更多活動" }).click();
  await expect(page.getByRole("alert")).toContainText("僅保留已載入的紀錄，尚未讀完");
  await expect(page.getByRole("alert")).toContainText("Evaluation、Publishing");
  await expect(page.locator(".activity-list > li")).toHaveCount(3);
  await expect(page.locator(".activity-summary")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "載入更多活動" })).toHaveCount(0);
  await page.getByRole("button", { name: "再試一次" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.locator(".activity-list > li")).toHaveCount(4);
  await expect(page.getByText("已載入全部活動", { exact: true })).toBeVisible();
  expect(firstReads).toBe(1);
  expect(nextReads).toBe(2);
  await expect(page.locator("#activity-heading")).toBeFocused();
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("activity-pagination-recovered-phone.png"),
    fullPage: true,
  });
});

for (const empty of [false, true]) {
  test(`workspace home hides cached ${empty ? "empty" : "populated"} sections and retries each owner independently`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    await page.route("**/me", (route) =>
      route.fulfill({
        json: {
          ...(platformResponse(route.request().url()).body as object),
          features: { generate_skill: true, creation_skill: true },
        },
      }),
    );

    const owners = [
      {
        pattern: "**/runs?*",
        title: "需要留意",
        error: "需要處理的試跑",
        heading: "workspace-attention-heading",
        initial: empty
          ? { runs: [] }
          : { runs: [RUNS.runs[1], { ...RUNS.runs[0], status: "running" }] },
        recovered: { runs: [RUNS.runs[1]] },
      },
      {
        pattern: "**/skills",
        title: "你的資產",
        error: "你的資產",
        heading: "workspace-assets-heading",
        initial: empty
          ? { skills: [], total: 0, limit: 100, truncated: false }
          : platformResponse("/skills").body,
        recovered: platformResponse("/skills").body,
      },
      {
        pattern: "**/me/publications",
        title: "發佈成果",
        error: "發佈成果",
        heading: "workspace-publications-heading",
        initial: empty ? { publications: [] } : OWN_PUBLICATIONS,
        recovered: OWN_PUBLICATIONS,
      },
      {
        pattern: "**/creation-sessions",
        title: "繼續進行",
        error: "最近的創作",
        heading: "workspace-creations-heading",
        initial: empty ? [] : [HOME_CREATION],
        recovered: [HOME_CREATION],
      },
    ];
    let fail = false;
    const reads = owners.map(() => 0);
    const retried = owners.map(() => false);
    for (const [index, owner] of owners.entries()) {
      await page.route(owner.pattern, (route) => {
        reads[index] += 1;
        return fail && !retried[index]
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fulfill({ json: retried[index] ? owner.recovered : owner.initial });
      });
    }
    await page.goto("/workspace");
    await expect(page.getByRole("heading", { name: "繼續推進你的工作" })).toBeVisible();
    await expect.poll(() => reads.every((count) => count === 1)).toBe(true);
    await page.getByRole("link", { name: "資料保存政策", exact: true }).click();
    await expect(page).toHaveURL(/\/policy$/);
    fail = true;
    await page
      .getByRole("navigation", { name: "主要導覽" })
      .getByRole("link", { name: "工作台", exact: true })
      .click();
    await expect(page.getByRole("alert")).toHaveCount(4);
    await expect(page.getByText("目前無法確認哪些試跑仍在執行", { exact: false })).toBeVisible();
    await expect(page.locator(".workspace-home-list, .workspace-home-assets")).toHaveCount(0);
    for (const text of [
      "目前沒有需要你留意的試跑。",
      "目前沒有正在執行的試跑。",
      "目前沒有發佈成果。",
      "資產庫目前是空的。",
    ]) {
      await expect(page.getByText(text, { exact: false })).toHaveCount(0);
    }
    await page.screenshot({
      path: testInfo.outputPath(`workspace-${empty}-failed-phone.png`),
      fullPage: true,
    });
    for (const [index, owner] of owners.entries()) {
      const section = page
        .locator(".workspace-home-section")
        .filter({ has: page.getByRole("heading", { name: owner.title, exact: true }) });
      await expect(section.getByRole("alert")).toContainText(`暫時無法讀取${owner.error}`);
      retried[index] = true;
      const before = [...reads];
      await section.getByRole("button", { name: "再試一次" }).focus();
      await page.keyboard.press("Enter");
      await expect(section.getByRole("alert")).toHaveCount(0);
      expect(reads.map((count, i) => count - before[i])).toEqual(
        owners.map((_, i) => (i === index ? 1 : 0)),
      );
      await expect(page.locator(`#${owner.heading}`)).toBeFocused();
    }
    await expect(page.getByText("尚待確認的創作", { exact: true })).toBeVisible();
    expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath(`workspace-${empty}-recovered-phone.png`),
      fullPage: true,
    });
  });
}

test("publishing reread failure hides cached releases and retries without losing its continuation", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/me/publications", (route) => {
    reads += 1;
    return reads === 2
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: OWN_PUBLICATIONS });
  });
  await page.goto(`/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`);
  const section = page.locator("#skill-publications");
  await expect(section.getByText("續接位置", { exact: true })).toBeVisible();
  await section.getByRole("button", { name: "重新整理", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(section.getByRole("alert")).toContainText("暫時無法讀取小工具發佈清單");
  await expect(section.locator(".download-item")).toHaveCount(0);
  await expect(section.getByText("還沒有任何小工具 Publication。", { exact: false })).toHaveCount(
    0,
  );
  await expect(page.locator("#publication-overview-title")).toBeFocused();
  await section.getByRole("button", { name: "再試一次" }).focus();
  await page.keyboard.press("Enter");
  await expect(section.getByText("續接位置", { exact: true })).toBeVisible();
  await expect(page.locator("#publication-overview-title")).toBeFocused();
  expect(new URL(page.url()).searchParams.get("publication")).toBe(`${PUBLISHER}/${PUBLICATION}`);
  expect(reads).toBe(3);
  expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("publishing-recovered-phone.png"),
    fullPage: true,
  });
});
