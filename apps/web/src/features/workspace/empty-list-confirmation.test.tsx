import { act } from "react";
import { expect, test } from "vitest";
import {
  platformResponse,
  PUBLISHER,
  PUBLICATION,
  OWN_PUBLICATIONS,
  SKILL,
  VERSION,
  TEST_CASE,
  ARTIFACT,
  ARTIFACT_ROW,
} from "../../testing/fixtures/platform";
import { renderedApp, type App, type Reply, type Request } from "../../testing/app";

const app = renderedApp();

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
  {
    name: "downloads",
    url: "/workspace/downloads",
    path: "/downloads",
    pattern: "**/downloads",
    key: "downloads",
    empty: "目前沒有可顯示的套件",
    what: "下載紀錄",
    selector: "#delivery-history .download-list > li",
    heading: "delivery-history-heading",
    scope: "#delivery-history",
    refresh: false,
  },
  {
    name: "workspace graders",
    url: "/lab/graders",
    path: "/evaluation/graders",
    pattern: "**/evaluation/graders",
    key: "graders",
    empty: "還沒有評分器。",
    what: "評分器",
    selector: ".criterion-list > li",
    heading: "grader-list-heading",
    scope: "main",
    refresh: true,
  },
  {
    name: "platform graders",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    pattern: "**/admin/evaluation/graders",
    key: "graders",
    empty: "還沒有評分器。",
    what: "評分器",
    selector: ".criterion-list > li",
    heading: "grader-list-heading",
    scope: "main",
    refresh: true,
  },
];

type List = (typeof lists)[number];

const countsAsRead = (list: List, request: Request) =>
  list.name !== "Catalog" || request.url.searchParams.get("limit") === "100";

async function verifyLayout(page: App) {
  expect(await page.axeViolations("body")).toEqual([]);
}

async function goToPolicyAndBack(page: App, offline = false) {
  await page.locator('a[href="/policy"]').first().click();
  await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
  if (offline) await page.setOffline(true);
  await page.goBack();
}

for (const list of lists) {
  for (const stillEmpty of [false, true]) {
    test(`${list.name} confirms a cached empty list through a failed reread and offline recovery to ${stillEmpty ? "empty" : "populated"}`, async () => {
      const page = app();
      let reads = 0;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const original = platformResponse(list.path).body as Record<string, unknown>;
      const rows = original[list.key] as unknown[];
      expect(rows.length).toBeGreaterThan(0);
      page.route(list.pattern, async (request): Promise<Reply> => {
        const countRead = countsAsRead(list, request);
        if (countRead) reads += 1;
        if (countRead && reads === 2) {
          await gate;
          return { status: 503, json: { error: "service unavailable" } };
        }
        const present = reads > 2 && !stillEmpty;
        return {
          json: {
            ...original,
            [list.key]: present ? [rows[0]] : [],
            total: present ? 1 : 0,
            truncated: false,
            limit: 24,
          },
        };
      });
      try {
        await page.goto(list.url);
        await expect(page.getByText(list.empty, { exact: false })).toBeVisible();
        await goToPolicyAndBack(page);
        await page.eventually(() => expect(reads).toBe(2));
        const scope = page.locator(list.scope);
        await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
        await expect(
          scope.getByRole("status").filter({ hasText: `載入${list.what}中` }),
        ).toBeVisible();
        if (list.refresh)
          await expect(
            scope.getByRole("button", { name: "重新整理中…", exact: true }),
          ).toBeDisabled();
        await verifyLayout(page);
        release();
        await expect(
          scope.getByRole("alert").filter({ hasText: `暫時無法讀取${list.what}` }),
        ).toBeVisible();
        await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
        await page.setOffline(true);
        await scope.getByRole("button", { name: "再試一次", exact: true }).focus();
        await page.press("Enter");
        await expect(
          scope.getByRole("status").filter({ hasText: `等待連線以讀取${list.what}` }),
        ).toBeVisible();
        await expect(
          scope.getByRole("button", { name: "重新讀取中…", exact: true }),
        ).toBeDisabled();
        await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
        expect(reads).toBe(2);
        await verifyLayout(page);
        await page.setOffline(false);
        if (stillEmpty)
          await expect(scope.getByRole("status").filter({ hasText: list.empty })).toBeVisible();
        else await expect(page.locator(list.selector)).toHaveCount(1);
        await expect(scope.getByRole("alert")).toHaveCount(0);
        await expect(scope.locator("[data-loading]")).toHaveCount(0);
        expect(reads).toBe(3);
        await verifyLayout(page);
      } finally {
        release();
        await page.setOffline(false);
      }
    });
  }
}

