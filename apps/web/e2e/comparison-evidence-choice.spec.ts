import AxeBuilder from "@axe-core/playwright";
import { expect, test, type BrowserContext, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  RUNS,
  SKILL_VERSIONS,
  VERSION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { STATIC_KINDS, serveStatic } from "./stub";

const SECOND = "7e9b2a00-1111-4111-8111-000000000002";
const IDS = [RUN, OTHER_RUN, SECOND];
const OUTPUTS = ["來源試跑的輸出", "候選甲的輸出", "候選乙的輸出"];
const EVALUATIONS = [
  "a1111111-1111-4111-8111-111111111111",
  "a2222222-2222-4222-8222-222222222222",
  "a3333333-3333-4333-8333-333333333333",
];

async function setup(context: BrowserContext, sameSecond: boolean) {
  const dates = [
    "2026-08-16T00:00:01.001Z",
    sameSecond ? "2026-08-16T00:00:01.999Z" : "2026-08-16T00:00:59Z",
  ];
  const original = RUNS.runs.find((run) => run.run_id === OTHER_RUN)!;
  const sourceVersion = SKILL_VERSIONS.versions.find((version) => version.version_number === 1)!;
  const comparisons: string[] = [];
  const writes: string[] = [];
  context.on("request", (request) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
  });
  await context.route("**/*", (route, request) => {
    if (STATIC_KINDS.has(request.resourceType())) return serveStatic(route);
    const url = new URL(request.url());
    if (url.pathname === "/runs")
      return route.fulfill({
        json: {
          runs: [OTHER_RUN, SECOND].map((id, index) => ({
            ...original,
            run_id: id,
            created_at: dates[index],
          })),
        },
      });
    if (url.pathname.endsWith("/comparison")) {
      comparisons.push(url.searchParams.get("against")!);
      const body = JSON.parse(JSON.stringify(COMPARISON).replaceAll(OTHER_RUN, SECOND));
      body.runs[0].skill_version_id = sourceVersion.version_id;
      body.runs[0].final_output = OUTPUTS[0];
      body.runs[1].skill_version_id = VERSION;
      body.runs[1].final_output = OUTPUTS[2];
      return route.fulfill({ json: body });
    }
    const id = url.pathname.match(/^\/runs\/([^/]+)/)?.[1];
    const index = id ? IDS.indexOf(id) : -1;
    const response = platformResponse(index >= 0 ? request.url().replace(id!, RUN) : request.url());
    if (index < 0) return route.fulfill({ status: response.status, json: response.body });
    const body = JSON.parse(
      JSON.stringify(response.body).replaceAll(RUN, id!).replaceAll("eval-2", EVALUATIONS[index]),
    );
    if (url.pathname.endsWith("/trace")) body.final_output = OUTPUTS[index];
    if (url.pathname === `/runs/${id}`)
      body.skill_version_id = index === 0 ? sourceVersion.version_id : VERSION;
    return route.fulfill({ status: response.status, json: body });
  });
  return { dates, comparisons, writes };
}

async function inspect(context: BrowserContext, link: Locator, output: string, keyboard: boolean) {
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener");
  if (keyboard) await link.focus();
  const [popup] = await Promise.all([
    context.waitForEvent("page"),
    keyboard ? link.press("Enter") : link.click(),
  ]);
  try {
    await expect(popup.getByRole("heading", { level: 1 })).toContainText("試跑");
    await expect(popup.getByText(output, { exact: true })).toBeVisible();
    expect(await popup.evaluate(() => window.opener === null)).toBe(true);
  } finally {
    await popup.close();
  }
}

async function capture(page: Page, path: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(path, JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
}

for (const sameSecond of [false, true]) {
  for (const width of [375, 1280]) {
    test(`candidate evidence with ${sameSecond ? "same-second" : "distinct-second"} creation preserves comparison intent at ${width}px`, async ({
      page,
      context,
    }, info) => {
      const state = await setup(context, sameSecond);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      await page.goto(`/runs/${RUN}/compare`);
      const rows = page.locator("main .download-list > .download-item");
      await expect(rows).toHaveCount(2);
      const source = page.getByRole("link", { name: "另開比較基準的試跑結果", exact: true });
      await expect(source).toHaveAttribute("href", `/runs/${RUN}`);
      await expect(source).toBeInViewport({ ratio: 1 });
      await expect(rows.getByRole("link", { name: "v2", exact: true })).toHaveCount(2);
      await expect(rows.nth(0).locator("time")).toHaveText(/:01$/);
      await expect(rows.nth(1).locator("time")).toHaveText(sameSecond ? /:01$/ : /:59$/);
      await expect(rows.nth(0).getByRole("button")).toBeInViewport({ ratio: 1 });
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
      await inspect(context, source, OUTPUTS[0], width === 1280);
      await expect(field).toHaveValue("預覽前保留的比較 ID");
      expect(state.comparisons).toEqual([]);
      await inspect(
        context,
        rows.nth(1).getByRole("link", { name: /^另開試跑結果/ }),
        OUTPUTS[2],
        width === 1280,
      );
      await expect(field).toHaveValue("預覽前保留的比較 ID");
      await expect(page).toHaveURL(`/runs/${RUN}/compare`);
      expect(state.comparisons).toEqual([]);
      await capture(page, info.outputPath("before-choice-a11y.json"));
      await page.screenshot({ path: info.outputPath("candidate-evidence.png") });
      await rows.nth(1).getByRole("button").focus();
      await page.keyboard.press("Enter");
      const table = page.getByRole("table", {
        name: "試跑紀錄任務判定與執行狀態對比",
        exact: true,
      });
      await expect(table).toBeVisible();
      await expect(
        table.getByRole("row").filter({ hasText: "最終輸出" }).getByRole("cell"),
      ).toHaveText(["來源試跑的輸出", "候選乙的輸出"]);
      await expect(page).toHaveURL(`/runs/${RUN}/compare?against=${SECOND}`);
      expect(state.comparisons).toEqual([SECOND]);
      expect(state.writes).toEqual([]);
      await capture(page, info.outputPath("after-choice-a11y.json"));
    });
  }
}
