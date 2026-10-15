import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import {
  platformResponse,
  PUBLISHER,
  PUBLICATION,
  OWN_PUBLICATIONS,
  SKILL,
  VERSION,
  TEST_CASE,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const lists = [
  {
    name: "Library",
    url: "/library",
    path: "/skills",
    pattern: "**/skills?*",
    key: "skills",
    empty: "資產庫還沒有任何小工具",
    what: "資產庫內容",
    selector: ".skill-grid > li",
    heading: "workspace-skills-heading",
    scope: "main",
    refresh: true,
  },
  {
    name: "publications",
    url: "/workspace/downloads",
    path: "/me/publications",
    pattern: "**/me/publications",
    key: "publications",
    empty: "還沒有任何小工具 Publication",
    what: "小工具發佈清單",
    selector: "#skill-publications .download-list > li",
    heading: "publication-overview-title",
    scope: "#skill-publications",
    refresh: true,
  },
  {
    name: "Catalog",
    url: "/",
    path: "/api/skills/catalog?limit=100",
    pattern: "**/api/skills/catalog?*",
    key: "results",
    empty: "目錄裡還沒有任何東西",
    what: "目錄",
    selector: ".catalog-skill-card",
    heading: "catalog-search-heading",
    scope: "main",
    refresh: false,
  },
  {
    name: "runs",
    url: "/workspace/runs",
    path: "/runs",
    pattern: "**/runs?*",
    key: "runs",
    empty: "還沒有做過任何試跑",
    what: "試跑活動",
    selector: "main [data-role=evidence] > li",
    heading: "workspace-runs-heading",
    scope: "main",
    refresh: false,
  },
  {
    name: "test cases",
    url: "/lab/test-cases",
    path: "/test-cases",
    pattern: "**/test-cases?*",
    key: "test_cases",
    empty: "還沒有測試題。",
    what: "測試題",
    selector: ".test-case-index [data-role=evidence] > li",
    heading: "existing-test-cases-heading",
    scope: ".test-case-index",
    refresh: true,
  },
  {
    name: "history",
    url: `/lab/test-cases/${TEST_CASE}`,
    path: "/runs",
    pattern: "**/runs?*",
    key: "runs",
    empty: "尚無執行。",
    what: "執行歷史",
    selector: ".test-case-history-panel [data-role=evidence] > li",
    heading: "test-case-history-heading",
    scope: ".test-case-history-panel",
    refresh: false,
  },
  {
    name: "version evidence",
    url: `/skills/${SKILL}/versions/${VERSION}`,
    path: "/runs",
    pattern: "**/runs?*",
    key: "runs",
    empty: "這個版本還沒有試跑紀錄。",
    what: "這個版本的試跑證據",
    selector: 'section[aria-labelledby="version-evidence-title"] [data-role=evidence] > li',
    heading: "version-evidence-title",
    scope: 'section[aria-labelledby="version-evidence-title"]',
    refresh: false,
  },
];

async function verifyLayout(page: Page, state: string) {
  const result = await new AxeBuilder({ page }).analyze();
  const path = test.info().outputPath(`${state}-accessibility.json`);
  await writeFile(path, JSON.stringify(result));
  await test.info().attach(`${state}-accessibility`, { path, contentType: "application/json" });
  expect(result.violations).toEqual([]);
  for (const review of result.incomplete) {
    expect(review.id).toBe("color-contrast");
    for (const node of review.nodes) {
      expect(node.html).toContain('aria-hidden="true"');
      expect(node.target).toHaveLength(1);
      expect(node.target[0]).toMatch(
        /(?:\.door-mono|\.skill-mono|\.skill-menu-caret|\.skill-card-validation > span)/,
      );
      const message = node.html.includes('class="skill-mono"') ? "shortTextContent" : "nonBmp";
      expect(node.any.every((check) => check.data?.messageKey === message)).toBe(true);
    }
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath(`${state}.png`), fullPage: true });
}

