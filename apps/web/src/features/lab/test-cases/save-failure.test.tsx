import { expect, test } from "vitest";
import { TEST_CASE } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

test("a refused simulated user save with no server reason says the save did not go through", async () => {
  const page = app();
  page.route(`**/test-cases/${TEST_CASE}`, (request) =>
    request.method === "PATCH" ? { status: 400, json: {} } : undefined,
  );
  await page.goto(`/lab/test-cases/${TEST_CASE}`);
  const field = page.getByLabel("使用者知道的事", { exact: true });
  await page.eventually(() => expect(field.count()).toBe(1));
  await field.fill("只看第一欄");
  await page.getByRole("button", { name: "儲存模擬使用者", exact: true }).click();

  await page.eventually(() =>
    expect(page.getByText("儲存沒有成功，可以再試一次。").count()).toBe(1),
  );
});
