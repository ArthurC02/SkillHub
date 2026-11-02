import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RUN, SKILL, platformResponse } from "../../testing/fixtures/platform";
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

const NEW_VERSION = "99999999-1111-4111-8111-111111111111";

for (const hasTestCase of [true, false]) {
  for (const leavesForm of [false, true]) {
    test(`an applied ${hasTestCase ? "rerun" : "version"} continuation ${leavesForm ? "preserves header focus" : "reveals the next action"}`, async () => {
      const page = app();
      page.route(`**/runs/${RUN}`, ({ url }) => ({
        json: {
          ...(platformResponse(url.pathname + url.search).body as object),
          ...(!hasTestCase ? { test_case_id: undefined } : {}),
        },
      }));
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      const writes: unknown[] = [];
      page.route(`**/skills/${SKILL}/versions/from-suggestions`, async ({ body }) => {
        writes.push(body);
        await held;
        return {
          status: 201,
          json: {
            skill_id: SKILL,
            version_id: NEW_VERSION,
            version_number: 3,
            content_hash: "sha256:cccc",
            duplicate: false,
            applied_suggestion_ids: ["s1"],
            rejected_suggestions: [],
          },
        };
      });
      try {
        await page.goto(`/runs/${RUN}`);
        const apply = page.getByRole("button", {
          name: "以已接受的 1 項建議建立新版本",
          exact: true,
        });
        await page.eventually(() => expect(apply.count()).toBe(1));
        await page.eventually(() => expect(apply.isDisabled()).toBe(false));
        await apply.focus();
        await apply.click();
        await page.eventually(() => expect(writes).toHaveLength(1));
        const search = page.locator("header input[type=search]");
        if (leavesForm) await search.fill("保留目前搜尋草稿");
        release();
        const continuation = page.getByRole("link", {
          name: hasTestCase ? "以新版本重跑這個測試題" : "開啟剛建立的版本",
          exact: true,
        });
        await page.eventually(() =>
          expect(
            page
              .getByRole("button", { name: "以已接受的 0 項建議建立新版本", exact: true })
              .isDisabled(),
          ).toBe(true),
        );
        if (leavesForm) {
          await page.eventually(() => expect(search.isFocused()).toBe(true));
          expect(search.value()).toBe("保留目前搜尋草稿");
        } else {
          await page.eventually(() => expect(continuation.isFocused()).toBe(true));
        }
        expect(writes).toEqual([{ evaluation_id: "eval-2", suggestion_ids: ["s1"] }]);
        const section = document.querySelector('section[aria-labelledby="suggestions-heading"]');
        expect(section).not.toBeNull();
        expect(await page.axeViolations('section[aria-labelledby="suggestions-heading"]')).toEqual(
          [],
        );
      } finally {
        release();
      }
    });
  }
}