for (const list of lists) {
  for (const stillEmpty of [false, true]) {
    test(`${list.name} confirms a cached empty list through a failed reread and offline recovery to ${stillEmpty ? "empty" : "populated"}`, async ({
      page,
      context,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      let reads = 0;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const original = platformResponse(list.path).body as Record<string, unknown>;
      const rows = original[list.key] as unknown[];
      expect(rows.length).toBeGreaterThan(0);
      await page.route(list.pattern, async (route) => {
        const countRead =
          list.name !== "Catalog" ||
          new URL(route.request().url()).searchParams.get("limit") === "100";
        if (countRead) reads += 1;
        if (countRead && reads === 2) {
          await gate;
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        }
        const present = reads > 2 && !stillEmpty;
        return route.fulfill({
          json: {
            ...original,
            [list.key]: present ? [rows[0]] : [],
            total: present ? 1 : 0,
            truncated: false,
            limit: 24,
          },
        });
      });
      try {
        await page.goto(list.url);
        await expect(page.getByText(list.empty, { exact: false })).toBeVisible();
        await page.locator('a[href="/policy"]').first().click();
        await expect(
          page.getByRole("heading", { name: "資料保存政策", exact: true }),
        ).toBeVisible();
        await page.goBack();
        await expect.poll(() => reads).toBe(2);
        const scope = page.locator(list.scope);
        await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
        await expect(
          scope.getByRole("status").filter({ hasText: `載入${list.what}中` }),
        ).toBeVisible();
        if (list.refresh)
          await expect(
            scope.getByRole("button", { name: "重新整理中…", exact: true }),
          ).toBeDisabled();
        await verifyLayout(page, "confirming");
        release();
        await expect(
          scope.getByRole("alert").filter({ hasText: `暫時無法讀取${list.what}` }),
        ).toBeVisible();
        await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
        await context.setOffline(true);
        await scope.getByRole("button", { name: "再試一次", exact: true }).focus();
        await page.keyboard.press("Enter");
        await expect(
          scope.getByRole("status").filter({ hasText: `等待連線以讀取${list.what}` }),
        ).toBeVisible();
        await expect(
          scope.getByRole("button", { name: "重新讀取中…", exact: true }),
        ).toBeDisabled();
        await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
        expect(reads).toBe(2);
        await verifyLayout(page, "paused");
        await context.setOffline(false);
        if (stillEmpty)
          await expect(scope.getByRole("status").filter({ hasText: list.empty })).toBeVisible();
        else await expect(page.locator(list.selector)).toHaveCount(1);
        await expect(scope.getByRole("alert")).toHaveCount(0);
        await expect(scope.locator("[data-loading]")).toHaveCount(0);
        expect(reads).toBe(3);
        await verifyLayout(page, "confirmed");
      } finally {
        release();
        await context.setOffline(false);
      }
    });
  }
}

for (const list of lists) {
  test(`${list.name} waits for connectivity before reconfirming its cached empty snapshot`, async ({
    page,
    context,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    await page.route(list.pattern, (route) => {
      if (
        list.name !== "Catalog" ||
        new URL(route.request().url()).searchParams.get("limit") === "100"
      )
        reads += 1;
      return route.fulfill({ json: { [list.key]: [], total: 0, truncated: false, limit: 24 } });
    });
    try {
      await page.goto(list.url);
      await expect(page.getByText(list.empty, { exact: false })).toBeVisible();
      await page.locator('a[href="/policy"]').first().click();
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
      await context.setOffline(true);
      await page.goBack();
      const scope = page.locator(list.scope);
      const waiting = scope.getByRole("status").filter({ hasText: `等待連線以讀取${list.what}` });
      await expect(waiting).toBeVisible();
      await expect(waiting.locator(".loading-skeleton")).toHaveCount(0);
      await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
      if (list.refresh) {
        const refresh = scope.getByRole("button", { name: "等待連線…", exact: true });
        await expect(refresh).toBeDisabled();
        await refresh.evaluate((element) => (element as HTMLButtonElement).click());
      }
      expect(reads).toBe(1);
      await verifyLayout(page, "cached-paused");
      await context.setOffline(false);
      await expect(scope.getByRole("status").filter({ hasText: list.empty })).toBeVisible();
      await expect(waiting).toHaveCount(0);
      expect(reads).toBe(2);
      await verifyLayout(page, "cached-confirmed");
    } finally {
      await context.setOffline(false);
    }
  });

  test(`${list.name} resumes an initially missing read after an offline retry without claiming emptiness`, async ({
    page,
    context,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    await page.route(list.pattern, (route) => {
      const countRead =
        list.name !== "Catalog" ||
        new URL(route.request().url()).searchParams.get("limit") === "100";
      if (countRead) reads += 1;
      if (countRead && reads === 1)
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      return route.fulfill({ json: { [list.key]: [], total: 0, truncated: false, limit: 24 } });
    });
    try {
      await page.goto(list.url);
      const scope = page.locator(list.scope);
      await expect(
        scope.getByRole("alert").filter({ hasText: `暫時無法讀取${list.what}` }),
      ).toBeVisible();
      await context.setOffline(true);
      await scope.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      const waiting = scope.getByRole("status").filter({ hasText: `等待連線以讀取${list.what}` });
      await expect(waiting).toBeVisible();
      await expect(waiting.locator(".loading-skeleton")).toHaveCount(0);
      await expect(page.locator(`#${list.heading}`)).toBeFocused();
      await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
      expect(reads).toBe(1);
      await verifyLayout(page, "initial-paused");
      await context.setOffline(false);
      await expect(scope.getByRole("status").filter({ hasText: list.empty })).toBeVisible();
      await expect(scope.getByRole("alert")).toHaveCount(0);
      expect(reads).toBe(2);
      await verifyLayout(page, "initial-confirmed");
    } finally {
      await context.setOffline(false);
    }
  });
}

test("a filtered Catalog confirms matching emptiness without declaring the deployment empty", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/skills/catalog?*", async (route) => {
    if (new URL(route.request().url()).searchParams.get("limit") === "100") {
      reads += 1;
      if (reads === 2) await gate;
    }
    return route.fulfill({ json: { results: [], total: 0, truncated: false } });
  });
  try {
    await page.goto("/?category=documents");
    const empty = page.getByRole("status").filter({ hasText: "沒有小工具符合目前的篩選條件" });
    await expect(empty).toBeVisible();
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    await page.goBack();
    await expect.poll(() => reads).toBe(2);
    await expect(empty).toHaveCount(0);
    await expect(page.getByRole("status").filter({ hasText: "載入目錄中" })).toBeVisible();
    await expect(page.getByText("目錄裡還沒有任何東西", { exact: false })).toHaveCount(0);
    release();
    await expect(empty).toBeVisible();
    expect(new URL(page.url()).searchParams.get("category")).toBe("documents");
    expect(reads).toBe(2);
    await verifyLayout(page, "filtered-confirmed");
  } finally {
    release();
  }
});

test("a selected publication is not declared missing while its empty snapshot is being confirmed", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/me/publications", async (route) => {
    reads += 1;
    if (reads === 1) return route.fulfill({ json: { publications: [] } });
    await gate;
    return route.fulfill({ json: OWN_PUBLICATIONS });
  });
  try {
    await page.goto(`/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`);
    const missing = page
      .getByRole("status")
      .filter({ hasText: "這個工作區目前找不到這筆小工具發佈" });
    await expect(missing).toBeVisible();
    await page.locator('a[href="/policy"]').first().click();
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    await page.goBack();
    await expect.poll(() => reads).toBe(2);
    await expect(missing).toHaveCount(0);
    await expect(
      page.getByRole("status").filter({ hasText: "載入小工具發佈清單中" }),
    ).toBeVisible();
    release();
    await expect(page.locator('#skill-publications [aria-current="location"]')).toBeFocused();
    await expect(missing).toHaveCount(0);
    expect(new URL(page.url()).searchParams.get("publication")).toBe(`${PUBLISHER}/${PUBLICATION}`);
    expect(reads).toBe(2);
    await verifyLayout(page, "selected-confirmed");
  } finally {
    release();
  }
});
