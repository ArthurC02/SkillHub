import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_AGENT_FINDINGS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const firstId = "dddddddd-dddd-4ddd-8ddd-000000000001";
const nextId = "dddddddd-dddd-4ddd-8ddd-000000000002";
const firstCursor = "2026-09-10T08:00:00Z_50";
const nextCursor = "2026-09-09T08:00:00Z_100";

async function inspect(page: Page, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`${label}.png`) });
}

type Arrival = "populated" | "empty" | "more" | 503 | 401 | 403 | 404;

async function findingPages(page: Page, arrival: Arrival) {
  await stubPlatform(page);
  const cursors: Array<string | null> = [];
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  let release!: () => void;
  let releaseRecovery!: () => void;
  let recovered = false;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const recovery = new Promise<void>((resolve) => {
    releaseRecovery = resolve;
  });
  await page.route(/\/admin\/agents\/findings(?:\?.*)?$/, async (route) => {
    const url = new URL(route.request().url());
    const cursor = url.searchParams.get("cursor");
    cursors.push(cursor);
    if (!cursor)
      return route.fulfill({
        json: {
          findings: [{ ...ADMIN_AGENT_FINDINGS.findings[0], title: firstId }],
          counts: ADMIN_AGENT_FINDINGS.counts,
          next_cursor: firstCursor,
        },
      });
    if (cursor === nextCursor)
      return route.fulfill({ json: { findings: [], counts: ADMIN_AGENT_FINDINGS.counts } });
    await gate;
    if (typeof arrival === "number" && !recovered)
      return route.fulfill({ status: arrival, json: { error: "controlled finding read" } });
    if (recovered) await recovery;
    return route.fulfill({
      json: {
        findings:
          arrival === "empty"
            ? []
            : [{ ...ADMIN_AGENT_FINDINGS.findings[0], id: nextId, title: nextId }],
        counts: ADMIN_AGENT_FINDINGS.counts,
        next_cursor: arrival === "more" ? nextCursor : undefined,
      },
    });
  });
  return {
    cursors,
    writes,
    release,
    releaseRecovery,
    recover: () => {
      recovered = true;
    },
  };
}

for (const arrival of ["populated", "empty", "more", 503, 401, 403, 404] as const) {
  for (const moved of [false, true]) {
    test(`finding ${arrival} pagination ${moved ? "preserves moved focus" : "continues keyboard focus"}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: moved ? 1280 : 375, height: 900 });
      await page.emulateMedia({ colorScheme: moved ? "dark" : "light" });
      const { cursors, writes, release, releaseRecovery, recover } = await findingPages(
        page,
        arrival,
      );
      try {
        await page.goto("/admin/agents");
        const inbox = page.locator("#admin-agent-findings");
        const rows = inbox.locator("ul.download-list > li");
        const more = inbox
          .getByRole("button")
          .filter({ hasText: /^(載入更多待辦|載入中…|重試載入更多待辦)$/ });
        await expect(rows).toHaveCount(1);
        await more.focus();
        await page.keyboard.press("Enter");
        await expect(more).toHaveText("載入中…");
        await expect(more).toBeDisabled();
        const navigation = page
          .getByRole("navigation", { name: "主要導覽", exact: true })
          .getByRole("link", { name: "工作台", exact: true });
        if (moved) await navigation.focus();
        release();
        if (arrival === 503) {
          await expect(inbox.getByRole("alert")).toContainText("已載入的待辦仍可查看");
          await expect(rows).toHaveCount(1);
          await expect(more).toHaveText("重試載入更多待辦");
          if (!moved) {
            await expect(more).toBeFocused();
            await page.screenshot({
              path: test.info().outputPath("finding-pagination-failed.png"),
            });
            recover();
            await page.keyboard.press("Enter");
            await expect(more).toBeDisabled();
            releaseRecovery();
            await expect(rows).toHaveCount(2);
            await expect(more).toHaveCount(0);
            await expect(rows.filter({ hasText: nextId })).toBeFocused();
            await expect(inbox.getByRole("alert")).toHaveCount(0);
          }
        } else if (typeof arrival === "number") {
          if (arrival === 401) {
            await expect(
              inbox.getByRole("status").filter({ hasText: "待辦需要登入。" }),
            ).toBeVisible();
            await expect(page.getByRole("link", { name: "使用 GitHub 登入" })).toBeVisible();
          } else await expect(inbox.getByRole("alert").first()).toBeVisible();
          await expect(rows).toHaveCount(0);
          await expect(page.getByText(firstId, { exact: true })).toHaveCount(0);
          await expect(more).toHaveCount(0);
          if (!moved)
            await expect(
              page.getByRole("heading", { level: 2, name: "待辦", exact: true }),
            ).toBeFocused();
        } else {
          await expect(rows).toHaveCount(arrival === "empty" ? 1 : 2);
          if (arrival !== "more") await expect(more).toHaveCount(0);
          if (!moved && arrival === "more") {
            await expect(more).toBeFocused();
            await page.keyboard.press("Enter");
            await expect(more).toHaveCount(0);
            await expect(rows.filter({ hasText: nextId })).toBeFocused();
          } else if (!moved)
            await expect(
              rows.filter({ hasText: arrival === "empty" ? firstId : nextId }),
            ).toBeFocused();
        }
        if (moved) await expect(navigation).toBeFocused();
        expect(cursors).toEqual(
          arrival === 503 && !moved
            ? [null, firstCursor, firstCursor]
            : arrival === "more" && !moved
              ? [null, firstCursor, nextCursor]
              : [null, firstCursor],
        );
        expect(writes).toEqual([]);
        await inspect(page, "finding-pagination");
      } finally {
        release();
        releaseRecovery();
        await writeFile(
          test.info().outputPath("finding-pagination-state.json"),
          JSON.stringify({
            arrival,
            moved,
            cursors,
            writes,
            focus: await page.evaluate(() => ({
              tag: document.activeElement?.tagName,
              text: document.activeElement?.textContent?.slice(0, 100),
            })),
          }),
        );
      }
    });
  }
}
