import { expect, test } from "vitest";
import { EVALUATION, RUN, SKILL, SUGGESTIONS } from "../../testing/fixtures/platform";
import { renderedApp, type App, type Reply } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

function evaluationOutage(
  page: App,
  outcome: "pending" | "unknown" | "refused",
  choice: "accepted" | "rejected",
) {
  let reads = 0;
  let feedbackWrites = 0;
  let feedback: unknown = EVALUATION.feedback;
  page.route(`**/runs/${RUN}/evaluation`, () => {
    reads++;
    return reads === 2
      ? { status: 503, json: { error: "controlled evaluation outage" } }
      : { json: { ...EVALUATION, feedback } };
  });
  page.route(`**/runs/${RUN}/evaluation/feedback`, ({ body }) => {
    feedbackWrites++;
    feedback = { ...(body as object), submitted_at: "2026-10-16T11:00:00Z" };
    return { json: { ...EVALUATION, feedback } };
  });
  const items = ["s1", "s2"].map((id) => ({
    ...SUGGESTIONS.suggestions[0],
    suggestion_id: id,
    decision: id === "s1" || choice === "rejected" ? "accepted" : "pending",
  }));
  page.route(`**/runs/${RUN}/suggestions`, () => ({
    json: { ...SUGGESTIONS, suggestions: items },
  }));
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const writes: string[] = [];
  let delivered = false;
  page.route("**/suggestions/s2/decision", async ({ body }): Promise<Reply> => {
    const decision = (body as { decision: string }).decision;
    writes.push(decision);
    const first = writes.length === 1;
    if (first) await gate;
    let reply: Reply;
    if (first && outcome === "refused") {
      reply = { status: 404, json: { error: "suggestion not found" } };
    } else {
      items[1].decision = decision;
      reply = { json: first && outcome === "unknown" ? null : items[1] };
    }
    delivered = true;
    return reply;
  });
  return {
    release,
    writes,
    reads: () => reads,
    feedbackWrites: () => feedbackWrites,
    delivered: () => delivered,
  };
}

test("an uncertain version creation survives an evaluation read failure without another write", async () => {
  const page = app();
  const control = evaluationOutage(page, "pending", "accepted");
  let writes = 0;
  page.route(`**/skills/${SKILL}/versions/from-suggestions`, () => {
    writes++;
    return { status: 201, json: null };
  });
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  const apply = page.getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true });
  await apply.click();
  const alert = page.getByRole("alert", { name: "建立版本結果", exact: true });
  await expect(alert).toContainText("無法確認是否已建立新版本");
  await expect(alert).toBeFocused();
  await page.getByRole("button", { name: "有幫助", exact: true }).click();
  const panel = page.locator("#run-decision");
  await expect(panel.getByRole("alert").filter({ hasText: "暫時無法讀取評估結果" })).toBeVisible();
  await expect(alert).toHaveCount(0);
  await expect(apply).toHaveCount(0);
  await panel.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(alert).toContainText("無法確認是否已建立新版本");
  await expect(alert).toBeFocused();
  await expect(
    page.getByRole("button", { name: "重新確認建議套用狀態", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "核對小工具版本", exact: true })).toHaveAttribute(
    "href",
    `/skills/${SKILL}`,
  );
  expect(writes).toBe(1);
  expect(control.reads()).toBe(3);
  expect(control.feedbackWrites()).toBe(1);
  expect(await page.axeViolations("body")).toEqual([]);
});

for (const [outcome, choice] of [
  ["pending", "accepted"],
  ["pending", "rejected"],
  ["unknown", "accepted"],
  ["unknown", "rejected"],
  ["refused", "accepted"],
] as const) {
  test(`${outcome} ${choice} decision survives a feedback-triggered evaluation read failure`, async () => {
    const page = app();
    const control = evaluationOutage(page, outcome, choice);
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const rows = page.locator("li.suggestion");
      await expect(rows).toHaveCount(2);
      const action = choice === "accepted" ? /接受/ : /拒絕/;
      await rows.nth(1).getByRole("button", { name: action }).click();
      await page.eventually(() => expect(control.writes).toEqual([choice]));
      await page.locator("#feedback-comment").fill("保留並送出這次回饋");
      await page.getByRole("button", { name: "有幫助", exact: true }).click();
      const panel = page.locator("#run-decision");
      await expect(
        panel.getByRole("alert").filter({ hasText: "暫時無法讀取評估結果" }),
      ).toBeVisible();
      await expect(rows).toHaveCount(0);
      await expect(panel.locator(".verdict")).toHaveCount(0);
      await expect(panel.getByRole("button", { name: /建立新版本/ })).toHaveCount(0);
      if (outcome !== "pending") {
        control.release();
        await page.eventually(() => expect(control.delivered()).toBe(true));
      }
      await panel.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(rows).toHaveCount(2);
      expect(control.reads()).toBe(3);
      expect(control.feedbackWrites()).toBe(1);
      expect(control.writes).toEqual([choice]);
      await expect(page.locator("#feedback-comment")).toHaveValue("保留並送出這次回饋");
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
      } else {
        const alert = second.getByRole("alert");
        await expect(alert).toContainText(
          outcome === "unknown"
            ? `尚未確認「${choice === "accepted" ? "接受" : "拒絕"}」是否已記錄`
            : "這個決定被拒絕。這個建議已經不在了，請重新讀取這份評估。",
        );
        await expect(alert).toBeFocused();
        if (outcome === "unknown") await expect(apply).toBeDisabled();
        else await expect(apply).toBeEnabled();
      }
      expect(await page.axeViolations("body")).toEqual([]);
      if (outcome === "pending") control.release();
      if (outcome === "unknown") await second.getByRole("button", { name: action }).click();
      if (outcome !== "refused") {
        await expect(second.getByRole("button", { name: action })).toHaveAttribute(
          "aria-pressed",
          "true",
        );
        await expect(second.getByRole("button", { name: action })).toBeEnabled();
        await expect(second.getByRole("alert")).toHaveCount(0);
        await expect(apply).toBeEnabled();
      }
      expect(control.writes).toHaveLength(outcome === "unknown" ? 2 : 1);
    } finally {
      control.release();
      if (control.writes.length)
        await page.eventually(() => expect(control.delivered()).toBe(true));
    }
  });
}
