import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_AUDIT_LOG } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const personId = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const agentId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";

async function inspectAccessibility(page: Page) {
  const scroll = page.locator(".table-scroll");
  const maximum = await scroll.evaluate((element) => element.scrollWidth - element.clientWidth);
  const reports = [];
  for (const position of maximum > 1 ? [0, maximum] : [0]) {
    if (maximum > 1) {
      await scroll.hover();
      await page.mouse.wheel(position === 0 ? -2000 : 2000, 0);
      await expect
        .poll(() => scroll.evaluate((element) => Math.round(element.scrollLeft)))
        .toBe(position);
    }
    const report = await new AxeBuilder({ page }).analyze();
    await writeFile(
      test.info().outputPath(`audit-cell-${position === 0 ? "start" : "end"}-a11y.json`),
      JSON.stringify(report, null, 2),
    );
    expect(report.violations).toEqual([]);
    reports.push(report);
  }
  expect(reports).toHaveLength(maximum > 1 ? 2 : 1);
  const contrastPasses = reports.map(
    (report) =>
      new Set(
        report.passes
          .filter((rule) => rule.id === "color-contrast")
          .flatMap((rule) => rule.nodes.map((node) => JSON.stringify(node.target))),
      ),
  );
  const reviewed = [];
  for (const [index, report] of reports.entries()) {
    for (const rule of report.incomplete) {
      expect(rule.id).toBe("color-contrast");
      for (const node of rule.nodes) {
        expect(node.all).toEqual([]);
        expect(node.none).toEqual([]);
        expect(node.any).toHaveLength(1);
        expect(node.any[0].data).toMatchObject({ messageKey: "elmPartiallyObscured" });
        expect(node.any[0].relatedNodes).toEqual([
          expect.objectContaining({ target: [".table-scroll"] }),
        ]);
        expect(
          contrastPasses.some(
            (passes, other) => other !== index && passes.has(JSON.stringify(node.target)),
          ),
        ).toBe(true);
        reviewed.push(node.target);
      }
    }
  }
  await writeFile(
    test.info().outputPath("audit-cell-contrast-review.json"),
    JSON.stringify(
      { scans: reports.length, rawIncompleteNodes: reviewed.length, reviewed, unresolved: 0 },
      null,
      2,
    ),
  );
}

