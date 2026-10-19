import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  ADMIN_AUDIT_LOG,
  ADMIN_AGENT_FINDINGS,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import { setOffline } from "./offline";

const surfaces = [
  {
    name: "statement",
    url: "/workspace/account",
    region: 'section[aria-label="點數紀錄"]',
    rows: ".download-list > li",
    what: "點數紀錄",
    more: "載入更早的紀錄",
    retry: "再試一次",
    heading: "#account-credits-heading",
    cursor: "credit-1",
    pattern: /\/me\/credits\/entries(?:\?.*)?$/,
  },
  {
    name: "audit",
    url: "/admin/audit-log",
    region: "main",
    rows: "tbody tr",
    what: "動作紀錄",
    more: "載入更多",
    retry: "重試載入更多",
    heading: "#main-content",
    cursor: "audit-1",
    pattern: /\/admin\/audit-log\?/,
  },
  {
    name: "findings",
    url: "/admin/agents",
    region: "#admin-agent-findings",
    rows: ".download-list > li",
    what: "待辦",
    more: "載入更多待辦",
    retry: "重試載入更多待辦",
    heading: "#admin-agent-findings-heading",
    cursor: "finding-1",
    pattern: /\/admin\/agents\/findings(?:\?.*)?$/,
  },
] as const;

type Surface = (typeof surfaces)[number];
type Stage = "initial retry" | "cached retry" | "next page" | "next retry";

function pageBody(surface: Surface, next: boolean) {
  if (surface.name === "statement")
    return {
      entries: [
        {
          id: next ? "credit-2" : "credit-1",
          kind: "grant",
          label: next ? "較早的點數紀錄" : "先前取得的點數紀錄",
          delta_credits: 40,
          estimated: false,
          created_at: "2026-08-17T09:00:00Z",
        },
      ],
      next_before: next ? undefined : surface.cursor,
      note: "",
    };
  if (surface.name === "audit")
    return {
      events: [ADMIN_AUDIT_LOG.events[next ? 1 : 0]],
      next_cursor: next ? undefined : surface.cursor,
    };
  return {
    findings: [
      {
        ...ADMIN_AGENT_FINDINGS.findings[0],
        id: next ? "dddddddd-dddd-4ddd-8ddd-000000000002" : ADMIN_AGENT_FINDINGS.findings[0].id,
        title: next ? "較早的待辦" : "先前取得的待辦",
      },
    ],
    counts: ADMIN_AGENT_FINDINGS.counts,
    next_cursor: next ? undefined : surface.cursor,
  };
}

async function cursorPages(page: Page, surface: Surface, stage: Stage) {
  await stubPlatform(page);
  const cursors: Array<string | null> = [];
  const writes: string[] = [];
  let nextReads = 0;
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  await page.route(surface.pattern, (route) => {
    const url = new URL(route.request().url());
    const cursor = url.searchParams.get(surface.name === "statement" ? "before" : "cursor");
    cursors.push(cursor);
    if (cursor) {
      expect(cursor).toBe(surface.cursor);
      nextReads += 1;
    }
    if (
      (stage === "initial retry" && cursors.length === 1) ||
      (stage === "cached retry" && cursors.length === 2) ||
      (stage === "next retry" && nextReads === 1)
    )
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    return route.fulfill({ json: pageBody(surface, Boolean(cursor)) });
  });
  return { cursors, writes };
}

