import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RUN, SKILL, SUGGESTIONS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

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
  test(`an apply receipt with ${mode} identifies the requested work before a success claim`, async () => {
    const page = app();
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
    page.route(`**/runs/${RUN}/suggestions`, () => {
      reads += 1;
      return { json: { ...SUGGESTIONS, suggestions } };
    });
    const writes: unknown[] = [];
    page.route(`**/skills/${SKILL}/versions/from-suggestions`, ({ body }) => {
      writes.push(body);
      return {
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
      };
    });
    await page.goto(`/runs/${RUN}`);
    await page.eventually(() => expect(page.locator(".auth-name").text()).toBe("tester"));
    const apply = page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ });
    await page.eventually(() => expect(apply.count()).toBe(1));
    await apply.click();
    const region = page.getByRole("region", { name: "改善建議", exact: true });
    if (mode === "partial" || mode === "uppercase identifiers") {
      await page.eventually(() =>
        expect(region.getByRole("status").text()).toContain("已建立新版本 #3"),
      );
      expect(
        region.getByRole("link", { name: "開啟剛建立的版本", exact: true }).attribute("href"),
      ).toBe(`/skills/${SKILL}/versions/${VERSION}`);
      expect(region.getByRole("alert").count()).toBe(0);
      expect(region.getByText("已建成版本的建議不能撤回", { exact: true }).count()).toBe(1);
      if (mode === "partial") {
        expect(region.text()).toContain("以下建議沒有被套用");
        expect(region.text()).toContain(REJECTION.message);
        expect(region.text()).toContain("目標檔案已經和建議產生當時不同");
      }
    } else {
      const alert = region.getByRole("alert", { name: "建立版本結果", exact: true });
      await page.eventually(() =>
        expect(alert.text()).toContain(
          "無法確認是否已建立新版本。請先核對版本清單與建議套用狀態，再決定是否重試。",
        ),
      );
      expect(alert.isFocused()).toBe(true);
      await page.eventually(() => expect(reads).toBe(2));
      expect(region.getByText(/^已建立新版本/).count()).toBe(0);
      expect(region.getByRole("link", { name: "開啟剛建立的版本", exact: true }).count()).toBe(0);
      expect(
        region.getByRole("link", { name: "以新版本重跑這個測試題", exact: true }).count(),
      ).toBe(0);
      expect(region.getByText("已建成版本的建議不能撤回", { exact: true }).count()).toBe(0);
      const retry = region.getByRole("button", { name: "重新確認建議套用狀態", exact: true });
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() => expect(reads).toBe(3));
      await page.eventually(() => expect(retry.isDisabled()).toBe(false));
      await page.eventually(() =>
        expect(page.locator("#suggestions-heading").isFocused()).toBe(true),
      );
      expect(alert.text()).toContain("無法確認是否已建立新版本");
      expect(
        region.getByRole("link", { name: "核對小工具版本", exact: true }).attribute("href"),
      ).toBe(`/skills/${SKILL}`);
    }
    expect(writes).toEqual([
      { evaluation_id: "eval-2", suggestion_ids: two ? ["s1", "s2"] : ["s1"] },
    ]);
    expect(page.pathname()).toBe(`/runs/${RUN}`);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