async function contentColumns(page: Page) {
  const columns = await page.locator("tbody").evaluate((body) => {
    const rows = Array.from(body.querySelectorAll("tr"));
    const resources = rows.map((row) => {
      const cell = row.querySelector('td[data-label="對象"]')!;
      const code = cell.querySelector("code")!.getBoundingClientRect();
      return {
        codeLeft: code.left,
        codeRight: code.right,
        cellRight: cell.getBoundingClientRect().right,
        workspaceLeft: cell.querySelector("p")!.getBoundingClientRect().left,
      };
    });
    const actor = rows[1].querySelector('td[data-label="行為者"]')!;
    const walker = document.createTreeWalker(actor, NodeFilter.SHOW_TEXT);
    let node: Node | null;
    while ((node = walker.nextNode())) {
      if (!node.textContent?.includes("平台 Agent")) continue;
      const label = document.createRange();
      label.selectNodeContents(node);
      const code = actor.querySelector("code")!.getBoundingClientRect();
      return {
        resources,
        agent: {
          codeLeft: code.left,
          codeRight: code.right,
          labelLeft: label.getBoundingClientRect().left,
          cellRight: actor.getBoundingClientRect().right,
        },
      };
    }
    throw new Error("Agent label missing");
  });
  expect(columns.resources).toHaveLength(3);
  for (const resource of columns.resources) {
    expect(resource.codeLeft).toBeGreaterThanOrEqual(resource.workspaceLeft - 1);
    expect(resource.codeRight).toBeLessThanOrEqual(resource.cellRight + 1);
  }
  expect(columns.agent.codeLeft).toBeGreaterThanOrEqual(columns.agent.labelLeft - 1);
  expect(columns.agent.codeRight).toBeLessThanOrEqual(columns.agent.cellRight + 1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  return columns;
}

for (const width of [320, 375, 640, 641, 1280]) {
  for (const dark of [false, true]) {
    test(`audit composite cells keep their values together at ${width} ${dark ? "dark" : "light"}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: dark ? "dark" : "light" });
      await stubPlatform(page);
      const first = ADMIN_AUDIT_LOG.events[0];
      await page.route("**/admin/audit-log?*", (route) =>
        route.fulfill({
          json: {
            events: [
              {
                ...first,
                actor_user_id: personId,
                metadata: {
                  ...first.metadata,
                  provider_execution_id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
                },
              },
              { ...first, actor_kind: "agent", actor_user_id: null, actor_agent_id: agentId },
              {
                ...first,
                actor_kind: "system",
                actor_user_id: null,
                actor_agent_id: null,
                resource_id: null,
                workspace_id: null,
                metadata: {},
              },
            ],
          },
        }),
      );
      const writes: string[] = [];
      page.on("request", (request) => {
        if (request.method() !== "GET") writes.push(request.url());
      });
      await page.goto("/admin/audit-log");
      const rows = page.locator("tbody tr");
      await expect(rows).toHaveCount(3);
      const actionLines = await rows
        .nth(0)
        .locator('th[scope="row"]')
        .evaluate((element) => {
          const range = document.createRange();
          range.selectNodeContents(element);
          return range.getClientRects().length;
        });
      expect(actionLines).toBe(1);
      await expect(page.getByRole("columnheader")).toHaveCount(5);
      await expect(rows.nth(0).locator('[data-label="行為者"]')).toHaveText(personId);
      await expect(rows.nth(1).locator('[data-label="行為者"]')).toContainText("平台 Agent");
      await expect(rows.nth(1).locator('[data-label="行為者"] code')).toHaveText(agentId);
      await expect(rows.nth(2).locator('[data-label="行為者"]')).toHaveText("平台自動");
      await expect(rows.nth(0).locator('[data-label="對象"] code').first()).toHaveText(
        first.resource_id,
      );
      await expect(rows.nth(0).locator('[data-label="對象"] p code')).toHaveText(
        first.workspace_id,
      );
      await expect(rows.nth(2).locator('[data-label="對象"] code')).toHaveText("不適用");
      await expect(rows.nth(2).locator('[data-label="對象"] p')).toHaveText("工作區：不適用");
      await expect(rows.nth(2).locator('[data-label="內容"]')).toHaveText("不適用");
      const collapsed = await contentColumns(page);
      const details = rows.nth(0).locator("details");
      const summary = details.locator("summary");
      await summary.focus();
      await page.keyboard.press("Enter");
      await expect(details).toHaveJSProperty("open", true);
      await expect(details.getByText("beta reward", { exact: true })).toBeVisible();
      const wordLines = await details
        .getByText("beta reward", { exact: true })
        .evaluate((element) => {
          const range = document.createRange();
          range.setStart(element.firstChild!, 5);
          range.setEnd(element.firstChild!, 11);
          return range.getClientRects().length;
        });
      expect(wordLines).toBe(1);
      const fields = await details
        .locator("dt,dd")
        .evaluateAll((elements) =>
          elements.map((element) => ({ width: element.clientWidth, content: element.scrollWidth })),
        );
      expect(fields).toHaveLength(8);
      for (const field of fields) expect(field.content).toBeLessThanOrEqual(field.width + 1);
      await page.keyboard.press("Space");
      await expect(details).toHaveJSProperty("open", false);
      await expect(summary).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(details).toHaveJSProperty("open", true);
      const expanded = await contentColumns(page);
      expect(writes).toEqual([]);
      await writeFile(
        test.info().outputPath("audit-cell-columns.json"),
        JSON.stringify({ width, dark, collapsed, expanded }, null, 2),
      );
      await inspectAccessibility(page);
      await page.screenshot({
        path: test.info().outputPath("audit-cell-layout.png"),
        fullPage: true,
      });
    });
  }
}
