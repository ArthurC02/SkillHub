import { expect, test } from "vitest";
import { SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp, type Request } from "../../../testing/app";

const app = renderedApp();
const CREATED = "00000000-0000-4000-8000-0000000000c1";

const MISSING_DATE = {
  name: "缺日期的發票",
  user_prompt: "這張發票沒有日期，幫我算總額",
  criteria: ["指出缺少日期", "仍然算出總額"],
};
const FOREIGN_CURRENCY = { name: "外幣發票", user_prompt: "這張是美元發票", criteria: [] };

function writes(requests: Request[]) {
  return requests
    .filter((r) => r.method !== "GET")
    .map((r) => ({ method: r.method, path: r.url.pathname, body: r.body }));
}

async function suggestedPage(reply: { status?: number; json?: unknown }) {
  const page = app();
  page.route("**/test-cases/suggest", () => reply);
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await page.getByLabel("想測的方向", { exact: true }).fill("發票缺欄位的時候");
  await page.getByRole("button", { name: "請系統建議", exact: true }).click();
  return page;
}

test("adopting a suggestion creates the test case and adds its criteria as unconfirmed suggestions", async () => {
  const page = app();
  page.route("**/test-cases/suggest", () => ({
    json: { suggestions: [MISSING_DATE, FOREIGN_CURRENCY] },
  }));
  page.route(
    (url) => url.pathname === "/test-cases",
    ({ method }) =>
      method === "POST" ? { status: 201, json: { test_case_id: CREATED } } : undefined,
  );
  page.route(`**/test-cases/${CREATED}/criteria`, () => ({
    status: 201,
    json: { test_case_id: CREATED },
  }));
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await page.getByLabel("想測的方向", { exact: true }).fill("發票缺欄位的時候");
  await page.getByRole("button", { name: "請系統建議", exact: true }).click();

  const proposal = page.getByRole("listitem").filter({ hasText: "缺日期的發票" });
  await page.eventually(() => expect(proposal.count()).toBe(1));
  await proposal.getByRole("button", { name: "採納", exact: true }).click();

  await page.eventually(() =>
    expect(page.getByText("已建立測試題，驗收條件標成系統建議、尚未確認。").count()).toBe(1),
  );
  expect(page.getByRole("link", { name: "打開這個測試題", exact: true }).attribute("href")).toBe(
    `/lab/test-cases/${CREATED}?version=${VERSION}`,
  );
  expect(page.getByRole("listitem").filter({ hasText: "缺日期的發票" }).count()).toBe(0);
  expect(page.getByRole("listitem").filter({ hasText: "外幣發票" }).count()).toBe(1);
  expect(writes(page.requests)).toEqual([
    {
      method: "POST",
      path: "/test-cases/suggest",
      body: { skill_id: SKILL, focus: "發票缺欄位的時候" },
    },
    {
      method: "POST",
      path: "/test-cases",
      body: { skill_id: SKILL, name: "缺日期的發票", user_prompt: "這張發票沒有日期，幫我算總額" },
    },
    {
      method: "POST",
      path: `/test-cases/${CREATED}/criteria`,
      body: { source: "suggested", text: "指出缺少日期" },
    },
    {
      method: "POST",
      path: `/test-cases/${CREATED}/criteria`,
      body: { source: "suggested", text: "仍然算出總額" },
    },
  ]);
  expect(await page.axeViolations("main")).toEqual([]);
});

test("a criterion that fails after the test case was created says how many are missing", async () => {
  const page = await suggestedPage({ json: { suggestions: [MISSING_DATE] } });
  page.route(
    (url) => url.pathname === "/test-cases",
    ({ method }) =>
      method === "POST" ? { status: 201, json: { test_case_id: CREATED } } : undefined,
  );
  page.route(`**/test-cases/${CREATED}/criteria`, () => ({ status: 500, json: { error: "boom" } }));
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "採納", exact: true }).count()).toBe(1),
  );
  await page.getByRole("button", { name: "採納", exact: true }).click();

  await page.eventually(() =>
    expect(
      page.getByText("已建立測試題，但有 2 條驗收條件沒有加進去，請到測試題裡補上。").count(),
    ).toBe(1),
  );
});

test("ignoring a suggestion removes it and writes nothing", async () => {
  const page = await suggestedPage({ json: { suggestions: [MISSING_DATE] } });
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "忽略", exact: true }).count()).toBe(1),
  );
  await page.getByRole("button", { name: "忽略", exact: true }).click();

  await page.eventually(() =>
    expect(page.getByRole("listitem").filter({ hasText: "缺日期的發票" }).count()).toBe(0),
  );
  expect(writes(page.requests).map((w) => w.path)).toEqual(["/test-cases/suggest"]);
});

test("an empty answer says there was nothing usable", async () => {
  const page = await suggestedPage({ json: { suggestions: [] } });
  await page.eventually(() =>
    expect(page.getByText("這次沒有可用的建議，請自己手動寫。", { exact: true }).count()).toBe(1),
  );
});

for (const { status, json, sentence } of [
  {
    status: 422,
    json: { error: "點數不足，無法開始建議測試題，沒有呼叫模型。請聯絡管理者為這個帳號加點。" },
    sentence:
      "點數不足，無法開始建議測試題，沒有呼叫模型。請聯絡管理者為這個帳號加點。測試題可以自己手動寫。",
  },
  {
    status: 422,
    json: { error: "insufficient credit" },
    sentence: "點數不足，這次沒有呼叫模型、也沒有扣點。請管理者加點後再試。測試題可以自己手動寫。",
  },
  { status: 403, json: { error: "封測" }, sentence: "自動建議只開放給封測受邀帳號" },
  {
    status: 503,
    json: { error: "平台暫停中，這次沒有呼叫模型、也沒有扣點。測試題可以自己手動寫，或稍後再試。" },
    sentence: "平台暫停中，這次沒有呼叫模型、也沒有扣點。",
  },
  {
    status: 200,
    json: { suggestions: [{ name: "少了 prompt" }] },
    sentence: "無法取得建議，可以再試一次。",
  },
]) {
  test(`a ${status} suggestion answer${status === 200 ? " of the wrong shape" : ""} (${JSON.stringify(json)}) explains itself`, async () => {
    const page = await suggestedPage({ status, json });
    await page.eventually(() => expect(page.getByRole("alert").text()).toContain(sentence));
    expect(page.getByRole("button", { name: "採納", exact: true }).count()).toBe(0);
  });
}

test("a focus at 500 characters is sent and one past it is refused before any request", async () => {
  const page = app();
  page.route("**/test-cases/suggest", () => ({ json: { suggestions: [] } }));
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  const focus = page.getByLabel("想測的方向", { exact: true });
  const ask = page.getByRole("button", { name: "請系統建議", exact: true });

  await focus.fill("字".repeat(501));
  await page.eventually(() => expect(ask.attribute("disabled")).not.toBeNull());
  expect(page.getByText("想測的方向最多 500 個字。", { exact: true }).count()).toBe(1);

  await focus.fill("字".repeat(500));
  await ask.click();
  await page.eventually(() => expect(writes(page.requests)).toHaveLength(1));
  expect(writes(page.requests)[0].body).toEqual({ skill_id: SKILL, focus: "字".repeat(500) });
});

test("the suggestion section is absent until the list is narrowed to one skill", async () => {
  const page = app();
  await page.goto("/lab/test-cases");
  await page.eventually(() =>
    expect(page.getByRole("heading", { name: "既有的測試題", exact: true }).count()).toBe(1),
  );
  expect(page.getByLabel("想測的方向", { exact: true }).count()).toBe(0);
});
