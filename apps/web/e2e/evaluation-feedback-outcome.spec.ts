import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { EVALUATION, ME, RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

for (const [fault, helpful, unknown] of [
  ["server error", true, true],
  ["lost reply", false, true],
  ["truncated reply", true, true],
  ["rejected input", false, false],
  ["missing verdict", true, false],
  ["pending session", false, false],
] as const) {
  test(`feedback with ${fault} preserves the draft and reports its actual submission certainty`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await stubPlatform(page);
    let holdSession = false;
    let sessionReads = 0;
    let release!: () => void;
    const sessionGate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route("**/me", async (route) => {
      sessionReads++;
      if (holdSession) await sessionGate;
      return route.fulfill({ json: ME });
    });
    let saved: { helpful: boolean; comment: string; submitted_at: string } | undefined;
    let reads = 0;
    const writes: { helpful: boolean; comment: string }[] = [];
    await page.route(`**/runs/${RUN}/evaluation`, (route) => {
      reads++;
      return route.fulfill({ json: { ...EVALUATION, feedback: saved } });
    });
    await page.route(`**/runs/${RUN}/evaluation/feedback`, (route) => {
      const intent = route.request().postDataJSON();
      writes.push(intent);
      if (unknown || writes.length > 1 || fault === "pending session") {
        saved = { ...intent, submitted_at: "2026-10-16T00:00:00Z" };
      }
      if (writes.length === 1 && fault !== "pending session") {
        if (fault === "lost reply") return route.abort("failed");
        if (fault === "truncated reply") {
          return route.fulfill({ status: 200, contentType: "application/json", body: "{" });
        }
        return route.fulfill({
          status: fault === "server error" ? 500 : fault === "missing verdict" ? 404 : 400,
          json: { error: "controlled feedback response" },
        });
      }
      return route.fulfill({ json: { ...EVALUATION, feedback: saved } });
    });
    try {
      await page.goto(`/runs/${RUN}`);
      const comment = page.locator("#feedback-comment");
      await comment.fill("這份判定需要補充證據");
      const main = page.getByRole("main");
      const submit = main.getByRole("button", {
        name: helpful ? "有幫助" : "沒幫助",
        exact: true,
      });
      if (fault === "pending session") {
        holdSession = true;
        const previousReads = sessionReads;
        await page.evaluate(() => {
          document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
        });
        await expect.poll(() => sessionReads).toBe(previousReads + 1);
      }
      await submit.click();
      const alert = main.getByRole("alert");
      await expect(alert).toContainText(
        unknown
          ? "尚未確認回饋是否已儲存。補充說明已保留，可以再按一次。"
          : fault === "missing verdict"
            ? "這次試跑目前沒有可以附回饋的判定。"
            : fault === "pending session"
              ? "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。"
              : "回饋沒有送出，可以再按一次。",
      );
      await expect(main.getByText("已送出回饋。", { exact: true })).toHaveCount(0);
      await expect(comment).toHaveValue("這份判定需要補充證據");
      await expect(comment).not.toHaveAttribute("readonly", "");
      expect(writes).toHaveLength(fault === "pending session" ? 0 : 1);
      expect(reads).toBe(1);
      if (unknown) expect(saved).toMatchObject({ helpful, comment: "這份判定需要補充證據" });
      else expect(saved).toBeUndefined();
      const report = await new AxeBuilder({ page }).analyze();
      await writeFile(test.info().outputPath("feedback-outcome-a11y.json"), JSON.stringify(report));
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await alert.scrollIntoViewIfNeeded();
      await page.screenshot({ path: test.info().outputPath("feedback-outcome-viewport.png") });
      holdSession = false;
      release();
      await expect(page.getByRole("banner").getByRole("status")).toHaveCount(0);
      await submit.click();
      await expect(main.getByText("已送出回饋。", { exact: true })).toBeVisible();
      await expect(alert).toHaveCount(0);
      expect(writes).toHaveLength(fault === "pending session" ? 1 : 2);
      expect(writes.every((intent) => intent.helpful === helpful)).toBe(true);
      expect(writes.every((intent) => intent.comment === "這份判定需要補充證據")).toBe(true);
      expect(saved).toMatchObject({ helpful, comment: "這份判定需要補充證據" });
      expect(reads).toBe(2);
    } finally {
      release();
    }
  });
}
