import { expect, test } from "vitest";
import { RUN, OTHER_RUN, SKILL, RUNS, platformResponse } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const VERSION_RECEIPT = "99999999-1111-4111-8111-111111111111";

const source = {
  ...(platformResponse(`http://platform.test/runs/${RUN}`).body as object),
  status: "failed",
  cleanup_status: RUNS.runs[1].cleanup_status,
};

function stubSource(page: ReturnType<typeof app>, altered: unknown, readable: boolean) {
  const control = { readable, reads: [] as string[] };
  page.route(`**/runs/${RUN}`, () => {
    control.reads.push(control.readable ? "source" : "altered");
    return { json: control.readable ? source : altered };
  });
  return control;
}

function writesOf(page: ReturnType<typeof app>) {
  return page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
}

for (const { name, body } of [
  { name: "null", body: null },
  { name: "foreign", body: { ...source, run_id: OTHER_RUN } },
]) {
  for (const phase of ["initial", "applied", "compare"]) {
    test(`a ${name} Run source in ${phase} recovers without substituting or replaying evidence`, async () => {
      const page = app();
      const control = stubSource(page, body, phase !== "initial");
      const nav = page.getByRole("navigation", { name: "這個小工具的工作台", exact: true });
      const apply = page.getByRole("button", { name: /^以已接受的 [01] 項建議建立新版本$/ });
      const rerun = page.getByRole("link", { name: "以新版本重跑這個測試題", exact: true });
      const receipt = page.getByRole("link", { name: "開啟剛建立的版本", exact: true });
      const cleanup = page.locator("#run-trace .badge-row").filter({ hasText: "清理狀態" });
      const verdict = page.locator("#run-decision .verdict");
      await page.goto(`/runs/${RUN}${phase === "compare" ? "/compare" : ""}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      if (phase === "compare") {
        await expect(nav).toHaveCount(1);
        await nav.getByRole("link", { name: "總覽", exact: true }).click();
        await expect(page.locator("h1")).not.toHaveText("試跑比較");
        control.readable = false;
        await page.goBack();
      } else if (phase === "applied") {
        await expect(apply).toBeEnabled();
        page.route(`**/skills/${SKILL}/versions/from-suggestions`, () => ({
          status: 201,
          json: {
            skill_id: SKILL,
            version_id: VERSION_RECEIPT,
            version_number: 3,
            content_hash: "hash-new",
            duplicate: false,
            applied_suggestion_ids: ["s1"],
            rejected_suggestions: [],
          },
        }));
        await apply.click();
        await expect(rerun).toHaveCount(1);
        control.readable = false;
        await page.getByRole("button", { name: "進階模式", exact: true }).click();
        await page.getByRole("button", { name: "一般模式", exact: true }).click();
      }
      await page.eventually(() => {
        expect(page.getByRole("alert").text()).toBe(
          `暫時無法讀取${phase === "compare" ? "目前這次試跑" : "這次試跑"}。請再試一次，或稍後再試。`,
        );
        expect(nav.count()).toBe(0);
      });
      if (phase !== "compare") {
        await page.eventually(() => {
          expect(verdict.text()).toBe("任務判定：未符合");
          expect(apply.isDisabled()).toBe(true);
          expect(page.locator("#suggestion-source-note").text()).toContain("試跑來源尚未確認");
          expect(page.getByText("正在讀取這次試跑屬於哪個小工具…", { exact: true }).count()).toBe(
            0,
          );
          expect(cleanup.count()).toBe(0);
          expect(rerun.count()).toBe(0);
          expect(page.getByText("這次試跑的測試題草稿已不存在").count()).toBe(0);
          if (phase === "applied") {
            expect(receipt.attribute("href")).toBe(`/skills/${SKILL}/versions/${VERSION_RECEIPT}`);
            expect(page.getByText("已建立新版本 #3", { exact: true }).count()).toBe(1);
          }
        });
      }
      expect(await page.axeViolations("main")).toEqual([]);
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await expect(retry).toBeFocused();
      control.readable = true;
      await page.press("Enter");
      await page.eventually(() => {
        expect(page.getByRole("alert").count()).toBe(0);
        expect(
          page
            .locator(phase === "compare" ? "#run-comparison-heading" : "#run-heading")
            .isFocused(),
        ).toBe(true);
        expect(nav.count()).toBe(1);
      });
      if (phase !== "compare") {
        await page.eventually(() => {
          expect(apply.isDisabled()).toBe(phase === "applied");
          expect(cleanup.text()).toContain("清理狀態：已清理");
          expect(verdict.text()).toBe("任務判定：未符合");
          if (phase === "applied") expect(rerun.count()).toBe(1);
        });
      }
      expect(await page.axeViolations("main")).toEqual([]);
      expect(control.reads).toContain("altered");
      expect(control.reads.at(-1)).toBe("source");
      expect(writesOf(page).map((request) => request.url.pathname)).toEqual(
        phase === "applied" ? [`/skills/${SKILL}/versions/from-suggestions`] : [],
      );
    });
  }
}

test("a case-equivalent Run source remains readable with its optional draft absent", async () => {
  const page = app();
  stubSource(page, { ...source, run_id: RUN.toUpperCase(), test_case_id: undefined }, false);
  await page.goto(`/runs/${RUN}`);
  await page.eventually(() => {
    expect(page.locator(".auth-name").text()).toBe("tester");
    expect(page.getByRole("navigation", { name: "這次試跑的來源", exact: true }).count()).toBe(1);
    expect(
      page.getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true }).isDisabled(),
    ).toBe(false);
    expect(page.locator("#run-decision .verdict").text()).toBe("任務判定：未符合");
    expect(page.getByRole("alert").count()).toBe(0);
  });
  expect(await page.axeViolations("main")).toEqual([]);
  expect(writesOf(page)).toEqual([]);
});
