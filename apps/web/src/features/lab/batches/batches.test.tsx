import { act } from "react";
import { expect, test } from "vitest";
import {
  BATCH,
  BATCH_REPORT,
  OTHER_BATCH,
  OTHER_RUN,
  PREFLIGHT,
  RUN,
  SKILL_VERSIONS,
  SKILL,
  TEST_CASE,
  VERSION,
  evaluationBatch,
} from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const NEW_BATCH = `/skills/${SKILL}/evaluation-batches?version=${VERSION}`;
const BATCH_PAGE = `/skills/${SKILL}/evaluation-batches/${BATCH}`;

function posts(page: ReturnType<typeof app>, pathEnd: string) {
  return page.requests.filter((r) => r.method === "POST" && r.url.pathname.endsWith(pathEnd));
}

async function chooseTheTestCase(page: ReturnType<typeof app>) {
  await page.getByRole("checkbox", { name: "去重複列" }).check();
}

test("the run count multiplies test cases by trials, and doubles with the skill-withheld control", async () => {
  const page = app();
  await page.goto(NEW_BATCH);
  await chooseTheTestCase(page);
  await page.getByLabel("每題跑幾次").fill("4");
  await page.eventually(() => expect(page.getByText(/^會開 4 次試跑/).count()).toBe(1));
  await page.getByRole("checkbox", { name: "每次都加跑一次不載入小工具的對照" }).check();
  await page.eventually(() => expect(page.getByText(/^會開 8 次試跑/).count()).toBe(1));
});

test("nothing is sent while no test case is chosen, and the button says why", async () => {
  const page = app();
  await page.goto(NEW_BATCH);
  await page.eventually(() =>
    expect(page.getByRole("checkbox", { name: "去重複列" }).count()).toBe(1),
  );
  const start = page.getByRole("button", { name: "開始評估批次" });
  expect(start.attribute("aria-disabled")).toBe("true");
  expect(page.getByRole("status").filter({ hasText: "先勾選至少一題測試題。" }).count()).toBe(1);
  const reason = start.attribute("aria-describedby");
  expect(reason && document.getElementById(reason)?.textContent).toBe("先勾選至少一題測試題。");
  await act(async () => start.element().click());
  expect(posts(page, "/evaluation-batches")).toHaveLength(0);
});

test.each([
  ["1", true],
  ["10", true],
  ["0", false],
  ["11", false],
])("trials %s is accepted: %s", async (trials, accepted) => {
  const page = app();
  await page.goto(NEW_BATCH);
  await chooseTheTestCase(page);
  await page.getByLabel("每題跑幾次").fill(trials);
  await page.eventually(() =>
    expect(page.getByRole("status").filter({ hasText: "每題要跑 1 到 10 次。" }).count()).toBe(
      accepted ? 0 : 1,
    ),
  );
});

test("a created batch sends exactly what was chosen and opens its result page", async () => {
  const page = app();
  page.route("**/evaluation-batches", (request) =>
    request.method === "POST"
      ? { status: 201, json: evaluationBatch(BATCH, { status: "launching" }) }
      : undefined,
  );
  await page.goto(NEW_BATCH);
  await chooseTheTestCase(page);
  await page.getByLabel("每題跑幾次").fill("2");
  await page.getByRole("checkbox", { name: "每次都加跑一次不載入小工具的對照" }).check();
  await page.getByRole("button", { name: "開始評估批次" }).click();
  await page.eventually(() => expect(page.pathname()).toBe(BATCH_PAGE));
  expect(posts(page, "/evaluation-batches").map((r) => r.body)).toEqual([
    {
      skill_version_id: VERSION,
      test_case_ids: [TEST_CASE],
      trials: 2,
      with_skill_withheld: true,
    },
  ]);
});

test("unconfirmed test cases show their run permissions and confirm them without starting a batch", async () => {
  const page = app();
  page.route("**/evaluation-batches", (request) =>
    request.method === "POST"
      ? {
          status: 422,
          json: {
            error: "run permissions are not confirmed",
            unconfirmed_test_case_ids: [TEST_CASE],
          },
        }
      : undefined,
  );
  page.route("**/runs/preflight/confirm", () => ({
    json: { confirmed: true, summary_hash: PREFLIGHT.summary_hash },
  }));
  await page.goto(NEW_BATCH);
  await chooseTheTestCase(page);
  await page.getByRole("button", { name: "開始評估批次" }).click();
  await page.eventually(() => {
    expect(page.getByRole("heading", { name: "這幾題要先確認試跑權限" }).count()).toBe(1);
    expect(page.getByText("網路", { exact: true }).count()).toBe(1);
  });
  await page.getByRole("button", { name: "我確認這一題的試跑權限" }).click();
  await page.eventually(() =>
    expect(page.getByText("已確認這一題的試跑權限。", { exact: true }).count()).toBe(1),
  );
  expect(posts(page, "/runs/preflight/confirm").map((r) => r.body)).toEqual([
    { version_id: VERSION, test_case_id: TEST_CASE, summary_hash: PREFLIGHT.summary_hash },
  ]);
  expect(posts(page, "/runs")).toHaveLength(0);
  expect(page.pathname()).toBe(`/skills/${SKILL}/evaluation-batches`);
});