for (const list of lists) {
  test(`${list.name} waits for connectivity before reconfirming its cached empty snapshot`, async () => {
    const page = app();
    let reads = 0;
    page.route(list.pattern, (request) => {
      if (countsAsRead(list, request)) reads += 1;
      return { json: { [list.key]: [], total: 0, truncated: false, limit: 24 } };
    });
    try {
      await page.goto(list.url);
      await expect(page.getByText(list.empty, { exact: false })).toBeVisible();
      await goToPolicyAndBack(page, true);
      const scope = page.locator(list.scope);
      const waiting = scope.getByRole("status").filter({ hasText: `等待連線以讀取${list.what}` });
      await expect(waiting).toBeVisible();
      await expect(waiting.locator(".loading-skeleton")).toHaveCount(0);
      await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
      if (list.refresh) {
        const refresh = scope.getByRole("button", { name: "等待連線…", exact: true });
        await expect(refresh).toBeDisabled();
        const element = refresh.element() as HTMLButtonElement;
        await act(async () => element.click());
      }
      expect(reads).toBe(1);
      await verifyLayout(page);
      await page.setOffline(false);
      await expect(scope.getByRole("status").filter({ hasText: list.empty })).toBeVisible();
      await expect(waiting).toHaveCount(0);
      expect(reads).toBe(2);
      await verifyLayout(page);
    } finally {
      await page.setOffline(false);
    }
  });

  test(`${list.name} resumes an initially missing read after an offline retry without claiming emptiness`, async () => {
    const page = app();
    let reads = 0;
    page.route(list.pattern, (request): Reply => {
      const countRead = countsAsRead(list, request);
      if (countRead) reads += 1;
      if (countRead && reads === 1) return { status: 503, json: { error: "service unavailable" } };
      return { json: { [list.key]: [], total: 0, truncated: false, limit: 24 } };
    });
    try {
      await page.goto(list.url);
      const scope = page.locator(list.scope);
      await expect(
        scope.getByRole("alert").filter({ hasText: `暫時無法讀取${list.what}` }),
      ).toBeVisible();
      await page.setOffline(true);
      await scope.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      const waiting = scope.getByRole("status").filter({ hasText: `等待連線以讀取${list.what}` });
      await expect(waiting).toBeVisible();
      await expect(waiting.locator(".loading-skeleton")).toHaveCount(0);
      await expect(page.locator(`#${list.heading}`)).toBeFocused();
      await expect(page.getByText(list.empty, { exact: false })).toHaveCount(0);
      expect(reads).toBe(1);
      await verifyLayout(page);
      await page.setOffline(false);
      await expect(scope.getByRole("status").filter({ hasText: list.empty })).toBeVisible();
      await expect(scope.getByRole("alert")).toHaveCount(0);
      expect(reads).toBe(2);
      await verifyLayout(page);
    } finally {
      await page.setOffline(false);
    }
  });
}

test("a filtered Catalog confirms matching emptiness without declaring the deployment empty", async () => {
  const page = app();
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route("**/api/skills/catalog?*", async (request) => {
    if (request.url.searchParams.get("limit") === "100") {
      reads += 1;
      if (reads === 2) await gate;
    }
    return { json: { results: [], total: 0, limit: 100, truncated: false } };
  });
  try {
    await page.goto("/?category=documents");
    const empty = page.getByRole("status").filter({ hasText: "沒有小工具符合目前的篩選條件" });
    await expect(empty).toBeVisible();
    await goToPolicyAndBack(page);
    await page.eventually(() => expect(reads).toBe(2));
    await expect(empty).toHaveCount(0);
    await expect(page.getByRole("status").filter({ hasText: "載入目錄中" })).toBeVisible();
    await expect(page.getByText("目錄裡還沒有任何東西", { exact: false })).toHaveCount(0);
    release();
    await expect(empty).toBeVisible();
    expect(new URLSearchParams(page.search()).get("category")).toBe("documents");
    expect(reads).toBe(2);
    await verifyLayout(page);
  } finally {
    release();
  }
});

