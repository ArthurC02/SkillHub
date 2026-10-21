import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { RUN, SKILL, SUGGESTIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const VERSION = "99999999-1111-4111-8111-111111111111";
const OTHER_SKILL = "77777777-8888-4888-8888-777777777777";
const REJECTION = {
  suggestion_id: "s2",
  blocked_reason: "target_changed",
  message:
    "README.md is no longer what it was when this suggestion was written, so the change was reasoned about other bytes; re-evaluate the newest version to get a suggestion about it",
};

for (const mode of [
  "partial",
  "uppercase identifiers",
  "foreign Skill",
  "unrequested suggestion",
  "unaccounted suggestion",
]) {
  test(`an apply receipt with ${mode} identifies the requested work before a success claim`, async ({
    page,
  }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    const two = mode === "partial" || mode === "unaccounted suggestion";
    const suggestions = [{ ...SUGGESTIONS.suggestions[0], decision: "accepted" }];
    if (two)
      suggestions.push({
        ...SUGGESTIONS.suggestions[0],
        suggestion_id: "s2",
        decision: "accepted",
        target_path: "README.md",
      });
    let reads = 0;
    await page.route(`**/runs/${RUN}/suggestions`, (route) => {
      reads += 1;
      return route.fulfill({ json: { ...SUGGESTIONS, suggestions } });
    });
    const writes: unknown[] = [];
    await page.route(`**/skills/${SKILL}/versions/from-suggestions`, (route) => {
      writes.push(route.request().postDataJSON());
      return route.fulfill({
        status: 201,
        json: {
          skill_id: mode === "foreign Skill" ? OTHER_SKILL : SKILL,
          version_id: VERSION,
          version_number: 3,
          content_hash: "hash-new",
          duplicate: false,
          applied_suggestion_ids:
            mode === "unrequested suggestion"
              ? ["s99"]
              : mode === "uppercase identifiers"
                ? ["S1"]
                : ["s1"],
          rejected_suggestions: mode === "partial" ? [REJECTION] : [],
        },
      });
    });
    await page.goto(`/runs/${RUN}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ }).click();
    const region = page.getByRole("region", { name: "改善建議", exact: true });
    if (mode === "partial" || mode === "uppercase identifiers") {
      await expect(region.getByRole("status")).toContainText("已建立新版本 #3");
      await expect(
        region.getByRole("link", { name: "開啟剛建立的版本", exact: true }),
      ).toHaveAttribute("href", `/skills/${SKILL}/versions/${VERSION}`);
      await expect(region.getByRole("alert")).toHaveCount(0);
      await expect(region.getByText("已建成版本的建議不能撤回", { exact: true })).toHaveCount(1);
      if (mode === "partial") {
        await expect(region).toContainText("以下建議沒有被套用");
        await expect(region).toContainText(REJECTION.message);
        await expect(region).toContainText("目標檔案已經和建議產生當時不同");
      }
    } else {
      const alert = region.getByRole("alert", { name: "建立版本結果", exact: true });
      await expect(alert).toContainText(
        "無法確認是否已建立新版本。請先核對版本清單與建議套用狀態，再決定是否重試。",
      );
      await expect(alert).toBeFocused();
      await expect.poll(() => reads).toBe(2);
      await expect(region.getByText(/^已建立新版本/)).toHaveCount(0);
      await expect(region.getByRole("link", { name: "開啟剛建立的版本", exact: true })).toHaveCount(
        0,
      );
      await expect(
        region.getByRole("link", { name: "以新版本重跑這個測試題", exact: true }),
      ).toHaveCount(0);
      await expect(region.getByText("已建成版本的建議不能撤回", { exact: true })).toHaveCount(0);
      const retry = region.getByRole("button", { name: "重新確認建議套用狀態", exact: true });
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => reads).toBe(3);
      await expect(retry).toBeEnabled();
      await expect(page.locator("#suggestions-heading")).toBeFocused();
      await expect(alert).toContainText("無法確認是否已建立新版本");
      await expect(
        region.getByRole("link", { name: "核對小工具版本", exact: true }),
      ).toHaveAttribute("href", `/skills/${SKILL}`);
    }
    expect(writes).toEqual([
      { evaluation_id: "eval-2", suggestion_ids: two ? ["s1", "s2"] : ["s1"] },
    ]);
    expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    const audit = await new AxeBuilder({ page }).include("main").analyze();
    await writeFile(info.outputPath("apply-receipt-a11y.json"), JSON.stringify(audit, null, 2));
    expect(audit.violations).toEqual([]);
    expect(audit.incomplete).toEqual([]);
    await region.scrollIntoViewIfNeeded();
    await page.screenshot({ path: info.outputPath("apply-receipt.png") });
  });
}
