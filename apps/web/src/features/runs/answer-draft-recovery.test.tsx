import { expect, test } from "vitest";
import { ACTIVITY, RUN, TRACE_GENERAL } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

const NEXT = "99999999-1111-4111-8111-111111111111";
const ASKING = {
  ...TRACE_GENERAL,
  status: "succeeded",
  questions: [
    { question: "要處理哪個工作表？", why: "確認資料", options: ["Sheet1", "Sheet2"] },
    { question: "重複列要保留哪一筆？", why: "確認去重方向" },
  ],
};
const REGION = "這次試跑需要你回答";

const regionOf = (page: App) => page.getByRole("region", { name: REGION, exact: true });
const answersOf = (page: App) =>
  regionOf(page).getByRole("textbox", { name: "你的回答", exact: true });
const posts = (page: App) => page.requests.filter((request) => request.method === "POST");

async function openQuestions(page: App, runId = RUN) {
  await page.goto(`/runs/${runId}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await expect(regionOf(page)).toHaveCount(1);
  return regionOf(page);
}

async function fillAnswers(page: App) {
  const region = regionOf(page);
  await region.getByRole("radio", { name: "Sheet2", exact: true }).check();
  await region
    .getByRole("textbox", { name: "你的回答", exact: true })
    .nth(1)
    .fill("保留最新的一筆");
  return region;
}

function isAdvanced(url: URL) {
  return url.searchParams.get("mode") === "advanced";
}

function askingReads(page: App) {
  page.route(`**/runs/${RUN}/trace*`, ({ url }) =>
    isAdvanced(url) ? undefined : { json: ASKING },
  );
}

const SUBMIT = "送出回答並繼續試跑";

test("switching trace detail preserves selected and typed answers without sending them", async () => {
  const page = app();
  askingReads(page);
  const region = await openQuestions(page);
  await fillAnswers(page);
  await page.getByRole("button", { name: "進階模式", exact: true }).click();
  await page.eventually(() =>
    expect(
      page.getByRole("button", { name: "進階模式", exact: true }).attribute("aria-pressed"),
    ).toBe("true"),
  );
  await expect(region).toHaveCount(1);
  expect(region.getByRole("radio", { name: "Sheet2", exact: true }).element()).toHaveProperty(
    "checked",
    true,
  );
  await expect(answersOf(page).nth(1)).toHaveValue("保留最新的一筆");
  await answersOf(page).nth(0).fill("Sheet3");
  await page.getByRole("button", { name: "一般模式", exact: true }).click();
  await expect(answersOf(page).nth(0)).toHaveValue("Sheet3");
  await expect(answersOf(page).nth(1)).toHaveValue("保留最新的一筆");
  expect(posts(page)).toEqual([]);
});

for (const mode of ["一般模式", "進階模式"]) {
  test(`a refused continuation and failed question reread in ${mode} retain editable answers and recover without replay`, async () => {
    const page = app();
    let reads = 0;
    const writes: unknown[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(`**/runs/${RUN}/trace*`, async ({ url }) => {
      if (isAdvanced(url)) return undefined;
      reads += 1;
      if (reads === 2) return { status: 503, json: { error: "service unavailable" } };
      if (reads === 3) await gate;
      return { json: ASKING };
    });
    page.route(`**/runs/${RUN}/continuations`, ({ body }) => {
      writes.push(body);
      return { status: 422, json: { error: "這段對話已經到回答輪數的上限。" } };
    });
    try {
      await openQuestions(page);
      if (mode === "進階模式") await page.getByRole("button", { name: mode, exact: true }).click();
      const region = await fillAnswers(page);
      const inputs = answersOf(page);
      const submit = region.getByRole("button", { name: SUBMIT, exact: true });
      await submit.click();
      await expect(region.getByRole("status")).toContainText("原始問題尚未確認");
      await expect(inputs.nth(0)).toHaveValue("Sheet2");
      await expect(inputs.nth(1)).toHaveValue("保留最新的一筆");
      await expect(inputs.nth(1)).toBeEnabled();
      expect(inputs.nth(1).attribute("readonly")).toBeNull();
      await expect(submit).toBeDisabled();
      await expect(region.getByRole("alert")).toHaveText(
        "回答未被接受。這段對話已經到回答輪數的上限。",
      );
      await inputs.nth(1).fill("更新後的理由");
      const retry = region.getByRole("button", { name: "重新確認問題", exact: true });
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() => expect(reads).toBe(3));
      await expect(retry).toBeDisabled();
      await expect(submit).toBeDisabled();
      await expect(page.getByRole("heading", { name: REGION, exact: true })).toBeFocused();
      await inputs.nth(1).fill("再補充一行");
      expect(writes).toEqual([{ answers: ["Sheet2", "保留最新的一筆"] }]);
      release();
      await expect(submit).toBeEnabled();
      await expect(retry).toHaveCount(0);
      await expect(inputs.nth(0)).toHaveValue("Sheet2");
      await expect(inputs.nth(1)).toHaveValue("再補充一行");
      await expect(inputs.nth(1)).toBeFocused();
      expect(writes).toHaveLength(1);
      expect(page.pathname()).toBe(`/runs/${RUN}`);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
    }
  });
}

test("a confirmed continuation still navigates when refreshing the original trace fails", async () => {
  const page = app();
  let sent = false;
  const writes: unknown[] = [];
  page.route(`**/runs/${RUN}/trace`, () =>
    sent ? { status: 503, json: { error: "service unavailable" } } : { json: ASKING },
  );
  page.route(`**/runs/${RUN}/continuations`, ({ body }) => {
    sent = true;
    writes.push(body);
    return { status: 201, json: { run_id: NEXT, status: "queued", provider: "unassigned" } };
  });
  await openQuestions(page);
  const region = await fillAnswers(page);
  await region.getByRole("button", { name: SUBMIT, exact: true }).click();
  await page.eventually(() => expect(page.pathname()).toBe(`/runs/${NEXT}`));
  await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toBeVisible();
  expect(writes).toEqual([{ answers: ["Sheet2", "保留最新的一筆"] }]);
});

for (const status of [401, 404]) {
  test(`a ${status} question read does not retain protected cached answers`, async () => {
    const page = app();
    let sent = false;
    let count = 0;
    page.route(`**/runs/${RUN}/trace`, () =>
      sent
        ? { status, json: { error: status === 401 ? "not authenticated" : "not found" } }
        : { json: ASKING },
    );
    page.route(`**/runs/${RUN}/continuations`, () => {
      sent = true;
      count += 1;
      return { status: 422, json: { error: "這段對話已經到回答輪數的上限。" } };
    });
    await openQuestions(page);
    const region = await fillAnswers(page);
    await region.getByRole("button", { name: SUBMIT, exact: true }).click();
    if (status === 401)
      await expect(page.getByText("執行紀錄需要登入。", { exact: false })).toBeVisible();
    else await expect(page.getByText("無法讀取執行紀錄。", { exact: true })).toBeVisible();
    await expect(region).toHaveCount(0);
    expect(count).toBe(1);
  });
}

test("questions belonging to another Run never become an answer form for this Run", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/trace`, () => ({ json: { ...ASKING, run_id: NEXT } }));
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator("#run-trace").getByRole("alert")).toHaveText("無法讀取執行紀錄。");
  await expect(
    page.locator('#run-trace p[role="status"]').filter({ hasText: "執行狀態" }),
  ).toHaveCount(0);
  await expect(page.locator("#run-decision .verdict")).toHaveText("任務判定：未符合");
  await expect(regionOf(page)).toHaveCount(0);
});

