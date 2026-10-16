import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { RUN, SUGGESTIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });

async function decisionOutage(
  page: Page,
  outcome: "pending" | "unknown" | "refused",
  choice: "accepted" | "rejected",
) {
  const items = ["s1", "s2"].map((id) => ({
    ...SUGGESTIONS.suggestions[0],
    suggestion_id: id,
    decision: id === "s2" && choice === "rejected" ? "accepted" : "pending",
  }));
  let reads = 0;
  await page.route(`**/runs/${RUN}/suggestions`, (route) => {
    reads++;
    if (reads === 2)
      return route.fulfill({ status: 503, json: { error: "controlled list outage" } });
    const suggestions = items.map((item) => ({
      ...item,
      suggestion_id:
        outcome !== "refused" && choice === "accepted" && reads >= 3
          ? item.suggestion_id.toUpperCase()
          : item.suggestion_id,
    }));
    return route.fulfill({ json: { ...SUGGESTIONS, suggestions } });
  });
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const writes: { id: string; decision: string }[] = [];
  let delivered = false;
  await page.route("**/suggestions/*/decision", async (route) => {
    const id = new URL(route.request().url()).pathname.split("/")[2].toLowerCase();
    const intent = route.request().postDataJSON();
    writes.push({ id, decision: intent.decision });
    const first = id === "s2" && writes.filter((write) => write.id === "s2").length === 1;
    if (first) await gate;
    if (first && outcome === "refused") {
      await route.fulfill({ status: 404, json: { error: "suggestion not found" } });
    } else {
      const item = items.find((candidate) => candidate.suggestion_id === id)!;
      item.decision = intent.decision;
      await route.fulfill({ json: first && outcome === "unknown" ? null : item });
    }
    if (id === "s2") delivered = true;
  });
  return { writes, release, reads: () => reads, delivered: () => delivered };
}

async function inspectDecisionState(page: Page, target: Locator) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath("decision-lifetime-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await target.scrollIntoViewIfNeeded();
  await page.screenshot({ path: test.info().outputPath("decision-lifetime-viewport.png") });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
}

for (const [outcome, choice] of [
  ["pending", "accepted"],
  ["pending", "rejected"],
  ["unknown", "accepted"],
  ["unknown", "rejected"],
  ["refused", "accepted"],
] as const) {
  test(`${outcome} ${choice} decision state survives replacement of its row`, async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.emulateMedia({ colorScheme: choice === "accepted" ? "light" : "dark" });
    await stubPlatform(page);
    const control = await decisionOutage(page, outcome, choice);
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const comment = page.locator("#feedback-comment");
      await comment.fill("保留這次判定的補充說明");
      const rows = page.locator("li.suggestion");
      await expect(rows).toHaveCount(2);
      const action = choice === "accepted" ? /接受/ : /拒絕/;
      await rows.nth(1).getByRole("button", { name: action }).click();
      await expect.poll(() => control.writes).toEqual([{ id: "s2", decision: choice }]);
      await rows.nth(0).getByRole("button", { name: "接受", exact: true }).click();
      await expect(
        page.getByRole("alert").filter({ hasText: "暫時無法讀取改善建議" }),
      ).toBeVisible();
      await expect(rows).toHaveCount(0);
      if (outcome !== "pending") {
        control.release();
        await expect.poll(control.delivered).toBe(true);
      }
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(rows).toHaveCount(2);
      expect(control.reads()).toBe(3);
      expect(control.writes).toEqual([
        { id: "s2", decision: choice },
        { id: "s1", decision: "accepted" },
      ]);
      const second = rows.nth(1);
      const apply = page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ });
      if (outcome === "pending") {
        expect(control.delivered()).toBe(false);
        await expect(second.getByRole("button", { name: /接受/ })).toBeDisabled();
        await expect(second.getByRole("button", { name: /拒絕/ })).toBeDisabled();
        await expect(second.getByRole("status")).toContainText(
          choice === "accepted" ? "正在接受" : "正在拒絕",
        );
        await expect(apply).toBeDisabled();
        await inspectDecisionState(page, second);
        control.release();
      } else {
        const alert = second.getByRole("alert");
        await expect(alert).toContainText(
          outcome === "unknown"
            ? `尚未確認「${choice === "accepted" ? "接受" : "拒絕"}」是否已記錄`
            : "suggestion not found",
        );
        await expect(alert).toBeFocused();
        if (outcome === "refused") await expect(alert).toContainText("這個決定被拒絕");
        await inspectDecisionState(page, alert);
        if (outcome === "unknown") {
          await expect(apply).toBeDisabled();
          await second.getByRole("button", { name: action }).click();
        } else {
          await expect(apply).toBeEnabled();
          await expect(page.locator("#suggestion-decisions-note")).toHaveCount(0);
        }
      }
      if (outcome !== "refused") {
        await expect.poll(control.delivered).toBe(true);
        await expect.poll(control.reads).toBe(4);
        await expect(second.getByRole("button", { name: action })).toHaveAttribute(
          "aria-pressed",
          "true",
        );
        await expect(second.getByRole("button", { name: action })).toBeEnabled();
        await expect(second.getByRole("alert")).toHaveCount(0);
        await expect(apply).toBeEnabled();
        await expect(page.locator("#suggestion-decisions-note")).toHaveCount(0);
      }
      await expect(comment).toHaveValue("保留這次判定的補充說明");
      expect(control.writes).toHaveLength(outcome === "unknown" ? 3 : 2);
    } finally {
      await writeFile(
        test.info().outputPath("decision-lifetime-state.json"),
        JSON.stringify({
          reads: control.reads(),
          writes: control.writes,
          delivered: control.delivered(),
          text: await page.locator("body").innerText(),
        }),
      );
      await page.screenshot({ path: test.info().outputPath("decision-lifetime-finally.png") });
      control.release();
      if (control.writes.some((write) => write.id === "s2"))
        await expect.poll(control.delivered).toBe(true);
    }
  });
}