async function inspect(page: Page, name: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(`${name}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`${name}.png`) });
}

async function expectPausedButtons(region: Locator, surface: Surface, stage: Stage) {
  if (stage === "initial retry") {
    await expect(region.getByRole("button")).toHaveCount(0);
    return;
  }
  const action = region.getByRole("button").last();
  await expect(action).toBeDisabled();
  if (stage === "cached retry") {
    await expect(action).toHaveText("重新讀取中…");
    await expect(region.getByRole("button")).toHaveCount(1);
    return;
  }
  await expect(action).toHaveText(
    surface.name === "statement" && stage === "next retry" ? "重新讀取中…" : "等待連線…",
  );
  if (surface.name !== "statement") {
    const buttons = region.getByRole("button", { name: "等待連線…", exact: true });
    await expect(buttons).toHaveCount(2);
    for (const button of await buttons.all()) await expect(button).toBeDisabled();
  }
}

async function openCursorRead(page: Page, surface: Surface, stage: Stage) {
  const region = page.locator(surface.region);
  const rows = region.locator(surface.rows);
  const action = region.getByRole("button").last();
  const firstPage = stage === "initial retry" || stage === "cached retry";
  await page.goto(surface.url);
  if (stage === "cached retry") {
    await expect(rows).toHaveCount(1);
    await page.getByRole("link", { name: "資料保存政策", exact: true }).click();
    await expect(page).toHaveURL(/\/policy$/);
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    await page.goBack();
  }
  if (firstPage) {
    await expect(region.getByRole("alert")).toBeVisible();
    await expect(rows).toHaveCount(0);
    await expect(action).toHaveText("再試一次");
  } else {
    await expect(rows).toHaveCount(1);
    await expect(action).toHaveText(surface.more);
    if (stage === "next retry") {
      await action.focus();
      await page.keyboard.press("Enter");
      await expect(region.getByRole("alert")).toBeVisible();
      await expect(rows).toHaveCount(1);
      await expect(action).toHaveText(surface.retry);
    }
  }
  return { region, rows, action, firstPage };
}

for (const surface of surfaces) {
  for (const stage of ["initial retry", "cached retry", "next page", "next retry"] as const) {
    for (const moved of stage === "next page" || stage === "next retry" ? [false, true] : [false]) {
      test(`${surface.name} ${stage} waits for connectivity and ${moved ? "preserves moved focus" : "resumes the intended read"}`, async ({
        page,
      }) => {
        await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
        await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
        const { cursors, writes } = await cursorPages(page, surface, stage);
        const { region, rows, action, firstPage } = await openCursorRead(page, surface, stage);
        const completedRows = firstPage ? 1 : 2;
        let requested = false;
        let resumed = false;
        let paused: unknown;
        try {
          const before = [...cursors];
          await setOffline(page, true);
          await action.focus();
          await page.keyboard.press("Enter");
          requested = true;
          const waiting = region
            .getByRole("status")
            .filter({ hasText: `等待連線以讀取${surface.what}` });
          await expect(waiting).toContainText("恢復連線後會繼續讀取。");
          await expect(rows).toHaveCount(firstPage ? 0 : 1);
          await expectPausedButtons(region, surface, stage);
          expect(cursors).toEqual(before);
          const navigation = page
            .getByRole("navigation", { name: "主要導覽", exact: true })
            .getByRole("link", { name: "工作台", exact: true });
          if (moved) await navigation.focus();
          await page.screenshot({ path: test.info().outputPath("cursor-waiting.png") });
          if (moved || firstPage) await inspect(page, "cursor-waiting");
          paused = await page.evaluate(() => ({ online: navigator.onLine }));
          await setOffline(page, false);
          await expect(rows).toHaveCount(completedRows);
          resumed = true;
          await expect(waiting).toHaveCount(0);
          if (moved) await expect(navigation).toBeFocused();
          else if (firstPage || (surface.name === "statement" && stage === "next retry"))
            await expect(page.locator(surface.heading)).toBeFocused();
          else await expect(rows.last()).toBeFocused();
          expect(cursors).toEqual(
            {
              "initial retry": [null, null],
              "cached retry": [null, null, null],
              "next retry": [null, surface.cursor, surface.cursor],
              "next page": [null, surface.cursor],
            }[stage],
          );
          expect(writes).toEqual([]);
          await inspect(page, "cursor-resumed");
        } finally {
          if (!resumed) {
            paused = await page.evaluate(() => ({ online: navigator.onLine }));
            await setOffline(page, false);
            if (requested) await expect(rows).toHaveCount(completedRows);
          }
          await writeFile(
            test.info().outputPath("cursor-connectivity-state.json"),
            JSON.stringify({ surface: surface.name, stage, moved, cursors, writes, paused }),
          );
        }
      });
    }
  }
}

test("a cached credit balance failure waits for connectivity and keeps recovery focus", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await stubPlatform(page);
  let reads = 0;
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  await page.route("**/me/credits", (route) => {
    reads += 1;
    return reads === 2
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: platformResponse("/me/credits").body });
  });
  const balance = page.getByRole("region", { name: "點數餘額", exact: true });
  try {
    await page.goto("/workspace/account");
    await expect(balance.getByText("目前餘額", { exact: false })).toBeVisible();
    await page.getByRole("link", { name: "資料保存政策", exact: true }).click();
    await expect(page).toHaveURL(/\/policy$/);
    await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
    await page.goBack();
    await expect(balance.getByRole("alert")).toBeVisible();
    await expect(balance.getByText("目前餘額", { exact: false })).toHaveCount(0);
    await setOffline(page, true);
    await balance.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(balance.getByRole("status")).toContainText("等待連線以讀取點數餘額");
    await expect(balance.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
    expect(reads).toBe(2);
    await inspect(page, "balance-waiting");
    await setOffline(page, false);
    await expect(balance.getByText("目前餘額", { exact: false })).toBeVisible();
    await expect(page.locator("#account-credits-heading")).toBeFocused();
    expect(reads).toBe(3);
    expect(writes).toEqual([]);
    await inspect(page, "balance-resumed");
  } finally {
    await setOffline(page, false);
    await writeFile(
      test.info().outputPath("balance-connectivity-state.json"),
      JSON.stringify({ reads, writes }),
    );
  }
});
