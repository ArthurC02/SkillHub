import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { RUN, TRACE_GENERAL, ACTIVITY } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const NEXT = "99999999-1111-4111-8111-111111111111";
const ASKING = {
  ...TRACE_GENERAL,
  status: "succeeded",
  questions: [
    { question: "要處理哪個工作表？", why: "確認資料", options: ["Sheet1", "Sheet2"] },
    { question: "重複列要保留哪一筆？", why: "確認去重方向" },
  ],
};
async function openQuestions(page: Page, runId = RUN) {
  await page.goto(`/runs/${runId}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  return page.getByRole("region", { name: "這次試跑需要你回答", exact: true });
}
async function fillAnswers(page: Page) {
  const region = page.getByRole("region", { name: "這次試跑需要你回答", exact: true });
  await region.getByRole("radio", { name: "Sheet2", exact: true }).check();
  await region
    .getByRole("textbox", { name: "你的回答", exact: true })
    .nth(1)
    .fill("保留最新的一筆");
  return region;
}
async function verifyFocus(page: Page) {
  const heading = page.getByRole("heading", { name: "這次試跑需要你回答", exact: true });
  await expect(heading).toBeFocused();
  const position = await heading.evaluate((element) => ({
    top: element.getBoundingClientRect().top,
    headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
  }));
  expect(position.top).toBeGreaterThanOrEqual(position.headerBottom);
}
async function askingReads(page: Page) {
  await page.route(`**/runs/${RUN}/trace*`, (route) =>
    new URL(route.request().url()).searchParams.get("mode") === "advanced"
      ? route.fallback()
      : route.fulfill({ json: ASKING }),
  );
}

test("switching trace detail preserves selected and typed answers without sending them", async ({
  page,
}) => {
  await stubPlatform(page);
  await askingReads(page);
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST") writes.push(request.url());
  });
  await openQuestions(page);
  const region = await fillAnswers(page);
  await page.getByRole("button", { name: "進階模式", exact: true }).click();
  await expect(page.getByRole("button", { name: "進階模式", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(region).toBeVisible();
  await expect(region.getByRole("radio", { name: "Sheet2", exact: true })).toBeChecked();
  await expect(region.getByRole("textbox", { name: "你的回答", exact: true }).nth(1)).toHaveValue(
    "保留最新的一筆",
  );
  await region.getByRole("textbox", { name: "你的回答", exact: true }).nth(0).fill("Sheet3");
  await page.getByRole("button", { name: "一般模式", exact: true }).click();
  await expect(region.getByRole("textbox", { name: "你的回答", exact: true }).nth(0)).toHaveValue(
    "Sheet3",
  );
  await expect(region.getByRole("textbox", { name: "你的回答", exact: true }).nth(1)).toHaveValue(
    "保留最新的一筆",
  );
  expect(writes).toEqual([]);
});

for (const mode of ["一般模式", "進階模式"]) {
  test(`a refused continuation and failed question reread in ${mode} retain editable answers and recover without replay`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    const writes: unknown[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/runs/${RUN}/trace*`, async (route) => {
      if (new URL(route.request().url()).searchParams.get("mode") === "advanced")
        return route.fallback();
      reads += 1;
      if (reads === 2)
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      if (reads === 3) await gate;
      return route.fulfill({ json: ASKING });
    });
    await page.route(`**/runs/${RUN}/continuations`, (route) => {
      writes.push(route.request().postDataJSON());
      return route.fulfill({ status: 422, json: { error: "這段對話已經到回答輪數的上限。" } });
    });
    try {
      await openQuestions(page);
      if (mode === "進階模式") await page.getByRole("button", { name: mode, exact: true }).click();
      const region = await fillAnswers(page);
      const inputs = region.getByRole("textbox", { name: "你的回答", exact: true });
      const submit = region.getByRole("button", { name: "送出回答並繼續試跑", exact: true });
      await submit.click();
      await expect(region.getByRole("status")).toContainText("原始問題尚未確認");
      await expect(inputs.nth(0)).toHaveValue("Sheet2");
      await expect(inputs.nth(1)).toHaveValue("保留最新的一筆");
      await expect(inputs.nth(1)).toBeEditable();
      await expect(submit).toBeDisabled();
      await expect(region.getByRole("alert")).toHaveText(
        "回答未被接受。這段對話已經到回答輪數的上限。",
      );
      await inputs.nth(1).fill("更新後的理由");
      const retry = region.getByRole("button", { name: "重新確認問題", exact: true });
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => reads).toBe(3);
      await expect(retry).toBeDisabled();
      await expect(submit).toBeDisabled();
      await verifyFocus(page);
      await inputs.nth(1).fill("再補充一行");
      expect(writes).toEqual([{ answers: ["Sheet2", "保留最新的一筆"] }]);
      release();
      await expect(submit).toBeEnabled();
      await expect(retry).toHaveCount(0);
      await expect(inputs.nth(0)).toHaveValue("Sheet2");
      await expect(inputs.nth(1)).toHaveValue("再補充一行");
      await expect(inputs.nth(1)).toBeFocused();
      expect(writes).toHaveLength(1);
      expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: test.info().outputPath("answer-draft-recovered.png") });
    } finally {
      release();
    }
  });
}