test("a create whose outcome is unknown warns against resending before checking the list", async () => {
  const page = app();
  page.route("**/evaluation-batches", (request) =>
    request.method === "POST" ? { status: 502, json: { error: "bad gateway" } } : undefined,
  );
  await page.goto(NEW_BATCH);
  await chooseTheTestCase(page);
  await page.getByRole("button", { name: "開始評估批次" }).click();
  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "不確定批次有沒有開成" }).count()).toBe(1),
  );
  expect(page.pathname()).toBe(`/skills/${SKILL}/evaluation-batches`);
});

test("the result page shows both arms, the skill's effect, and links each trial to its run", async () => {
  const page = app();
  await page.goto(BATCH_PAGE);
  await page.eventually(() => {
    expect(page.getByText("50%（1 / 2 次），95% 信賴區間 9%～91%").count()).toBe(1);
    expect(page.getByText("0%（0 / 2 次），95% 信賴區間 0%～66%").count()).toBe(1);
    expect(page.getByText("+50 個百分點").count()).toBe(1);
  });
  expect(page.getByRole("link", { name: "第 1 次：符合" }).attribute("href")).toBe(`/runs/${RUN}`);
  expect(page.getByRole("link", { name: "第 2 次：未符合" }).attribute("href")).toBe(
    `/runs/${OTHER_RUN}`,
  );
});

test("a batch that is not this skill's says so and offers no retry", async () => {
  const page = app();
  page.route(`**/evaluation-batches/${BATCH}`, () => ({
    status: 404,
    json: { error: "evaluation batch not found" },
  }));
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "找不到這個評估批次" }).count()).toBe(1),
  );
  expect(page.getByRole("button", { name: "再試一次" }).count()).toBe(0);
});

test("a report for another batch is refused instead of shown as this one", async () => {
  const page = app();
  page.route(`**/evaluation-batches/${BATCH}`, () => ({
    json: { ...BATCH_REPORT, batch: evaluationBatch(OTHER_BATCH) },
  }));
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "暫時無法讀取評估批次" }).count()).toBe(1),
  );
  expect(page.getByText(/通過率/).count()).toBe(0);
});

test("a batch paused for credit explains it and resumes on request", async () => {
  const page = app();
  page.route(`**/evaluation-batches/${BATCH}`, () => ({
    json: {
      ...BATCH_REPORT,
      batch: evaluationBatch(BATCH, { status: "paused", stop_reason: "credit_balance" }),
    },
  }));
  page.route("**/resume", () => ({ status: 204 }));
  await page.goto(BATCH_PAGE);
  await page.eventually(() =>
    expect(page.getByText(/^點數不夠開下一次試跑，批次暫停了。/).count()).toBe(1),
  );
  await page.getByRole("button", { name: "繼續這個批次" }).click();
  await page.eventually(() =>
    expect(posts(page, `/evaluation-batches/${BATCH}/resume`)).toHaveLength(1),
  );
});

test("resuming a batch that is no longer paused says the page will catch up", async () => {
  const page = app();
  page.route(`**/evaluation-batches/${BATCH}`, () => ({
    json: {
      ...BATCH_REPORT,
      batch: evaluationBatch(BATCH, { status: "paused", stop_reason: "quota_daily" }),
    },
  }));
  page.route("**/resume", () => ({ status: 409, json: { error: "batch is not paused" } }));
  await page.goto(BATCH_PAGE);
  await page.getByRole("button", { name: "繼續這個批次" }).click();
  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "已經不是暫停狀態" }).count()).toBe(1),
  );
});

test("choosing a reference batch puts it in the address and shows the paired difference", async () => {
  const page = app();
  await page.goto(BATCH_PAGE);
  await page.getByRole("combobox", { name: "比較基準" }).select(OTHER_BATCH);
  await page.eventually(() => {
    expect(page.search()).toBe(`?against=${OTHER_BATCH}`);
    expect(page.getByText("+25 個百分點").count()).toBe(1);
    expect(
      page.getByText("這個批次比較常通過 1 題、比較基準比較常通過 0 題、一樣 0 題").count(),
    ).toBe(1);
  });
  expect(page.getByRole("link", { name: "開啟比較基準的批次結果" }).attribute("href")).toBe(
    `/skills/${SKILL}/evaluation-batches/${OTHER_BATCH}`,
  );
});

const NEWER_VERSION = "22222222-2222-2222-2222-333333333333";
const WITH_A_NEWER_VERSION = {
  versions: [
    {
      version_id: NEWER_VERSION,
      version_number: 3,
      content_hash: "sha256:cccc",
      created_at: "2026-11-06T00:00:00Z",
    },
    ...SKILL_VERSIONS.versions,
  ],
};

