import { act } from "react";
import { expect, test } from "vitest";
import { RUN, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();
const SAVED = "00000000-0000-4000-8000-0000000000e1";

function saves(page: ReturnType<typeof app>) {
  return page.requests
    .filter((r) => r.method === "POST" && r.url.pathname === `/runs/${RUN}/test-case`)
    .map((r) => r.body);
}

async function named(page: ReturnType<typeof app>, name: string) {
  const input = page.getByLabel("存成測試題，名稱", { exact: true });
  await page.eventually(() => expect(input.count()).toBe(1));
  await input.fill(name);
}

test("saving a run sends the trimmed name and links to the new test case", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/test-case`, () => ({ status: 201, json: { test_case_id: SAVED } }));
  await page.goto(`/runs/${RUN}`);
  await named(page, "  發票缺日期  ");
  await page.getByRole("button", { name: "存成測試題", exact: true }).click();

  await page.eventually(() => expect(page.getByText(/^已存成新的測試題。/).count()).toBe(1));
  expect(page.getByRole("link", { name: "打開這個測試題", exact: true }).attribute("href")).toBe(
    `/lab/test-cases/${SAVED}?version=${VERSION}`,
  );
  expect(saves(page)).toEqual([{ name: "發票缺日期" }]);
  expect(await page.axeViolations("main")).toEqual([]);
});

test("a blank name and one past 200 characters send nothing, and 200 is accepted", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/test-case`, () => ({ status: 201, json: { test_case_id: SAVED } }));
  await page.goto(`/runs/${RUN}`);
  const button = page.getByRole("button", { name: "存成測試題", exact: true });

  await named(page, "   ");
  expect(button.attribute("aria-disabled")).toBe("true");
  await act(async () => button.element().click());
  expect(page.getByText("先填測試題名稱。", { exact: true }).count()).toBe(1);
  await named(page, "名".repeat(201));
  await page.eventually(() =>
    expect(page.getByText("名稱最多 200 個字。", { exact: true }).count()).toBe(1),
  );
  expect(button.attribute("aria-disabled")).toBe("true");

  await named(page, "名".repeat(200));
  await page.eventually(() => expect(button.attribute("aria-disabled")).toBe("false"));
  await button.click();
  await page.eventually(() => expect(saves(page)).toEqual([{ name: "名".repeat(200) }]));
  expect(saves(page)).toHaveLength(1);
});

test("a refused save shows the server's sentence and keeps the name for another try", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/test-case`, () => ({
    status: 404,
    json: { error: "找不到這個小工具" },
  }));
  await page.goto(`/runs/${RUN}`);
  await named(page, "發票缺日期");
  await page.getByRole("button", { name: "存成測試題", exact: true }).click();
  await page.eventually(() =>
    expect(page.getByRole("alert").filter({ hasText: "找不到這個小工具" }).count()).toBe(1),
  );
  expect(page.getByLabel("存成測試題，名稱", { exact: true }).value()).toBe("發票缺日期");
  expect(page.getByText(/^已存成新的測試題。/).count()).toBe(0);
});

test("a save answer without an id is reported as unconfirmed, not as saved", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/test-case`, () => ({ status: 201, json: {} }));
  await page.goto(`/runs/${RUN}`);
  await named(page, "發票缺日期");
  await page.getByRole("button", { name: "存成測試題", exact: true }).click();
  await page.eventually(() =>
    expect(page.getByRole("alert").text()).toContain("尚未確認是否已存成測試題"),
  );
  expect(page.getByText(/^已存成新的測試題。/).count()).toBe(0);
});