test("a confirmed continuation still navigates when refreshing the original trace fails", async ({
  page,
}) => {
  await stubPlatform(page);
  let sent = false;
  const writes: unknown[] = [];
  await page.route(`**/runs/${RUN}/trace`, (route) =>
    sent
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ASKING }),
  );
  await page.route(`**/runs/${RUN}/continuations`, (route) => {
    sent = true;
    writes.push(route.request().postDataJSON());
    return route.fulfill({
      status: 201,
      json: { run_id: NEXT, status: "queued", provider: "unassigned" },
    });
  });
  await openQuestions(page);
  const region = await fillAnswers(page);
  await region.getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/runs/${NEXT}$`));
  await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toBeVisible();
  expect(writes).toEqual([{ answers: ["Sheet2", "保留最新的一筆"] }]);
});

for (const status of [401, 404]) {
  test(`a ${status} question read does not retain protected cached answers`, async ({ page }) => {
    await stubPlatform(page);
    let sent = false;
    let posts = 0;
    await page.route(`**/runs/${RUN}/trace`, (route) =>
      sent
        ? route.fulfill({
            status,
            json: { error: status === 401 ? "not authenticated" : "not found" },
          })
        : route.fulfill({ json: ASKING }),
    );
    await page.route(`**/runs/${RUN}/continuations`, (route) => {
      sent = true;
      posts += 1;
      return route.fulfill({ status: 422, json: { error: "這段對話已經到回答輪數的上限。" } });
    });
    await openQuestions(page);
    const region = await fillAnswers(page);
    await region.getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
    if (status === 401)
      await expect(page.getByText("執行紀錄需要登入。", { exact: false })).toBeVisible();
    else await expect(page.getByText("無法讀取執行紀錄。", { exact: true })).toBeVisible();
    await expect(region).toHaveCount(0);
    expect(posts).toBe(1);
  });
}

test("questions belonging to another Run never become an answer form for this Run", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.route(`**/runs/${RUN}/trace`, (route) =>
    route.fulfill({ json: { ...ASKING, run_id: NEXT } }),
  );
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator("#run-trace").getByRole("alert")).toHaveText("無法讀取執行紀錄。");
  await expect(
    page.locator('#run-trace p[role="status"]').filter({ hasText: "執行狀態" }),
  ).toHaveCount(0);
  await expect(page.locator("#run-decision .verdict")).toHaveText("任務判定：未符合");
  await expect(page.getByRole("region", { name: "這次試跑需要你回答", exact: true })).toHaveCount(
    0,
  );
});

for (const status of ["running", "failed"]) {
  test(`questions on a ${status} Run do not offer continuation before an eligible completion`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.route(`**/runs/${RUN}/trace`, (route) =>
      route.fulfill({ json: { ...ASKING, status } }),
    );
    await page.goto(`/runs/${RUN}`);
    await expect(
      page.locator('#run-trace p[role="status"]').filter({ hasText: "執行狀態" }),
    ).toContainText(status);
    await expect(page.getByRole("region", { name: "這次試跑需要你回答", exact: true })).toHaveCount(
      0,
    );
  });
}

test("a cached next Run receives a fresh answer draft rather than the previous Run's answers", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.route("**/me/activity*", (route) =>
    route.fulfill({
      json: {
        ...ACTIVITY,
        items: ACTIVITY.items.map((item) =>
          item.kind === "run"
            ? { ...item, source_id: RUN, continuation: { kind: "run", run_id: RUN } }
            : item,
        ),
      },
    }),
  );
  await page.route(`**/runs/*/trace*`, (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("mode") === "advanced") return route.fallback();
    return route.fulfill({ json: { ...ASKING, run_id: url.pathname.split("/")[2] } });
  });
  await page.route(`**/runs/${RUN}/continuations`, (route) =>
    route.fulfill({
      status: 201,
      json: { run_id: NEXT, status: "queued", provider: "unassigned" },
    }),
  );
  await openQuestions(page, NEXT);
  await expect(page.getByRole("textbox", { name: "你的回答", exact: true }).nth(0)).toBeVisible();
  await page
    .getByRole("navigation", { name: "主要導覽", exact: true })
    .getByRole("link", { name: "活動", exact: true })
    .click();
  await expect(page.getByRole("heading", { name: "活動", exact: true })).toBeVisible();
  await page.locator(`main a[href="/runs/${RUN}"]`).first().click();
  const region = await fillAnswers(page);
  await region.getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/runs/${NEXT}$`));
  await expect(region.getByRole("textbox", { name: "你的回答", exact: true }).nth(0)).toHaveValue(
    "",
  );
  await expect(region.getByRole("textbox", { name: "你的回答", exact: true }).nth(1)).toHaveValue(
    "",
  );
});
