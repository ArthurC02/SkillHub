import { expect, test } from "vitest";
import { TEST_CASE, TEST_CASE_DRAFT, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

function criterionReadFailure(page: App, status: number) {
  let failing = false;
  let writes = 0;
  let stored = TEST_CASE_DRAFT;
  page.route(`**/test-cases/${TEST_CASE}`, () => {
    if (failing && status === 0) return "network-error";
    return failing ? { status, json: { error: "read unavailable" } } : { json: stored };
  });
  page.route(`**/test-cases/${TEST_CASE}/criteria`, (request) => {
    writes += 1;
    const added = request.body as { text: string };
    stored = {
      ...stored,
      acceptance_criteria: [
        ...stored.acceptance_criteria,
        {
          id: "77777777-7777-4777-8777-777777777777",
          text: added.text,
          source: "user",
          confirmed_at: null,
        },
      ],
    };
    failing = true;
    return { status: 201, json: stored };
  });
  return {
    recover: () => {
      failing = false;
    },
    writes: () => writes,
  };
}

for (const status of [503, 408, 429, 0]) {
  test(`a successful criterion followed by a failed read ${status} preserves unrelated unsaved test-case fields`, async () => {
    const page = app();
    const server = criterionReadFailure(page, status);
    await page.goto(`/lab/test-cases/${TEST_CASE}?version=${VERSION}`);
    const name = page.getByLabel("名稱", { exact: true });
    const prompt = page.getByLabel("User Prompt", { exact: true });
    const facts = page.getByLabel("使用者知道的事", { exact: true });
    const rubric = page.getByLabel("Rubric 版本", { exact: true });
    await expect(name).toHaveCount(1);
    await name.fill("未儲存的新名稱");
    await prompt.fill("未儲存的任務提示");
    await facts.fill("未儲存的情境事實");
    await rubric.fill("draft-rubric-v2");
    await page.getByLabel("新增驗收條件", { exact: true }).fill("已經新增的條件");
    await page.getByRole("button", { name: "新增", exact: true }).click();
    await page.eventually(() => {
      expect(page.getByRole("alert").text()).toContain("無法讀取測試題");
      expect(page.locator(".test-case-detail-page").count()).toBe(0);
      expect(document.querySelector(".test-case-detail-page")?.hasAttribute("inert")).toBe(true);
      expect(page.getByRole("textbox", { name: "名稱", exact: true }).count()).toBe(0);
      expect(page.getByRole("button", { name: "新增", exact: true }).count()).toBe(0);
      expect(page.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(1);
    });
    server.recover();
    await page.getByRole("button", { name: "再試一次", exact: true }).click();
    await page.eventually(() => {
      expect(name.value()).toBe("未儲存的新名稱");
      expect(prompt.value()).toBe("未儲存的任務提示");
      expect(facts.value()).toBe("未儲存的情境事實");
      expect(rubric.value()).toBe("draft-rubric-v2");
      expect(
        page.locator(".criterion-list .criterion").filter({ hasText: "已經新增的條件" }).count(),
      ).toBe(1);
    });
    expect(server.writes()).toBe(1);
    expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}

for (const status of [401, 403, 404]) {
  test(`a subsequent test-case read ${status} removes the protected editor instead of retaining readable cached content`, async () => {
    const page = app();
    criterionReadFailure(page, status);
    await page.goto(`/lab/test-cases/${TEST_CASE}`);
    const name = page.getByLabel("名稱", { exact: true });
    await expect(name).toHaveCount(1);
    await name.fill("不能保留的保護內容");
    await page.getByLabel("新增驗收條件", { exact: true }).fill("新條件");
    await page.getByRole("button", { name: "新增", exact: true }).click();
    await page.eventually(() => {
      if (status === 401) expect(page.getByText("測試題需要登入。").count()).toBeGreaterThan(0);
      else expect(page.getByRole("alert").count()).toBeGreaterThan(0);
      expect(document.querySelectorAll("#edit-name")).toHaveLength(0);
      expect(document.querySelectorAll(".test-case-detail-page")).toHaveLength(0);
    });
  });
}
