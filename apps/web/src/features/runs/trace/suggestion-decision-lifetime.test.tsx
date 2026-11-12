import { expect, test } from "vitest";
import { RUN, SUGGESTIONS } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";
import { stubScrollIntoView } from "../../../testing/dom";

const app = renderedApp();

stubScrollIntoView();

function decisionOutage(
  page: App,
  outcome: "pending" | "unknown" | "refused",
  choice: "accepted" | "rejected",
) {
  const items = ["s1", "s2"].map((id) => ({
    ...SUGGESTIONS.suggestions[0],
    suggestion_id: id,
    decision: id === "s2" && choice === "rejected" ? "accepted" : "pending",
  }));
  let reads = 0;
  page.route(`**/runs/${RUN}/suggestions`, () => {
    reads++;
    if (reads === 2) return { status: 503, json: { error: "controlled list outage" } };
    const suggestions = items.map((item) => ({
      ...item,
      suggestion_id:
        outcome !== "refused" && choice === "accepted" && reads >= 3
          ? item.suggestion_id.toUpperCase()
          : item.suggestion_id,
    }));
    return { json: { ...SUGGESTIONS, suggestions } };
  });
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const writes: { id: string; decision: string }[] = [];
  let delivered = false;
  page.route("**/suggestions/*/decision", async (request) => {
    const id = request.url.pathname.split("/")[2].toLowerCase();
    const intent = request.body as { decision: string };
    writes.push({ id, decision: intent.decision });
    const first = id === "s2" && writes.filter((write) => write.id === "s2").length === 1;
    if (first) await gate;
    if (id === "s2") delivered = true;
    if (first && outcome === "refused")
      return { status: 404, json: { error: "suggestion not found" } };
    const item = items.find((candidate) => candidate.suggestion_id === id)!;
    item.decision = intent.decision;
    return { json: first && outcome === "unknown" ? null : item };
  });
  return { writes, release, reads: () => reads, delivered: () => delivered };
}

for (const [outcome, choice] of [
  ["pending", "accepted"],
  ["pending", "rejected"],
  ["unknown", "accepted"],
  ["unknown", "rejected"],
  ["refused", "accepted"],
] as const) {
  test(`${outcome} ${choice} decision state survives replacement of its row`, async () => {
    const page = app();
    const control = decisionOutage(page, outcome, choice);
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const comment = page.locator("#feedback-comment");
      await expect(comment).toHaveCount(1);
      await comment.fill("保留這次判定的補充說明");
      const rows = page.locator("li.suggestion");
      await expect(rows).toHaveCount(2);
      const action = choice === "accepted" ? /接受/ : /拒絕/;
      await rows.nth(1).getByRole("button", { name: action }).click();
      await page.eventually(() => expect(control.writes).toEqual([{ id: "s2", decision: choice }]));
      await rows.nth(0).getByRole("button", { name: "接受", exact: true }).click();
      await page.eventually(() => {
        expect(page.getByRole("alert").filter({ hasText: "暫時無法讀取改善建議" }).count()).toBe(1);
        expect(rows.count()).toBe(0);
      });
      if (outcome !== "pending") {
        control.release();
        await page.eventually(() => expect(control.delivered()).toBe(true));
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
        await page.eventually(() => {
          expect(second.getByRole("button", { name: /接受/ }).isDisabled()).toBe(true);
          expect(second.getByRole("button", { name: /拒絕/ }).isDisabled()).toBe(true);
          expect(second.getByRole("status").text()).toContain(
            choice === "accepted" ? "正在接受" : "正在拒絕",
          );
          expect(apply.isDisabled()).toBe(true);
        });
        expect(await page.axeViolations("body")).toEqual([]);
        control.release();
      } else {
        const alert = second.getByRole("alert");
        await page.eventually(() => {
          expect(alert.text()).toContain(
            outcome === "unknown"
              ? `尚未確認「${choice === "accepted" ? "接受" : "拒絕"}」是否已記錄`
              : "這個決定被拒絕。這個建議已經不在了，請重新讀取這份評估。",
          );
          expect(alert.isFocused()).toBe(true);
        });
        expect(await page.axeViolations("body")).toEqual([]);
        if (outcome === "unknown") {
          await expect(apply).toBeDisabled();
          await second.getByRole("button", { name: action }).click();
        } else {
          await expect(apply).toBeEnabled();
          await expect(page.locator("#suggestion-decisions-note")).toHaveCount(0);
        }
      }
      if (outcome !== "refused") {
        await page.eventually(() => {
          expect(control.delivered()).toBe(true);
          expect(control.reads()).toBe(4);
          expect(second.getByRole("button", { name: action }).attribute("aria-pressed")).toBe(
            "true",
          );
          expect(second.getByRole("button", { name: action }).isDisabled()).toBe(false);
          expect(second.getByRole("alert").count()).toBe(0);
          expect(apply.isDisabled()).toBe(false);
          expect(page.locator("#suggestion-decisions-note").count()).toBe(0);
        });
      }
      await expect(comment).toHaveValue("保留這次判定的補充說明");
      expect(control.writes).toHaveLength(outcome === "unknown" ? 3 : 2);
    } finally {
      control.release();
    }
  });
}
