import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, SUGGESTIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });

const DIFF = { target_path: "SKILL.md", applicable: true, unified_diff: "+ 清楚列出欄位" };
const DRAFT = "保留這份評估的補充說明";

async function inspectRead(page: Page, target: Locator, label: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath(`${label}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await target.scrollIntoViewIfNeeded();
  await page.screenshot({ path: test.info().outputPath(`${label}-viewport.png`) });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
}

for (const [kind, fault, payload] of [
  ["list", "null envelope", null],
  ["list", "missing items", { evaluation_id: SUGGESTIONS.evaluation_id }],
  [
    "list",
    "unknown choice",
    { ...SUGGESTIONS, suggestions: [{ ...SUGGESTIONS.suggestions[0], decision: "approve" }] },
  ],
  [
    "list",
    "unreadable evidence",
    { ...SUGGESTIONS, suggestions: [{ ...SUGGESTIONS.suggestions[0], evidence: [null] }] },
  ],
  ["diff", "null envelope", null],
  ["diff", "missing fields", {}],
  ["diff", "string applicability", { ...DIFF, applicable: "true" }],
  [
    "diff",
    "unknown reason",
    { target_path: "SKILL.md", applicable: false, blocked_reason: "unknown" },
  ],
] as const) {
  for (const phase of ["first read", "reread"] as const) {
    test(`${kind} with ${fault} on ${phase} keeps Run context and recovers only that section`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 375, height: 812 });
      await page.emulateMedia({ colorScheme: phase === "first read" ? "light" : "dark" });
      await stubPlatform(page);
      let release!: () => void;
      const faultGate = new Promise<void>((resolve) => {
        release = resolve;
      });
      let reads = 0;
      const failAt = phase === "first read" ? 1 : 2;
      const writes: unknown[] = [];
      const endpoint = kind === "list" ? `**/runs/${RUN}/suggestions` : "**/suggestions/s1/diff";
      await page.route(endpoint, async (route) => {
        reads++;
        if (reads === failAt) {
          await faultGate;
          return route.fulfill({ json: payload });
        }
        return route.fulfill({ json: kind === "list" ? SUGGESTIONS : DIFF });
      });
      await page.route("**/suggestions/s1/decision", (route) => {
        writes.push(route.request().postDataJSON());
        return route.fulfill({ json: SUGGESTIONS.suggestions[0] });
      });
      try {
        await page.goto(`/runs/${RUN}`);
        await expect(page.locator(".auth-name")).toHaveText("tester");
        const comment = page.locator("#feedback-comment");
        await comment.fill(DRAFT);
        if (kind === "list" && phase === "reread") {
          await page
            .locator("li.suggestion")
            .getByRole("button", { name: "接受", exact: true })
            .click();
        }
        if (kind === "diff") {
          const collapsed = page.getByRole("button", { name: "查看差異", exact: true });
          await expect(collapsed).toHaveAttribute("aria-expanded", "false");
          if (phase === "first read") await collapsed.press("Enter");
          else await collapsed.click();
          const expanded = page.getByRole("button", { name: "收起差異", exact: true });
          await expect(expanded).toHaveAttribute("aria-expanded", "true");
          if (phase === "reread") {
            await expect(page.locator(".diff")).toContainText("清楚列出欄位");
            await expanded.press("Space");
            await expect(collapsed).toHaveAttribute("aria-expanded", "false");
            await expect(collapsed).toBeFocused();
            await expect(page.getByRole("region", { name: "建議差異", exact: true })).toHaveCount(
              0,
            );
            await collapsed.press("Enter");
            await expect(expanded).toHaveAttribute("aria-expanded", "true");
            await expect(expanded).toBeFocused();
          }
        }
        await expect.poll(() => reads).toBe(failAt);
        release();
        const alert = page
          .getByRole("alert")
          .filter({ hasText: kind === "list" ? "暫時無法讀取改善建議" : "暫時無法讀取差異" });
        await expect(alert).toBeVisible();
        await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toBeVisible();
        await expect(comment).toHaveValue(DRAFT);
        await expect(
          page.getByRole("heading", { name: "這一頁暫時無法顯示", exact: true }),
        ).toHaveCount(0);
        await expect(page.getByText("沒有可顯示的差異。", { exact: true })).toHaveCount(0);
        if (kind === "list") await expect(page.locator("li.suggestion")).toHaveCount(0);
        else await expect(page.locator(".diff")).toHaveCount(0);
        await inspectRead(page, alert, "read-failure");
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
        if (kind === "list") {
          await expect(page.getByRole("heading", { name: "改善建議", exact: true })).toBeFocused();
          await expect(page.locator("li.suggestion")).toHaveCount(1);
          await expect(
            page.getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true }),
          ).toBeEnabled();
        } else {
          await expect(page.getByRole("region", { name: "建議差異", exact: true })).toBeFocused();
          await expect(page.getByRole("button", { name: "收起差異", exact: true })).toHaveAttribute(
            "aria-expanded",
            "true",
          );
          await expect(page.locator(".diff")).toContainText("清楚列出欄位");
        }
        await expect(alert).toHaveCount(0);
        await expect(comment).toHaveValue(DRAFT);
        expect(reads).toBe(failAt + 1);
        expect(writes).toEqual(
          kind === "list" && phase === "reread" ? [{ decision: "accepted" }] : [],
        );
      } finally {
        release();
      }
    });
  }
}

test("a readable empty improvement list stays distinct from a failed read", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await stubPlatform(page);
  await page.route(`**/runs/${RUN}/suggestions`, (route) =>
    route.fulfill({ json: { ...SUGGESTIONS, suggestions: [] } }),
  );
  await page.goto(`/runs/${RUN}`);
  const message = page.getByText("這份評估沒有產生改善建議。", { exact: true });
  await expect(message).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
  await inspectRead(page, message, "empty-list");
});

for (const applicable of [true, false]) {
  test(`a readable empty diff with applicable=${applicable} keeps its actual meaning`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.emulateMedia({ colorScheme: applicable ? "light" : "dark" });
    await stubPlatform(page);
    await page.route("**/suggestions/s1/diff", (route) =>
      route.fulfill({ json: { target_path: "SKILL.md", applicable, unified_diff: "" } }),
    );
    await page.goto(`/runs/${RUN}`);
    await page.getByRole("button", { name: "查看差異", exact: true }).click();
    const region = page.getByRole("region", { name: "建議差異", exact: true });
    await expect(region).toContainText("沒有可顯示的差異。");
    if (applicable) await expect(region).not.toContainText("目前無法套用");
    else await expect(region).toContainText("目前無法套用：伺服器沒有給原因。");
    await expect(region.getByRole("alert")).toHaveCount(0);
    await inspectRead(page, region, "empty-diff");
  });
}

for (const outcome of ["confirmed", "refused"] as const) {
  test(`a ${outcome} decision settles after another decision's list refresh unmounts their rows`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await stubPlatform(page);
    let reads = 0;
    const items = ["s1", "s2"].map((id) => ({
      ...SUGGESTIONS.suggestions[0],
      suggestion_id: id,
      decision: "pending",
    }));
    await page.route(`**/runs/${RUN}/suggestions`, (route) => {
      reads++;
      if (reads === 2)
        return route.fulfill({ status: 503, json: { error: "controlled list outage" } });
      return route.fulfill({ json: { ...SUGGESTIONS, suggestions: items } });
    });
    let release!: () => void;
    const secondGate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: string[] = [];
    let secondDelivered = false;
    await page.route("**/suggestions/*/decision", async (route) => {
      const id = new URL(route.request().url()).pathname.split("/")[2];
      writes.push(id);
      if (id === "s2") {
        await secondGate;
        if (outcome === "refused") {
          await route.fulfill({ status: 400, json: { error: "controlled decision refusal" } });
          secondDelivered = true;
          return;
        }
      }
      const item = items.find((candidate) => candidate.suggestion_id === id)!;
      item.decision = "accepted";
      await route.fulfill({ json: item });
      if (id === "s2") secondDelivered = true;
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await page.locator("#feedback-comment").fill(DRAFT);
      const rows = page.locator("li.suggestion");
      await expect(rows).toHaveCount(2);
      await rows.nth(1).getByRole("button", { name: "接受", exact: true }).click();
      await expect.poll(() => writes).toEqual(["s2"]);
      await rows.nth(0).getByRole("button", { name: "接受", exact: true }).click();
      const alert = page.getByRole("alert").filter({ hasText: "暫時無法讀取改善建議" });
      await expect(alert).toBeVisible();
      await expect(rows).toHaveCount(0);
      release();
      await expect.poll(() => secondDelivered).toBe(true);
      if (outcome === "refused") {
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
      }
      await expect(rows).toHaveCount(2);
      await expect(
        page.getByRole("button", {
          name: `以已接受的 ${outcome === "confirmed" ? 2 : 1} 項建議建立新版本`,
          exact: true,
        }),
      ).toBeEnabled();
      await expect(page.locator("#suggestion-decisions-note")).toHaveCount(0);
      await expect(page.locator("#feedback-comment")).toHaveValue(DRAFT);
      expect(writes).toEqual(["s2", "s1"]);
      expect(reads).toBe(3);
      await inspectRead(
        page,
        page.getByRole("heading", { name: "改善建議", exact: true }),
        `${outcome}-decisions`,
      );
    } finally {
      release();
    }
  });
}