function checked(page: ReturnType<typeof app>, name: string) {
  return (page.getByRole("checkbox", { name }).element() as HTMLInputElement).checked;
}

test("a newest version with no batch offers re-running the most recent batch on it", async () => {
  const page = app();
  page.route("**/versions", () => ({ json: WITH_A_NEWER_VERSION }));
  await page.goto(NEW_BATCH);
  const offer = page.getByRole("link", { name: "用 v2 最近一批的題目重跑 v3" });
  await page.eventually(() => expect(offer.count()).toBe(1));
  expect(page.getByText(/^v3 還沒有評估批次。/).count()).toBe(1);
  expect(offer.attribute("href")).toBe(
    `/skills/${SKILL}/evaluation-batches?version=${NEWER_VERSION}&rerun=${BATCH}`,
  );
});

test("no re-run is offered while the newest version already has a batch", async () => {
  const page = app();
  await page.goto(NEW_BATCH);
  await page.eventually(() =>
    expect(page.getByRole("link", { name: /^v2・每題 2 次/ }).count()).toBe(2),
  );
  expect(page.getByText(/還沒有評估批次。/).count()).toBe(0);
});

test("a batch result offers re-running its test cases on the newest other version", async () => {
  const page = app();
  page.route("**/versions", () => ({ json: WITH_A_NEWER_VERSION }));
  await page.goto(BATCH_PAGE);
  const rerun = page.getByRole("link", { name: "用同樣的題目與次數重跑 v3" });
  await page.eventually(() => expect(rerun.count()).toBe(1));
  expect(rerun.attribute("href")).toBe(
    `/skills/${SKILL}/evaluation-batches?version=${NEWER_VERSION}&rerun=${BATCH}`,
  );
});

test("a batch on the newest version offers re-running on a version still to be picked", async () => {
  const page = app();
  await page.goto(BATCH_PAGE);
  const rerun = page.getByRole("link", { name: "用同樣的題目與次數重跑另一個版本" });
  await page.eventually(() => expect(rerun.count()).toBe(1));
  expect(rerun.attribute("href")).toBe(`/skills/${SKILL}/evaluation-batches?rerun=${BATCH}`);
});

test("re-running a batch starts from its choices and opens the new batch compared against it", async () => {
  const page = app();
  page.route("**/versions", () => ({ json: WITH_A_NEWER_VERSION }));
  page.route("**/evaluation-batches", (request) =>
    request.method === "POST"
      ? { status: 201, json: evaluationBatch(OTHER_BATCH, { skill_version_id: NEWER_VERSION }) }
      : undefined,
  );
  await page.goto(`/skills/${SKILL}/evaluation-batches?version=${NEWER_VERSION}&rerun=${BATCH}`);
  await page.eventually(() =>
    expect(
      page
        .getByText("已照 v2 那一批填好題目、次數與對照；開出來之後會直接和那一批逐題比較。")
        .count(),
    ).toBe(1),
  );
  await page.eventually(() => expect(checked(page, "去重複列")).toBe(true));
  expect(checked(page, "每次都加跑一次不載入小工具的對照")).toBe(true);
  expect(page.getByLabel("每題跑幾次").value()).toBe("2");

  await page.getByRole("button", { name: "開始評估批次" }).click();
  await page.eventually(() =>
    expect(page.pathname()).toBe(`/skills/${SKILL}/evaluation-batches/${OTHER_BATCH}`),
  );
  expect(page.search()).toBe(`?against=${BATCH}`);
  expect(posts(page, "/evaluation-batches").map((r) => r.body)).toEqual([
    {
      skill_version_id: NEWER_VERSION,
      test_case_ids: [TEST_CASE],
      trials: 2,
      with_skill_withheld: true,
    },
  ]);
});

test("a batch to re-run that cannot be read says so and leaves an empty form usable", async () => {
  const page = app();
  page.route(`**/evaluation-batches/${BATCH}`, () => ({ status: 500, json: { error: "boom" } }));
  page.route("**/evaluation-batches", (request) =>
    request.method === "POST" ? { status: 201, json: evaluationBatch(OTHER_BATCH) } : undefined,
  );
  await page.goto(`${NEW_BATCH}&rerun=${BATCH}`);
  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "要重跑的評估批次" }).count()).toBe(1),
  );
  await page.eventually(() =>
    expect(page.getByRole("checkbox", { name: "去重複列" }).count()).toBe(1),
  );
  expect(checked(page, "去重複列")).toBe(false);
  expect(page.getByText(/^已照/).count()).toBe(0);
  await chooseTheTestCase(page);
  await page.getByRole("button", { name: "開始評估批次" }).click();
  await page.eventually(() =>
    expect(page.pathname()).toBe(`/skills/${SKILL}/evaluation-batches/${OTHER_BATCH}`),
  );
  expect(page.search()).toBe("");
});
