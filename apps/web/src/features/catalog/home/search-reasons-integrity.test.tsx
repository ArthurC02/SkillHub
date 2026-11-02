import { afterAll, beforeAll, expect, test } from "vitest";
import { SEARCH, SKILL, SKILL_B } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});

afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const MODEL_REASON = "模型判斷這個版本不符合任務需要的格式。";
const VALID_REASON = { reasons: [{ skill_id: SKILL, reason: MODEL_REASON, fits: false }] };
const CASES = [
  { name: "null item", reply: { reasons: [null] }, invalid: true },
  {
    name: "invalid verdict",
    reply: { reasons: [{ skill_id: SKILL, reason: "不應採用的模型說明", fits: "false" }] },
    invalid: true,
  },
  { name: "missing list", reply: { results: [] }, invalid: true },
  { name: "valid negative", reply: VALID_REASON, invalid: false },
];

for (const scenario of CASES) {
  test(`optional reasons ${scenario.name} retain search decisions`, async () => {
    const page = app();
    const errors: string[] = [];
    const recordError = (event: ErrorEvent) => errors.push(event.message);
    window.addEventListener("error", recordError);
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const reasonRequests = () =>
      page.requests.filter((request) => request.url.pathname === "/api/skills/search/reasons");
    page.route("**/api/skills/search/reasons", async () => {
      const attempt = reasonRequests().length;
      if (attempt === 1) await held;
      return { json: attempt === 1 ? scenario.reply : VALID_REASON };
    });
    try {
      await page.goto(`/?${new URLSearchParams({ q: SEARCH.query })}`);
      const main = page.locator("main");
      const rows = main.locator(".search-results > li");
      await page.eventually(() => {
        expect(rows.count()).toBe(2);
        expect(reasonRequests()).toHaveLength(1);
      });
      expect(reasonRequests()[0].body).toEqual({
        query: "pdf 摘要",
        skill_ids: [SKILL, SKILL_B],
      });
      await rows.first().getByRole("checkbox").check();
      const input = page.getByLabel("任務描述", { exact: true });
      await input.fill("尚未送出的搜尋草稿");
      const selectedURL = page.pathname() + page.search();
      release();
      if (!scenario.invalid) await expect(rows.first().getByText(MODEL_REASON)).toHaveCount(1);
      await page.eventually(() => {
        expect(rows.count()).toBe(2);
        expect(input.value()).toBe("尚未送出的搜尋草稿");
        expect(input.isFocused()).toBe(true);
        expect(rows.first().getByRole("checkbox").element()).toHaveProperty("checked", true);
      });
      expect(page.pathname() + page.search()).toBe(selectedURL);
      if (scenario.invalid) {
        await expect(rows.locator(".match-reason .badge-source-model")).toHaveCount(0);
        await page.locator('a[href="/policy"]').first().click();
        await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(
          1,
        );
        await page.goBack();
        await page.eventually(() => expect(reasonRequests()).toHaveLength(2));
        expect(reasonRequests()[1].body).toEqual(reasonRequests()[0].body);
      }
      await page.eventually(() => {
        expect(rows.first().locator(".match-reason").text()).toContain(MODEL_REASON);
        expect(rows.first().locator(".match-reason .badge-warning").text()).toBe("可能不合用");
        expect(rows.nth(1).locator(".match-reason").text()).toContain("查詢與文件共同出現：pdf");
        expect(rows.locator(".result-card-head > a").text()).toBe("PDF Summariser Doc Splitter");
        expect(rows.first().getByRole("checkbox").element()).toHaveProperty("checked", true);
        expect(main.getByRole("alert").count()).toBe(0);
      });
      expect(errors).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      release();
      window.removeEventListener("error", recordError);
    }
  });
}
