import { expect, test } from "vitest";
import { RUNS, SKILL, TEST_CASE, VERSION, platformResponse } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

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
    test(`${list.name} withdraw cached ${empty ? "empty" : "populated"} results and recover in place`, async () => {
      const page = app();
      let reads = 0;
      page.route(list.pattern, () => {
        reads += 1;
        return reads === 2
          ? { status: 503, json: { error: "service unavailable" } }
          : {
              json: {
                [list.key]: empty && reads === 1 ? [] : list.rows,
                limit: list.pageSize,
                total: empty && reads === 1 ? 0 : list.rows.length,
                truncated: false,
              },
            };
      });
      await page.goto(list.url);
      if (empty)
        await page.eventually(() => expect(page.getByText(list.empty).count()).toBeGreaterThan(0));
      else
        await page.eventually(() =>
          expect(page.locator(list.selector).count()).toBe(list.rows.length),
        );
      await page.locator('a[href="/policy"]').first().click();
      await page.eventually(() => expect(page.pathname()).toBe("/policy"));
      await page.eventually(() =>
        expect(page.getByRole("heading", { name: "資料保存政策", exact: true }).count()).toBe(1),
      );
      await page.goBack();
      await page.eventually(() => expect(page.getByRole("alert").text()).toContain("暫時無法讀取"));
      expect(page.locator(list.selector).count()).toBe(0);
      expect(page.getByText(list.empty).count()).toBe(0);
      if (list.name === "history") {
        expect(page.getByText("沿用最近一次試跑紀錄", { exact: true }).count()).toBe(0);
        expect(page.getByText("最近使用的 Version 無法確認", { exact: true }).count()).toBe(1);
      }
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      expect(retry.count()).toBe(1);
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() =>
        expect(page.locator(list.selector).count()).toBe(list.rows.length),
      );
      await page.eventually(() => expect(page.locator(`#${list.heading}`).isFocused()).toBe(true));
      expect(reads).toBe(3);
      expect(await page.axeViolations("main")).toEqual([]);
    });
  }

  for (const status of [401, 403, 404, 503]) {
    test(`${list.name} pagination ${status} uses the appropriate recovery and coverage`, async () => {
      const page = app();
      let firstReads = 0;
      let nextReads = 0;
      const pagedRows = Array.from({ length: list.pageSize + 1 }, (_, index) => ({
        ...list.rows[0],
        [list.key === "runs" ? "run_id" : list.key === "skills" ? "skill_id" : "test_case_id"]:
          `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
      }));
      page.route(list.pattern, ({ url }) => {
        const offset = Number(url.searchParams.get("offset"));
        if (offset === 0) {
          firstReads += 1;
          return {
            json: {
              [list.key]:
                firstReads === 1 ? pagedRows.slice(0, list.name === "assets" ? 24 : 51) : list.rows,
              limit: list.pageSize,
              total: firstReads === 1 ? pagedRows.length : list.rows.length,
              truncated: firstReads === 1,
            },
          };
        }
        expect(offset).toBe(list.pageSize);
        nextReads += 1;
        return nextReads === 1
          ? { status, json: { error: status === 401 ? "not authenticated" : "read failed" } }
          : {
              json: {
                [list.key]: [pagedRows[list.pageSize]],
                limit: list.pageSize,
                total: pagedRows.length,
                truncated: false,
              },
            };
      });
      await page.goto(list.url);
      await page.eventually(() => expect(page.locator(list.selector).count()).toBe(list.pageSize));
      if (list.name === "test cases") {
        await page.getByLabel("名稱", { exact: true }).fill("未送出的測試題");
        await page.getByLabel("User Prompt", { exact: true }).fill("保留草稿內容");
      }
      await page.getByRole("button", { name: list.loadMore, exact: true }).click();
      if (status === 401) {
        await page.eventually(() =>
          expect(page.getByRole("status").filter({ hasText: "需要登入" }).count()).toBeGreaterThan(
            0,
          ),
        );
        expect(page.locator(list.selector).count()).toBe(0);
        expect(page.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(0);
        return;
      }
      await page.eventually(() => expect(page.getByRole("alert").count()).toBeGreaterThan(0));
      expect(page.locator(list.selector).count()).toBe(status === 503 ? list.pageSize : 0);
      if (status === 503) {
        expect(page.getByRole("alert").text()).toContain("尚未讀完");
        expect(page.getByText("目前沒有正在執行的試跑。", { exact: true }).count()).toBe(0);
      }
      expect(page.getByRole("button", { name: list.loadMore, exact: true }).count()).toBe(0);
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await page.eventually(() =>
        expect(page.locator(list.selector).count()).toBe(
          status === 503 ? list.pageSize + 1 : list.rows.length,
        ),
      );
      expect(firstReads).toBe(status === 503 ? 1 : 2);
      expect(nextReads).toBe(status === 503 ? 2 : 1);
      await page.eventually(() => expect(page.locator(`#${list.heading}`).isFocused()).toBe(true));
      expect(page.getByRole("button", { name: list.loadMore, exact: true }).count()).toBe(0);
      if (list.name === "test cases") {
        expect(page.getByLabel("名稱", { exact: true }).value()).toBe("未送出的測試題");
        expect(page.getByLabel("User Prompt", { exact: true }).value()).toBe("保留草稿內容");
      }
    });
  }
}

for (const cached of [false, true]) {
  test(`test-case skill read failure ${cached ? "after a cached absence" : "on arrival"} preserves the draft and avoids an ownership conclusion`, async () => {
    const page = app();
    let reads = 0;
    let writes = 0;
    page.route("**/skills?*", () => {
      reads += 1;
      return cached && reads === 1
        ? { json: { skills: [], limit: 24, total: 0, truncated: false } }
        : reads === (cached ? 2 : 1)
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: skills };
    });
    page.route("**/test-cases", () => {
      writes += 1;
      return { status: 400, json: { error: "不應送出" } };
    });
    await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
    if (cached) {
      await page.eventually(() =>
        expect(page.getByText("這個小工具不在你的工作區。").count()).toBeGreaterThan(0),
      );
      await page.locator('a[href="/policy"]').first().click();
      await page.eventually(() => expect(page.pathname()).toBe("/policy"));
      await page.eventually(() =>
        expect(page.getByRole("heading", { name: "資料保存政策", exact: true }).count()).toBe(1),
      );
      await page.goBack();
    }
    await page.eventually(() =>
      expect(page.getByRole("alert").text()).toContain("暫時無法讀取你的小工具清單"),
    );
    expect(page.getByText("這個小工具不在你的工作區。").count()).toBe(0);
    expect(page.locator("#create-skills-why").text()).toContain("目前無法確認可用的小工具");
    await page.getByLabel("名稱", { exact: true }).fill("未送出的測試題");
    await page.getByLabel("User Prompt", { exact: true }).fill("保留草稿內容");
    expect(page.getByRole("button", { name: "建立", exact: true }).isDisabled()).toBe(true);
    await page.locator("#tc-name").press("Enter");
    expect(writes).toBe(0);
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await page.eventually(() => expect(page.getByRole("alert").count()).toBe(0));
    expect(page.getByLabel("名稱", { exact: true }).value()).toBe("未送出的測試題");
    expect(page.getByLabel("User Prompt", { exact: true }).value()).toBe("保留草稿內容");
    expect(page.getByLabel("小工具", { exact: true }).value()).toBe(SKILL);
    await page.eventually(() =>
      expect(page.getByRole("button", { name: "建立", exact: true }).isDisabled()).toBe(false),
    );
    await page.eventually(() =>
      expect(page.locator("#create-test-case-heading").isFocused()).toBe(true),
    );
    expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
    expect(writes).toBe(0);
  });
}
