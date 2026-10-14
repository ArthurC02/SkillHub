import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import {
  RUNS,
  SKILL,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const skills = platformResponse("/skills").body as { skills: Array<Record<string, unknown>> };
const cases = platformResponse("/test-cases").body as {
  test_cases: Array<Record<string, unknown>>;
};
const lists = [
  {
    name: "runs",
    url: "/workspace/runs",
    pattern: "**/runs?*",
    key: "runs",
    rows: RUNS.runs,
    selector: "[data-role=evidence] > li",
    heading: "workspace-runs-heading",
    empty: "還沒有做過任何試跑",
    pageSize: 50,
    loadMore: "載入更多",
  },
  {
    name: "assets",
    url: "/library",
    pattern: "**/skills?*",
    key: "skills",
    rows: skills.skills,
    selector: ".skill-grid > li",
    heading: "workspace-skills-heading",
    empty: "資產庫還沒有任何小工具",
    pageSize: 24,
    loadMore: "載入更多",
  },
  {
    name: "test cases",
    url: "/lab/test-cases",
    pattern: "**/test-cases?*",
    key: "test_cases",
    rows: cases.test_cases,
    selector: ".test-case-index [data-role=evidence] > li",
    heading: "existing-test-cases-heading",
    empty: "還沒有測試題。",
    pageSize: 50,
    loadMore: "載入更多",
  },
  {
    name: "history",
    url: `/lab/test-cases/${TEST_CASE}`,
    pattern: "**/runs?*",
    key: "runs",
    rows: RUNS.runs,
    selector: ".test-case-history-panel [data-role=evidence] > li",
    heading: "test-case-history-heading",
    empty: "尚無執行。",
    pageSize: 50,
    loadMore: "載入更多",
  },
  {
    name: "version evidence",
    url: `/skills/${SKILL}/versions/${VERSION}`,
    pattern: "**/runs?*",
    key: "runs",
    rows: RUNS.runs,
    selector: "section[aria-labelledby=version-evidence-title] [data-role=evidence] > li",
    heading: "version-evidence-title",
    empty: "這個版本還沒有試跑紀錄。",
    pageSize: 50,
    loadMore: "載入更多證據",
  },
];

for (const list of lists) {
  for (const empty of [false, true]) {
    test(`${list.name} withdraw cached ${empty ? "empty" : "populated"} results and recover in place`, async ({
      page,
    }, testInfo) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let reads = 0;
      await page.route(list.pattern, (route) => {
        reads += 1;
        return reads === 2
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fulfill({
              json: {
                [list.key]: empty && reads === 1 ? [] : list.rows,
                total: empty && reads === 1 ? 0 : list.rows.length,
                truncated: false,
              },
            });
      });
      await page.goto(list.url);
      if (empty) await expect(page.getByText(list.empty, { exact: false })).toBeVisible();
      else await expect(page.locator(list.selector)).toHaveCount(list.rows.length);
      await page.locator('a[href="/policy"]').first().click();
      await expect(page).toHaveURL(/\/policy$/);
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
      await page.goBack();
      await expect(page.getByRole("alert")).toContainText("暫時無法讀取");
      await expect(page.locator(list.selector)).toHaveCount(0);
      await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
      if (list.name === "history") {
        await expect(page.getByText("沿用最近一次試跑紀錄", { exact: true })).toHaveCount(0);
        await expect(page.getByText("最近使用的 Version 無法確認", { exact: true })).toBeVisible();
      }
      await expect(page.getByRole("button", { name: "再試一次", exact: true })).toBeVisible();
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(page.locator(list.selector)).toHaveCount(list.rows.length);
      await expect(page.locator(`#${list.heading}`)).toBeFocused();
      expect(reads).toBe(3);
      expect((await new AxeBuilder({ page }).include("main").analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.evaluate(() => scrollTo(0, 0));
      await page.screenshot({
        path: testInfo.outputPath(`${list.name}-recovered.png`),
        fullPage: true,
      });
    });
  }

  for (const status of [401, 403, 404, 503]) {
    test(`${list.name} pagination ${status} uses the appropriate recovery and coverage`, async ({
      page,
    }) => {
      await stubPlatform(page);
      let firstReads = 0;
      let nextReads = 0;
      const pagedRows = Array.from({ length: list.pageSize + 1 }, (_, index) => ({
        ...list.rows[0],
        [list.key === "runs" ? "run_id" : list.key === "skills" ? "skill_id" : "test_case_id"]:
          `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
      }));
      await page.route(list.pattern, (route) => {
        const offset = Number(new URL(route.request().url()).searchParams.get("offset"));
        if (offset === 0) {
          firstReads += 1;
          return route.fulfill({
            json: {
              [list.key]:
                firstReads === 1 ? pagedRows.slice(0, list.name === "assets" ? 24 : 51) : list.rows,
              total: pagedRows.length,
              truncated: firstReads === 1,
            },
          });
        }
        expect(offset).toBe(list.pageSize);
        nextReads += 1;
        return nextReads === 1
          ? route.fulfill({
              status,
              json: { error: status === 401 ? "not authenticated" : "read failed" },
            })
          : route.fulfill({
              json: {
                [list.key]: [pagedRows[list.pageSize]],
                total: pagedRows.length,
                truncated: false,
              },
            });
      });
      await page.goto(list.url);
      await expect(page.locator(list.selector)).toHaveCount(list.pageSize);
      if (list.name === "test cases") {
        await page.getByLabel("名稱", { exact: true }).fill("未送出的測試題");
        await page.getByLabel("User Prompt", { exact: true }).fill("保留草稿內容");
      }
      await page.getByRole("button", { name: list.loadMore, exact: true }).click();
      if (status === 401) {
        await expect(page.getByRole("status").filter({ hasText: "需要登入" })).toBeVisible();
        await expect(page.locator(list.selector)).toHaveCount(0);
        await expect(page.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
      } else {
        await expect(page.getByRole("alert")).toBeVisible();
        await expect(page.locator(list.selector)).toHaveCount(status === 503 ? list.pageSize : 0);
        if (status === 503) {
          await expect(page.getByRole("alert")).toContainText("尚未讀完");
          await expect(page.getByText("目前沒有正在執行的試跑。", { exact: true })).toHaveCount(0);
        }
        await expect(page.getByRole("button", { name: list.loadMore, exact: true })).toHaveCount(0);
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect(page.locator(list.selector)).toHaveCount(
          status === 503 ? list.pageSize + 1 : list.rows.length,
        );
        expect(firstReads).toBe(status === 503 ? 1 : 2);
        expect(nextReads).toBe(status === 503 ? 2 : 1);
        await expect(page.locator(`#${list.heading}`)).toBeFocused();
      }
      await expect(page.getByRole("button", { name: list.loadMore, exact: true })).toHaveCount(0);
      if (list.name === "test cases") {
        await expect(page.getByLabel("名稱", { exact: true })).toHaveValue("未送出的測試題");
        await expect(page.getByLabel("User Prompt", { exact: true })).toHaveValue("保留草稿內容");
      }
    });
  }
}

for (const cached of [false, true]) {
  test(`test-case skill read failure ${cached ? "after a cached absence" : "on arrival"} preserves the draft and avoids an ownership conclusion`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let reads = 0;
    let writes = 0;
    await page.route("**/skills", (route) => {
      reads += 1;
      return cached && reads === 1
        ? route.fulfill({ json: { skills: [] } })
        : reads === (cached ? 2 : 1)
          ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
          : route.fulfill({ json: skills });
    });
    await page.route("**/test-cases", (route) => {
      writes += 1;
      return route.fulfill({ status: 400, json: { error: "不應送出" } });
    });
    await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
    if (cached) {
      await expect(page.getByText("這個小工具不在你的工作區。", { exact: false })).toBeVisible();
      await page.locator('a[href="/policy"]').first().click();
      await expect(page).toHaveURL(/\/policy$/);
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
      await page.goBack();
    }
    await expect(page.getByRole("alert")).toContainText("暫時無法讀取你的小工具清單");
    await expect(page.getByText("這個小工具不在你的工作區。", { exact: false })).toHaveCount(0);
    await expect(page.locator("#create-skills-why")).toContainText("目前無法確認可用的小工具");
    await page.getByLabel("名稱", { exact: true }).fill("未送出的測試題");
    await page.getByLabel("User Prompt", { exact: true }).fill("保留草稿內容");
    await expect(page.getByRole("button", { name: "建立", exact: true })).toBeDisabled();
    await page.locator("#tc-name").press("Enter");
    expect(writes).toBe(0);
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(page.getByLabel("名稱", { exact: true })).toHaveValue("未送出的測試題");
    await expect(page.getByLabel("User Prompt", { exact: true })).toHaveValue("保留草稿內容");
    await expect(page.getByLabel("小工具", { exact: true })).toHaveValue(SKILL);
    await expect(page.getByRole("button", { name: "建立", exact: true })).toBeEnabled();
    await expect(page.locator("#create-test-case-heading")).toBeFocused();
    expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
    expect(writes).toBe(0);
  });
}
