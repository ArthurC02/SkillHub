import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { RUN, RUNS, TRACE_GENERAL, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const NEXT = "5c2f8e10-0b7a-4d63-9a41-7e3d2b1c9f44";
const ASKING = {
  ...TRACE_GENERAL,
  status: "succeeded",
  questions: [{ question: "要處理哪個工作表？", why: "確認來源", options: ["Sheet1", "Sheet2"] }],
};
const form = (page: Page) => page.getByRole("region", { name: "這次試跑需要你回答", exact: true });

async function fillAnswer(page: Page) {
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await form(page).getByRole("radio", { name: "Sheet2", exact: true }).check();
}

async function focusedFailure(page: Page) {
  const failure = form(page).getByRole("alert", { name: "回答送出結果", exact: true });
  await expect(failure).toContainText("無法確認回答是否已送出並開始下一次試跑");
  await expect(failure).toBeFocused();
  const position = await failure.evaluate((element) => ({
    top: element.getBoundingClientRect().top,
    headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
  }));
  expect(position.top).toBeGreaterThanOrEqual(position.headerBottom);
  return failure;
}

for (const outcome of ["network", "server", "malformed", "incomplete", "same Run"] as const) {
  test(`${outcome} continuation outcome preserves answers and uncertainty until a deliberate resend`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: unknown[] = [];
    await page.route(`**/runs/${RUN}/trace*`, async (route) => {
      if (new URL(route.request().url()).searchParams.get("mode") === "advanced")
        return route.fallback();
      reads += 1;
      if (reads > 1) await gate;
      if (reads === 2 && outcome === "server")
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      return route.fulfill({ json: ASKING });
    });
    await page.route(`**/runs/${RUN}/continuations`, (route) => {
      writes.push(route.request().postDataJSON());
      if (writes.length > 1) return route.fulfill({ status: 201, json: { run_id: NEXT } });
      if (outcome === "network") return route.abort("failed");
      if (outcome === "server")
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      if (outcome === "malformed")
        return route.fulfill({ status: 201, contentType: "application/json", body: "{" });
      return route.fulfill({ status: 201, json: outcome === "same Run" ? { run_id: RUN } : {} });
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await fillAnswer(page);
      await form(page).getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
      const failure = await focusedFailure(page);
      await expect.poll(() => reads).toBe(2);
      await expect(form(page).getByRole("radio", { name: "Sheet2", exact: true })).toBeChecked();
      const answer = form(page).getByRole("textbox", { name: "你的回答", exact: true });
      await expect(answer).toBeEditable();
      const resend = form(page).getByRole("button", {
        name: "核對後仍要重新送出回答",
        exact: true,
      });
      await expect(resend).toBeDisabled();
      await expect(
        failure.getByRole("link", { name: "另開試跑紀錄", exact: true }),
      ).toHaveAttribute("href", "/workspace/runs");
      await expect(
        failure.getByRole("link", { name: "另開試跑紀錄", exact: true }),
      ).toHaveAttribute("target", "_blank");
      await answer.fill("Sheet3");
      await expect(failure).toContainText("原始問題仍然顯示，不代表尚未收到回答");
      await expect(answer).toBeFocused();
      await page.getByRole("button", { name: "進階模式", exact: true }).click();
      await expect(answer).toHaveValue("Sheet3");
      await expect(failure).toBeVisible();
      release();
      if (outcome === "server") {
        await expect(form(page).getByRole("status")).toContainText("原始問題尚未確認");
        await expect(resend).toBeDisabled();
        await expect(answer).toHaveValue("Sheet3");
        await expect(failure).toBeVisible();
        await form(page).getByRole("button", { name: "重新確認問題", exact: true }).click();
      }
      await expect(resend).toBeEnabled();
      expect(new URL(page.url()).pathname).toBe(`/runs/${RUN}`);
      expect(writes).toEqual([{ answers: ["Sheet2"] }]);
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({ path: test.info().outputPath("continuation-unknown.png") });
      await resend.focus();
      await page.keyboard.press("Enter");
      await expect(page).toHaveURL(new RegExp(`/runs/${NEXT}$`));
      await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toBeVisible();
      expect(writes).toEqual([{ answers: ["Sheet2"] }, { answers: ["Sheet3"] }]);
    } finally {
      release();
    }
  });
}

test("a confirmed continuation navigates before the original Trace reread finishes", async ({
  page,
}) => {
  await stubPlatform(page);
  let reads = 0;
  let posts = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(`**/runs/${RUN}/trace`, async (route) => {
    reads += 1;
    if (reads > 1) await gate;
    return route.fulfill({ json: ASKING });
  });
  await page.route(`**/runs/${RUN}/continuations`, (route) => {
    posts += 1;
    return route.fulfill({ status: 201, json: { run_id: NEXT } });
  });
  try {
    await page.goto(`/runs/${RUN}`);
    await fillAnswer(page);
    await form(page).getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/runs/${NEXT}$`));
    await expect(page.getByRole("heading", { name: "試跑結果", exact: true })).toBeVisible();
    await expect.poll(() => reads).toBe(2);
    expect(posts).toBe(1);
  } finally {
    release();
  }
});

test("opening Run history preserves the unknown continuation draft and retries a failed read without another POST", async ({
  page,
}) => {
  await stubPlatform(page);
  let lists = 0;
  let posts = 0;
  await page.context().route("**/*", (route, request) => {
    if (["document", "stylesheet", "script", "font"].includes(request.resourceType()))
      return route.continue();
    const { status, body } = platformResponse(request.url());
    return route.fulfill({ status, json: body });
  });
  await page.context().route("**/runs?*", (route) => {
    lists += 1;
    if (lists === 1) return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    return route.fulfill({
      json: { runs: [{ ...RUNS.runs[0], run_id: NEXT }, ...RUNS.runs] },
    });
  });
  await page.route(`**/runs/${RUN}/trace`, (route) => route.fulfill({ json: ASKING }));
  await page.route(`**/runs/${RUN}/continuations`, (route) => {
    posts += 1;
    return route.fulfill({ status: 201, json: {} });
  });
  await page.goto(`/runs/${RUN}`);
  await fillAnswer(page);
  await form(page).getByRole("button", { name: "送出回答並繼續試跑", exact: true }).click();
  const failure = await focusedFailure(page);
  const opened = page.waitForEvent("popup");
  await failure.getByRole("link", { name: "另開試跑紀錄", exact: true }).click();
  const history = await opened;
  await expect(history.getByRole("heading", { name: "試跑活動", exact: true })).toBeVisible();
  await expect(history.getByRole("alert")).toContainText("暫時無法讀取試跑活動");
  await expect(history.locator(`main a[href="/runs/${NEXT}"]`)).toHaveCount(0);
  await history.getByRole("button", { name: "再試一次", exact: true }).click();
  await expect(history.locator(`main a[href="/runs/${NEXT}"]`)).toBeVisible();
  expect(lists).toBe(2);
  expect(posts).toBe(1);
  await history.close();
  await expect(form(page).getByRole("textbox", { name: "你的回答", exact: true })).toHaveValue(
    "Sheet2",
  );
  await expect(failure).toContainText("無法確認回答是否已送出");
  await expect(page).toHaveURL(new RegExp(`/runs/${RUN}$`));
});
