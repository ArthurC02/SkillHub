import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  RUNS,
  SKILL_VERSIONS,
  VERSION,
  platformResponse,
} from "../../testing/fixtures/platform";
import { renderedApp, type App, type Locator } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const SECOND = "7e9b2a00-1111-4111-8111-000000000002";
const IDS = [RUN, OTHER_RUN, SECOND];
const OUTPUTS = ["來源試跑的輸出", "候選甲的輸出", "候選乙的輸出"];
const EVALUATIONS = [
  "a1111111-1111-4111-8111-111111111111",
  "a2222222-2222-4222-8222-222222222222",
  "a3333333-3333-4333-8333-333333333333",
];

function setup(page: App, sameSecond: boolean) {
  const dates = [
    "2026-08-16T00:00:01.001Z",
    sameSecond ? "2026-08-16T00:00:01.999Z" : "2026-08-16T00:00:59Z",
  ];
  const original = RUNS.runs.find((run) => run.run_id === OTHER_RUN)!;
  const sourceVersion = SKILL_VERSIONS.versions.find((version) => version.version_number === 1)!;
  const comparisons: string[] = [];
  page.route(
    (url) => url.pathname.startsWith("/runs"),
    ({ url }) => {
      if (url.pathname === "/runs")
        return {
          json: {
            runs: [OTHER_RUN, SECOND].map((id, index) => ({
              ...original,
              run_id: id,
              created_at: dates[index],
            })),
          },
        };
      if (url.pathname.endsWith("/comparison")) {
        comparisons.push(url.searchParams.get("against")!);
        const body = JSON.parse(JSON.stringify(COMPARISON).replaceAll(OTHER_RUN, SECOND));
        body.runs[0].skill_version_id = sourceVersion.version_id;
        body.runs[0].final_output = OUTPUTS[0];
        body.runs[1].skill_version_id = VERSION;
        body.runs[1].final_output = OUTPUTS[2];
        return { json: body };
      }
      const id = url.pathname.match(/^\/runs\/([^/]+)/)?.[1];
      const index = id ? IDS.indexOf(id) : -1;
      if (index < 0) return undefined;
      const response = platformResponse((url.pathname + url.search).replace(id!, RUN));
      const body = JSON.parse(
        JSON.stringify(response.body).replaceAll(RUN, id!).replaceAll("eval-2", EVALUATIONS[index]),
      );
      if (url.pathname.endsWith("/trace")) body.final_output = OUTPUTS[index];
      if (url.pathname === `/runs/${id}`)
        body.skill_version_id = index === 0 ? sourceVersion.version_id : VERSION;
      return { status: response.status, json: body };
    },
  );
  return { dates, comparisons };
}

async function openInNewTab(page: App, link: Locator, keyboard: boolean) {
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener");
  const location = page.pathname() + page.search();
  if (keyboard) {
    await link.focus();
    await link.press("Enter");
  } else await link.click();
  expect(page.pathname() + page.search()).toBe(location);
}

for (const sameSecond of [false, true]) {
  test(`candidate evidence with ${sameSecond ? "same-second" : "distinct-second"} creation preserves comparison intent`, async () => {
    const page = app();
    const state = setup(page, sameSecond);
    const writes = () => page.requests.filter((request) => request.method !== "GET");
    await page.goto(`/runs/${RUN}/compare`);
    const rows = page.locator("main .download-list > .download-item");
    await expect(rows).toHaveCount(2);
    const source = page.getByRole("link", { name: "另開比較基準的試跑結果", exact: true });
    await expect(source).toHaveAttribute("href", `/runs/${RUN}`);
    await expect(rows.getByRole("link", { name: "v2", exact: true })).toHaveCount(2);
    await expect(rows.nth(0).locator("time")).toHaveText(/:01$/);
    await expect(rows.nth(1).locator("time")).toHaveText(sameSecond ? /:01$/ : /:59$/);
    await expect(rows.nth(0).getByRole("button")).toBeVisible();
    for (const [index, id] of [OTHER_RUN, SECOND].entries()) {
      const row = rows.nth(index);
      await expect(row.locator("time")).toHaveAttribute("datetime", state.dates[index]);
      const identity = row.getByText(id, { exact: true });
      await expect(identity).not.toBeVisible();
      await row.getByText("試跑紀錄 ID", { exact: true }).click();
      await expect(identity).toBeVisible();
      await row.getByText("試跑紀錄 ID", { exact: true }).click();
      await expect(identity).not.toBeVisible();
      await expect(row.getByRole("link", { name: /^另開試跑結果/ })).toHaveAttribute(
        "href",
        `/runs/${id}`,
      );
    }
    await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
    const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
    await field.fill("預覽前保留的比較 ID");
    await openInNewTab(page, source, false);
    await expect(field).toHaveValue("預覽前保留的比較 ID");
    expect(state.comparisons).toEqual([]);
    await openInNewTab(page, rows.nth(1).getByRole("link", { name: /^另開試跑結果/ }), true);
    await expect(field).toHaveValue("預覽前保留的比較 ID");
    expect(page.pathname()).toBe(`/runs/${RUN}/compare`);
    expect(page.search()).toBe("");
    expect(state.comparisons).toEqual([]);
    expect(await page.axeViolations("body")).toEqual([]);
    await rows.nth(1).getByRole("button").focus();
    await page.press("Enter");
    const table = page.locator("table").filter({ hasText: "試跑紀錄任務判定與執行狀態對比" });
    await expect(table).toBeVisible();
    await expect(
      table.getByRole("row").filter({ hasText: "最終輸出" }).getByRole("cell"),
    ).toHaveText("來源試跑的輸出 候選乙的輸出");
    expect(page.pathname()).toBe(`/runs/${RUN}/compare`);
    expect(page.search()).toBe(`?against=${SECOND}`);
    expect(state.comparisons).toEqual([SECOND]);
    expect(writes()).toEqual([]);
    expect(await page.axeViolations("body")).toEqual([]);
  });
}
