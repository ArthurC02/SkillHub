import { expect, test } from "vitest";
import { PREFLIGHT, SKILL, TEST_CASE, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

for (const owner of ["version", "test case"] as const) {
  test(`a summary for another ${owner} requires a matching reread before confirmation`, async () => {
    const page = app();
    let reads = 0;
    const queries: Array<{ version: string | null; testCase: string | null }> = [];
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(`**/skills/${SKILL}/runs/preflight?*`, async ({ url }) => {
      queries.push({
        version: url.searchParams.get("version_id"),
        testCase: url.searchParams.get("test_case_id"),
      });
      reads += 1;
      if (reads === 1)
        return {
          json: {
            ...PREFLIGHT,
            summary: {
              ...PREFLIGHT.summary,
              ...(owner === "version"
                ? { skill_version_id: "55555555-5555-4555-8555-555555555555" }
                : { test_case_id: "66666666-6666-4666-8666-666666666666" }),
            },
          },
        };
      await held;
      return { json: PREFLIGHT };
    });
    const writes = () => page.requests.filter((request) => request.method === "POST");
    try {
      await page.goto(`/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}`);
      const main = page.locator("main");
      const confirm = page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true });
      await page.eventually(() => {
        expect(main.locator("[role=alert]").text()).toContain("暫時無法讀取權限摘要");
        expect(confirm.count()).toBe(0);
        expect(main.locator(".preflight-facts").count()).toBe(0);
      });
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
      await main.locator("button").filter({ hasText: "再試一次" }).click();
      await page.eventually(() => expect(reads).toBe(2));
      const heading = page.getByRole("heading", { name: "執行前權限確認", exact: true });
      await page.eventually(() => {
        expect(heading.isFocused()).toBe(true);
        expect(confirm.count()).toBe(0);
      });
      release();
      await page.eventually(() => {
        expect(confirm.isDisabled()).toBe(false);
        expect(main.locator("[role=alert]").count()).toBe(0);
        expect(main.locator(".preflight-facts").text()).toContain("rows.csv");
        expect(page.getByLabel("小工具 Version", { exact: true }).value()).toBe(VERSION);
        expect(heading.isFocused()).toBe(true);
      });
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
      expect(writes()).toEqual([]);
      expect(reads).toBe(2);
      expect(queries).toEqual([
        { version: VERSION, testCase: TEST_CASE },
        { version: VERSION, testCase: TEST_CASE },
      ]);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      release();
    }
  });
}
