import { act } from "react";
import { expect, test } from "vitest";
import { SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp, type Request } from "../../../testing/app";

const app = renderedApp();
const FIRST = "00000000-0000-4000-8000-0000000000d1";
const SECOND = "00000000-0000-4000-8000-0000000000d2";

function writes(requests: Request[]) {
  return requests
    .filter((r) => r.method !== "GET")
    .map((r) => ({ method: r.method, path: r.url.pathname, body: r.body }));
}

async function pick(page: ReturnType<typeof app>, name: string, content: BlobPart) {
  const input = page.getByLabel("要匯入的檔案", { exact: true });
  await page.eventually(() => expect(input.count()).toBe(1));
  const element = input.element() as HTMLInputElement;
  Object.defineProperty(element, "files", {
    value: [new File([content], name)],
    configurable: true,
  });
  await act(async () => {
    element.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

const TWO_ROWS = [
  JSON.stringify({ name: "缺日期", user_prompt: "沒有日期的發票", criteria: ["指出缺日期"] }),
  JSON.stringify({ query: "外幣發票" }),
  "not json",
].join("\n");

test("each readable row is shown, unreadable ones are named, and importing creates them in order", async () => {
  const page = app();
  const ids = [FIRST, SECOND];
  page.route(
    (url) => url.pathname === "/test-cases",
    ({ method }) =>
      method === "POST" ? { status: 201, json: { test_case_id: ids.shift() } } : undefined,
  );
  page.route("**/criteria", () => ({ status: 201, json: { test_case_id: FIRST } }));
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await pick(page, "cases.jsonl", TWO_ROWS);

  await page.eventually(() =>
    expect(page.getByRole("alert").text()).toContain("第 3 列：這一行不是有效的 JSON。"),
  );
  expect(page.getByRole("heading", { name: "匯入的第 2 列", exact: true }).count()).toBe(1);
  expect(writes(page.requests)).toEqual([]);

  await page.getByRole("button", { name: "匯入這 2 題", exact: true }).click();
  await page.eventually(() =>
    expect(page.getByText("已匯入 2 題，驗收條件標成你自己寫的。", { exact: true }).count()).toBe(
      1,
    ),
  );
  expect(page.getByRole("button", { name: /^匯入這/ }).count()).toBe(0);
  expect(writes(page.requests)).toEqual([
    {
      method: "POST",
      path: "/test-cases",
      body: { skill_id: SKILL, name: "缺日期", user_prompt: "沒有日期的發票" },
    },
    {
      method: "POST",
      path: `/test-cases/${FIRST}/criteria`,
      body: { source: "user", text: "指出缺日期" },
    },
    {
      method: "POST",
      path: "/test-cases",
      body: { skill_id: SKILL, name: "匯入的第 2 列", user_prompt: "外幣發票" },
    },
  ]);
  expect(await page.axeViolations("main")).toEqual([]);
});

test("a refused row stops the import and keeps it and the rows after it for another try", async () => {
  const page = app();
  let creates = 0;
  page.route(
    (url) => url.pathname === "/test-cases",
    ({ method }) => {
      if (method !== "POST") return undefined;
      creates += 1;
      return creates === 1
        ? { status: 201, json: { test_case_id: FIRST } }
        : { status: 400, json: { error: "名稱不能空白" } };
    },
  );
  page.route("**/criteria", () => ({ status: 201, json: { test_case_id: FIRST } }));
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await pick(
    page,
    "cases.jsonl",
    `${TWO_ROWS}
${JSON.stringify({ query: "第三題" })}`,
  );
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "匯入這 3 題", exact: true }).count()).toBe(1),
  );
  await page.getByRole("button", { name: "匯入這 3 題", exact: true }).click();

  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "名稱不能空白" }).count()).toBe(1),
  );
  expect(page.getByText("已匯入 1 題，驗收條件標成你自己寫的。", { exact: true }).count()).toBe(1);
  expect(page.getByRole("heading", { name: "缺日期", exact: true }).count()).toBe(0);
  expect(page.getByRole("button", { name: "匯入這 2 題", exact: true }).count()).toBe(1);
  expect(creates).toBe(2);
});

test("a row whose criterion fails is counted as created and named for repair", async () => {
  const page = app();
  page.route(
    (url) => url.pathname === "/test-cases",
    ({ method }) =>
      method === "POST" ? { status: 201, json: { test_case_id: FIRST } } : undefined,
  );
  page.route("**/criteria", () => ({ status: 500, json: { error: "boom" } }));
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await pick(page, "cases.jsonl", TWO_ROWS);
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "匯入這 2 題", exact: true }).count()).toBe(1),
  );
  await page.getByRole("button", { name: "匯入這 2 題", exact: true }).click();
  await page.eventually(() =>
    expect(
      page
        .getByText(
          "已匯入 2 題，驗收條件標成你自己寫的。 其中 1 題有驗收條件沒有加進去，請到那幾題裡補上。",
          { exact: true },
        )
        .count(),
    ).toBe(1),
  );
});

test("a file over 2 MB is refused before it is read", async () => {
  const page = app();
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await pick(page, "big.jsonl", new Uint8Array(2_000_001));
  await page.eventually(() =>
    expect(page.getByRole("alert").text()).toContain("檔案超過 1.9 MB，沒有讀。"),
  );
  expect(page.getByRole("button", { name: /^匯入這/ }).count()).toBe(0);
});

test("a file of exactly 2 MB is read", async () => {
  const page = app();
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  const row = JSON.stringify({ query: "p" });
  await pick(page, "edge.jsonl", row + " ".repeat(2_000_000 - row.length));
  await page.eventually(() =>
    expect(page.getByRole("button", { name: "匯入這 1 題", exact: true }).count()).toBe(1),
  );
});

test("the import section is absent until the list is narrowed to one skill", async () => {
  const page = app();
  await page.goto("/lab/test-cases");
  await page.eventually(() =>
    expect(page.getByRole("heading", { name: "既有的測試題", exact: true }).count()).toBe(1),
  );
  expect(page.getByLabel("要匯入的檔案", { exact: true }).count()).toBe(0);
});
