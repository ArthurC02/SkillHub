import { expect, test } from "vitest";
import { COMPARISON, OTHER_RUN, RUN, RUNS } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const SECOND = "7e9b2a00-1111-4111-8111-000000000002";
const OTHER = RUNS.runs.find((run) => run.run_id === OTHER_RUN)!;
const SOURCE = RUNS.runs.find((run) => run.run_id === RUN)!;

function writes(page: ReturnType<typeof app>) {
  return page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
}

for (const fault of ["missing-run-id", "null-verdict"] as const) {
  for (const continuation of [false, true]) {
    test(`comparison recovers ${fault} on ${continuation ? "a continuation page" : "first read"} without losing its draft`, async () => {
      const page = app();
      let release!: () => void;
      const held = new Promise<void>((resolve) => (release = resolve));
      let corrupt = true;
      const offsets: number[] = [];
      const comparisons: string[] = [];
      page.route("**/runs?*", async ({ url }) => {
        const params = url.searchParams;
        expect(params.get("test_case_id")).toBe(SOURCE.test_case_id);
        expect(params.get("limit")).toBe("51");
        const offset = Number(params.get("offset"));
        offsets.push(offset);
        if (continuation && offset === 0)
          return { json: { runs: [OTHER, ...Array.from({ length: 50 }, () => SOURCE)] } };
        await held;
        const valid = { ...OTHER, run_id: continuation ? SECOND : OTHER_RUN };
        const invalid =
          fault === "missing-run-id"
            ? { ...valid, run_id: undefined }
            : { ...valid, evaluation: null };
        return { json: { runs: [corrupt ? invalid : valid] } };
      });
      page.route(`**/runs/${RUN}/comparison?*`, ({ url }) => {
        const against = url.searchParams.get("against")!;
        comparisons.push(against);
        return { json: JSON.parse(JSON.stringify(COMPARISON).replaceAll(OTHER_RUN, against)) };
      });
      try {
        await page.goto(`/runs/${RUN}/compare`);
        await page.eventually(() =>
          expect(page.getByText("直接輸入試跑紀錄 ID", { exact: true }).count()).toBe(1),
        );
        await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
        const draft = page.getByLabel("要比較的另一個試跑紀錄 ID");
        await draft.fill("清單恢復前保留的比較 ID");
        const rows = page.locator("main .download-list > .download-item");
        const loadMore = page.getByRole("button", { name: "載入更早的試跑紀錄", exact: true });
        if (continuation) {
          await page.eventually(() => expect(rows.count()).toBe(1));
          await loadMore.click();
        }
        await page.eventually(() => expect(offsets).toEqual(continuation ? [0, 50] : [0]));
        release();
        const alert = page.getByRole("alert").filter({ hasText: "暫時無法讀取可比較的試跑紀錄" });
        await page.eventually(() => {
          expect(alert.count()).toBe(1);
          expect(page.getByRole("heading", { name: "試跑比較", exact: true }).count()).toBe(1);
          expect(draft.value()).toBe("清單恢復前保留的比較 ID");
          expect(rows.count()).toBe(0);
          expect(page.getByText("這個測試題目前只有這一次試跑紀錄", { exact: false }).count()).toBe(
            0,
          );
          expect(page.getByText("這一頁暫時無法顯示", { exact: true }).count()).toBe(0);
          expect(
            page
              .getByRole("link", { name: "另開比較基準的試跑結果", exact: true })
              .attribute("href"),
          ).toBe(`/runs/${RUN}`);
        });
        expect(comparisons).toEqual([]);
        expect(await page.axeViolations("body")).toEqual([]);
        corrupt = false;
        const retry = page.locator("main button").filter({ hasText: "再試一次" });
        expect(retry.count()).toBe(1);
        await retry.focus();
        await page.press("Enter");
        await page.eventually(() => {
          expect(rows.count()).toBe(1);
          expect(alert.count()).toBe(0);
          expect(page.locator("#run-comparison-heading").isFocused()).toBe(true);
          expect(draft.value()).toBe("清單恢復前保留的比較 ID");
        });
        if (continuation) {
          await loadMore.click();
          await page.eventually(() => expect(rows.count()).toBe(2));
        }
        expect(offsets).toEqual(continuation ? [0, 50, 0, 50] : [0, 0]);
        expect(await page.axeViolations("body")).toEqual([]);
        const last = rows.nth(rows.count() - 1).locator("button");
        await last.focus();
        await page.press("Enter");
        const against = continuation ? SECOND : OTHER_RUN;
        await page.eventually(() => {
          expect(page.pathname() + page.search()).toBe(`/runs/${RUN}/compare?against=${against}`);
          expect(
            page.locator("table").filter({ hasText: "試跑紀錄任務判定與執行狀態對比" }).count(),
          ).toBe(1);
        });
        expect(comparisons).toEqual([against]);
        expect(writes(page)).toEqual([]);
      } finally {
        release();
      }
    });
  }
}

for (const target of [
  {
    path: "/workspace",
    heading: "繼續推進你的工作",
    what: "需要處理的試跑",
    focus: "workspace-attention-heading",
  },
  {
    path: "/workspace/runs",
    heading: "試跑活動",
    what: "試跑活動",
    focus: "workspace-runs-heading",
  },
]) {
  test(`${target.path} recovers an unreadable Run verdict locally`, async () => {
    const page = app();
    let corrupt = true;
    let reads = 0;
    page.route("**/runs?*", () => {
      reads += 1;
      return { json: { runs: [{ ...OTHER, evaluation: corrupt ? null : OTHER.evaluation }] } };
    });
    await page.goto(target.path);
    const alert = page.getByRole("alert").filter({ hasText: `暫時無法讀取${target.what}` });
    await page.eventually(() => {
      expect(alert.count()).toBe(1);
      expect(page.getByRole("heading", { name: target.heading, exact: true }).count()).toBe(1);
      expect(page.getByText("這一頁暫時無法顯示", { exact: true }).count()).toBe(0);
    });
    expect(await page.axeViolations("body")).toEqual([]);
    corrupt = false;
    const retry = page.locator("main button").filter({ hasText: "再試一次" });
    expect(retry.count()).toBe(1);
    await retry.focus();
    await page.press("Enter");
    await page.eventually(() => {
      expect(alert.count()).toBe(0);
      expect(page.locator(`#${target.focus}`).isFocused()).toBe(true);
      expect(page.locator("main").text()).toContain("任務判定：不符合");
    });
    expect(reads).toBe(2);
    expect(writes(page)).toEqual([]);
    expect(await page.axeViolations("body")).toEqual([]);
  });
}