for (const status of ["running", "failed"]) {
  test(`questions on a ${status} Run do not offer continuation before an eligible completion`, async () => {
    const page = app();
    page.route(`**/runs/${RUN}/trace`, () => ({ json: { ...ASKING, status } }));
    await page.goto(`/runs/${RUN}`);
    const statusLine = page.locator('#run-trace p[role="status"]').filter({ hasText: "執行狀態" });
    await expect(statusLine).toContainText(status);
    await expect(regionOf(page)).toHaveCount(0);
  });
}

test("a cached next Run receives a fresh answer draft rather than the previous Run's answers", async () => {
  const page = app();
  page.route("**/me/activity*", () => ({
    json: {
      ...ACTIVITY,
      items: ACTIVITY.items.map((item) =>
        item.kind === "run"
          ? { ...item, source_id: RUN, continuation: { kind: "run", run_id: RUN } }
          : item,
      ),
    },
  }));
  page.route(`**/runs/*/trace*`, ({ url }) => {
    if (isAdvanced(url)) return undefined;
    return { json: { ...ASKING, run_id: url.pathname.split("/")[2] } };
  });
  page.route(`**/runs/${RUN}/continuations`, () => ({
    status: 201,
    json: { run_id: NEXT, status: "queued", provider: "unassigned" },
  }));
  await openQuestions(page, NEXT);
  await expect(answersOf(page).nth(0)).toHaveCount(1);
  await page
    .getByRole("navigation", { name: "主要導覽", exact: true })
    .getByRole("link", { name: "活動", exact: true })
    .click();
  await expect(page.getByRole("heading", { name: "活動", exact: true })).toBeVisible();
  await expect(page.locator(`main a[href="/runs/${RUN}"]`)).toBeVisible();
  await page.locator(`main a[href="/runs/${RUN}"]`).first().click();
  await expect(regionOf(page)).toHaveCount(1);
  const region = await fillAnswers(page);
  await region.getByRole("button", { name: SUBMIT, exact: true }).click();
  await page.eventually(() => expect(page.pathname()).toBe(`/runs/${NEXT}`));
  await expect(answersOf(page).nth(0)).toHaveValue("");
  await expect(answersOf(page).nth(1)).toHaveValue("");
});
