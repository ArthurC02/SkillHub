import { expect, test } from "vitest";
import { RUN, SKILL, RUNS, platformResponse } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const VERSION_RECEIPT = "99999999-1111-4111-8111-111111111111";

type Page = ReturnType<typeof app>;

function stubSource(page: Page, status: number) {
  const control = { unavailable: false, reads: [] as string[] };
  page.route(`**/runs/${RUN}`, ({ url }) => {
    control.reads.push(control.unavailable ? "unavailable" : "available");
    return control.unavailable
      ? { status, json: { error: "Run source unavailable" } }
      : {
          json: {
            ...(platformResponse(url.pathname + url.search).body as object),
            status: "failed",
            cleanup_status: RUNS.runs[1].cleanup_status,
          },
        };
  });
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
  return control;
}

for (const status of [403, 503]) {
  for (const phase of ["ready", "applied", "compare"]) {
    test(`${status} Run source recovery preserves ${phase} evidence without stale actions`, async () => {
      const page = app();
      const control = stubSource(page, status);
      const writes = () =>
        page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
      const nav = page.getByRole("navigation", { name: "這個小工具的工作台", exact: true });
      const apply = page.getByRole("button", { name: /^以已接受的 [01] 項建議建立新版本$/ });
      const cleanup = page.locator("#run-trace .badge-row").filter({ hasText: "清理狀態" });
      const rerun = page.getByRole("link", { name: "以新版本重跑這個測試題", exact: true });
      const receipt = page.getByRole("link", { name: "開啟剛建立的版本", exact: true });
      const verdict = page.locator("#run-decision .verdict");
      const note = page.locator("#suggestion-source-note");
      await page.goto(`/runs/${RUN}${phase === "compare" ? "/compare" : ""}`);
      await page.eventually(() => {
        expect(page.locator(".auth-name").text()).toBe("tester");
        expect(nav.count()).toBe(1);
      });
      if (phase === "compare") {
        await nav.getByRole("link", { name: "總覽", exact: true }).click();
        await page.eventually(() => {
          expect(page.pathname()).toBe(`/skills/${SKILL}`);
          expect(page.locator("h1").text()).not.toBe("試跑比較");
        });
        control.unavailable = true;
        await page.goBack();
      } else {
        await page.eventually(() => {
          expect(apply.isDisabled()).toBe(false);
          expect(cleanup.text()).toContain("清理狀態：已清理");
        });
        if (phase === "applied") {
          await apply.click();
          await expect(rerun).toHaveCount(1);
          expect(writes()).toHaveLength(1);
        }
        control.unavailable = true;
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
          expect(cleanup.count()).toBe(0);
          expect(apply.isDisabled()).toBe(true);
          expect(apply.attribute("aria-describedby")).toBe("suggestion-source-note");
          expect(note.text()).toContain("試跑來源尚未確認，暫時不能建立新版本。");
          expect(page.getByText("正在讀取這次試跑屬於哪個小工具…", { exact: true }).count()).toBe(
            0,
          );
          expect(rerun.count()).toBe(0);
          expect(page.getByText("這次試跑的測試題草稿已不存在").count()).toBe(0);
        });
        if (phase === "applied") {
          await page.eventually(() => {
            expect(receipt.attribute("href")).toBe(`/skills/${SKILL}/versions/${VERSION_RECEIPT}`);
            expect(page.getByText("已建立新版本 #3", { exact: true }).count()).toBe(1);
            expect(
              page.getByText("試跑來源尚未確認，暫時無法提供相同測試題的重跑入口").count(),
            ).toBe(1);
          });
        }
      }
      expect(await page.axeViolations("main")).toEqual([]);
      if (phase !== "compare") {
        const reconfirm = page.getByRole("link", { name: "重新確認試跑來源", exact: true });
        expect(reconfirm.attribute("href")).toBe("#run-heading");
        await expect(page.locator("#run-heading")).toHaveCount(1);
      }
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await expect(retry).toBeFocused();
      control.unavailable = false;
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
          expect(cleanup.text()).toContain("清理狀態：已清理");
          expect(apply.isDisabled()).toBe(phase === "applied");
          expect(verdict.text()).toBe("任務判定：未符合");
          expect(note.count()).toBe(0);
          if (phase === "applied") {
            expect(rerun.count()).toBe(1);
            expect(receipt.count()).toBe(1);
          }
        });
      }
      expect(await page.axeViolations("main")).toEqual([]);
      expect(control.reads).toContain("unavailable");
      expect(control.reads.at(-1)).toBe("available");
      expect(writes().map((request) => request.url.pathname)).toEqual(
        phase === "applied" ? [`/skills/${SKILL}/versions/from-suggestions`] : [],
      );
    });
  }
}
