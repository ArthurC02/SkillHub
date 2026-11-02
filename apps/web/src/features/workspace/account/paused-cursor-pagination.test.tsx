import { expect, test } from "vitest";
import {
  ADMIN_AGENT_FINDINGS,
  ADMIN_AUDIT_LOG,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { Locator, renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

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

const lastOf = (locator: Locator) => new Locator(() => locator.all().slice(-1));
const writesOf = (page: App) => page.requests.filter((request) => request.method !== "GET");

function cursorPages(page: App, surface: Surface, stage: Stage) {
  const cursors: Array<string | null> = [];
  let nextReads = 0;
  page.route(surface.pattern, ({ url }) => {
    const cursor = url.searchParams.get(surface.name === "statement" ? "before" : "cursor");
    cursors.push(cursor);
    if (cursor) nextReads += 1;
    if (
      (stage === "initial retry" && cursors.length === 1) ||
      (stage === "cached retry" && cursors.length === 2) ||
      (stage === "next retry" && nextReads === 1)
    )
      return { status: 503, json: { error: "service unavailable" } };
    return { json: pageBody(surface, Boolean(cursor)) };
  });
  return cursors;
}

async function goThroughPolicyAndBack(page: App) {
  await page.getByRole("link", { name: "資料保存政策", exact: true }).click();
  await page.eventually(() => {
    expect(page.pathname()).toBe("/policy");
    expect(page.getByRole("heading", { name: "資料保存政策", exact: true }).count()).toBe(1);
  });
  await page.goBack();
}

async function expectPausedButtons(page: App, region: Locator, surface: Surface, stage: Stage) {
  const buttons = region.locator("button");
  if (stage === "initial retry") {
    await expect(buttons).toHaveCount(0);
    return;
  }
  const action = lastOf(buttons);
  if (stage === "cached retry") {
    await page.eventually(() => {
      expect(action.isDisabled()).toBe(true);
      expect(action.text()).toBe("重新讀取中…");
      expect(buttons.count()).toBe(1);
    });
    return;
  }
  const expectedText =
    surface.name === "statement" && stage === "next retry" ? "重新讀取中…" : "等待連線…";
  await page.eventually(() => {
    expect(action.isDisabled()).toBe(true);
    expect(action.text()).toBe(expectedText);
    if (surface.name !== "statement") {
      const waiting = buttons.filter({ hasText: "等待連線…" });
      expect(waiting.count()).toBe(2);
      for (let i = 0; i < 2; i += 1) expect(waiting.nth(i).isDisabled()).toBe(true);
    }
  });
}

async function openCursorRead(page: App, surface: Surface, stage: Stage) {
  const region = page.locator(surface.region);
  const rows = region.locator(surface.rows);
  const action = lastOf(region.locator("button"));
  const firstPage = stage === "initial retry" || stage === "cached retry";
  await page.goto(surface.url);
  if (stage === "cached retry") {
    await expect(rows).toHaveCount(1);
    await goThroughPolicyAndBack(page);
  }
  if (firstPage) {
    await page.eventually(() => {
      expect(region.locator("[role=alert]").count()).toBeGreaterThan(0);
      expect(rows.count()).toBe(0);
      expect(action.text()).toBe("再試一次");
    });
  } else {
    await page.eventually(() => {
      expect(rows.count()).toBe(1);
      expect(action.text()).toBe(surface.more);
    });
    if (stage === "next retry") {
      await action.focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(region.locator("[role=alert]").count()).toBeGreaterThan(0);
        expect(rows.count()).toBe(1);
        expect(action.text()).toBe(surface.retry);
      });
    }
  }
  return { region, rows, action, firstPage };
}

for (const surface of surfaces) {
  for (const stage of ["initial retry", "cached retry", "next page", "next retry"] as const) {
    for (const moved of stage === "next page" || stage === "next retry" ? [false, true] : [false]) {
      test(`${surface.name} ${stage} waits for connectivity and ${moved ? "preserves moved focus" : "resumes the intended read"}`, async () => {
        const page = app();
        const cursors = cursorPages(page, surface, stage);
        const { region, rows, action, firstPage } = await openCursorRead(page, surface, stage);
        const completedRows = firstPage ? 1 : 2;
        let requested = false;
        let resumed = false;
        try {
          const before = [...cursors];
          await page.setOffline(true);
          await action.focus();
          await page.press("Enter");
          requested = true;
          const waiting = region
            .locator("[role=status]")
            .filter({ hasText: `等待連線以讀取${surface.what}` });
          await page.eventually(() => {
            expect(waiting.text()).toContain("恢復連線後會繼續讀取。");
            expect(rows.count()).toBe(firstPage ? 0 : 1);
          });
          await expectPausedButtons(page, region, surface, stage);
          expect(cursors).toEqual(before);
          const navigation = page
            .getByRole("navigation", { name: "主要導覽", exact: true })
            .locator("a")
            .filter({ hasText: "工作台" });
          if (moved) await navigation.focus();
          if (moved || firstPage) expect(await page.axeViolations("body")).toEqual([]);
          await page.setOffline(false);
          await expect(rows).toHaveCount(completedRows);
          resumed = true;
          await page.eventually(() => {
            expect(waiting.count()).toBe(0);
            if (moved) expect(navigation.isFocused()).toBe(true);
            else if (firstPage || (surface.name === "statement" && stage === "next retry"))
              expect(page.locator(surface.heading).isFocused()).toBe(true);
            else expect(lastOf(rows).isFocused()).toBe(true);
          });
          expect(cursors).toEqual(
            {
              "initial retry": [null, null],
              "cached retry": [null, null, null],
              "next retry": [null, surface.cursor, surface.cursor],
              "next page": [null, surface.cursor],
            }[stage],
          );
          expect(writesOf(page)).toEqual([]);
          expect(await page.axeViolations("body")).toEqual([]);
        } finally {
          if (!resumed) {
            await page.setOffline(false);
            if (requested) await expect(rows).toHaveCount(completedRows);
          }
        }
      });
    }
  }
}

test("a cached credit balance failure waits for connectivity and keeps recovery focus", async () => {
  const page = app();
  let reads = 0;
  page.route("**/me/credits", () => {
    reads += 1;
    return reads === 2
      ? { status: 503, json: { error: "service unavailable" } }
      : { json: platformResponse("/me/credits").body };
  });
  const balance = page.getByRole("region", { name: "點數餘額", exact: true });
  const retry = balance.locator("button").filter({ hasText: "再試一次" });
  try {
    await page.goto("/workspace/account");
    await expect(balance).toContainText("目前餘額");
    await goThroughPolicyAndBack(page);
    await page.eventually(() => {
      expect(balance.locator("[role=alert]").count()).toBe(1);
      expect(balance.text()).not.toContain("目前餘額");
    });
    await page.setOffline(true);
    await retry.focus();
    await page.press("Enter");
    await page.eventually(() => {
      expect(balance.locator("[role=status]").text()).toContain("等待連線以讀取點數餘額");
      const reading = balance.locator("button").filter({ hasText: "重新讀取中…" });
      expect(reading.isDisabled()).toBe(true);
    });
    expect(reads).toBe(2);
    expect(await page.axeViolations("body")).toEqual([]);
    await page.setOffline(false);
    await page.eventually(() => {
      expect(balance.text()).toContain("目前餘額");
      expect(page.locator("#account-credits-heading").isFocused()).toBe(true);
    });
    expect(reads).toBe(3);
    expect(writesOf(page)).toEqual([]);
    expect(await page.axeViolations("body")).toEqual([]);
  } finally {
    await page.setOffline(false);
  }
});