test("a selected publication is not declared missing while its empty snapshot is being confirmed", async () => {
  const page = app();
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route("**/me/publications", async () => {
    reads += 1;
    if (reads === 1) return { json: { publications: [] } };
    await gate;
    return { json: OWN_PUBLICATIONS };
  });
  try {
    await page.goto(`/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`);
    const missing = page
      .getByRole("status")
      .filter({ hasText: "這個工作區目前找不到這筆小工具發佈" });
    await expect(missing).toBeVisible();
    await goToPolicyAndBack(page);
    await page.eventually(() => expect(reads).toBe(2));
    await expect(missing).toHaveCount(0);
    await expect(
      page.getByRole("status").filter({ hasText: "載入小工具發佈清單中" }),
    ).toBeVisible();
    release();
    await expect(page.locator('#skill-publications [aria-current="location"]')).toBeFocused();
    await expect(missing).toHaveCount(0);
    expect(new URLSearchParams(page.search()).get("publication")).toBe(
      `${PUBLISHER}/${PUBLICATION}`,
    );
    expect(reads).toBe(2);
    await verifyLayout(page);
  } finally {
    release();
  }
});

test("deleting the last visible package does not claim that no packaging ever occurred", async () => {
  const page = app();
  let deletes = 0;
  let reads = 0;
  page.route("**/downloads", () => {
    reads += 1;
    return { json: { downloads: deletes ? [] : [ARTIFACT_ROW] } };
  });
  page.route(`**/downloads/${ARTIFACT}`, (request) => {
    expect(request.method).toBe("DELETE");
    deletes += 1;
    return { status: 204 };
  });
  await page.goto("/workspace/downloads");
  const scope = page.locator("#delivery-history");
  await expect(scope.locator(".download-list > li")).toHaveCount(1);
  await scope.getByRole("button", { name: "刪除", exact: true }).click();
  await expect(scope.getByRole("button", { name: "確認刪除", exact: true })).toBeFocused();
  await page.press("Enter");
  await expect(scope.getByRole("status").filter({ hasText: "已刪除" })).toBeVisible();
  await expect(scope).not.toContainText("還沒有打包過");
  await expect(scope.getByRole("status").filter({ hasText: "目前沒有可顯示的套件" })).toBeVisible();
  expect(deletes).toBe(1);
  expect(reads).toBe(2);
  await verifyLayout(page);
});

test("a selected package is not declared missing during an empty snapshot reread", async () => {
  const page = app();
  let reads = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route("**/downloads", async () => {
    reads += 1;
    if (reads === 1) return { json: { downloads: [] } };
    await gate;
    return { json: { downloads: [ARTIFACT_ROW] } };
  });
  const scope = page.locator("#delivery-history");
  const missing = scope.getByRole("status").filter({ hasText: "目前找不到這筆交付紀錄" });
  try {
    await page.goto(`/workspace/downloads?artifact=${ARTIFACT}`);
    await expect(missing).toBeVisible();
    await goToPolicyAndBack(page);
    await page.eventually(() => expect(reads).toBe(2));
    await expect(missing).toHaveCount(0);
    await expect(scope.getByRole("status").filter({ hasText: "載入下載紀錄中" })).toBeVisible();
    release();
    await expect(scope.locator('[aria-current="location"]')).toBeFocused();
    await expect(missing).toHaveCount(0);
    expect(new URLSearchParams(page.search()).get("artifact")).toBe(ARTIFACT);
    expect(reads).toBe(2);
    await verifyLayout(page);
  } finally {
    release();
  }
});
