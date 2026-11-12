import { expect, test } from "vitest";
import {
  COMPARISON,
  OTHER_RUN,
  RUN,
  SKILL,
  SKILL_B,
  SKILL_C,
  skillDetail,
} from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

test("run comparison withholds cached decisions during refresh and recovers after failure", async () => {
  const page = app();
  let reads = 0;
  let releaseRead!: () => void;
  const held = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  page.route(`**/runs/${RUN}/comparison?*`, async () => {
    reads += 1;
    if (reads === 2) {
      await held;
      return { status: 503, json: { error: "service unavailable" } };
    }
    return {
      json: {
        ...COMPARISON,
        runs: COMPARISON.runs.map((side) => ({
          ...side,
          final_output: reads === 1 ? "舊的輸出" : "更新後的輸出",
        })),
      },
    };
  });
  await page.goto(`/runs/${RUN}/compare?against=${OTHER_RUN}`);
  const table = page.locator("table").filter({ hasText: "試跑紀錄任務判定與執行狀態對比" });
  await expect(table.getByText("舊的輸出", { exact: true })).toHaveCount(2);
  const refresh = page.getByRole("button", { name: /^更新比較/ });
  await refresh.focus();
  await page.press("Enter");
  try {
    await page.eventually(() => expect(reads).toBe(2));
    await expect(page.locator("main .verdict")).toHaveCount(0);
    await expect(table).toHaveCount(0);
    await expect(refresh).toBeFocused();
    await expect(refresh).toHaveAttribute("aria-disabled", "true");
    await page.press("Enter");
    await expect(page.getByRole("status").filter({ hasText: "載入比較中" })).toBeVisible();
  } finally {
    releaseRead();
  }
  await expect(page.getByRole("alert")).toContainText("暫時無法讀取比較結果");
  await expect(table).toHaveCount(0);
  await expect(page.locator("main .verdict")).toHaveCount(0);
  await page.getByRole("button", { name: "再試一次" }).focus();
  await page.press("Enter");
  await expect(table.getByText("更新後的輸出", { exact: true })).toHaveCount(2);
  await expect(page.locator("#run-comparison-heading")).toBeFocused();
  expect(reads).toBe(3);
  expect(await page.axeViolations("main")).toEqual([]);
});

test("skill comparison excludes a failed cached column and retries only that skill", async () => {
  const page = app();
  const ids = [SKILL, SKILL_B, SKILL_C];
  const names = ["摘要工具", "失敗項目", "翻譯工具"];
  const reads = [0, 0, 0];
  let releaseRead!: () => void;
  const held = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  for (const [index, id] of ids.entries()) {
    page.route(`**/api/skills/${id}?view=embedded`, async () => {
      reads[index] += 1;
      if (index === 1 && reads[index] === 2)
        return { status: 503, json: { error: "service unavailable" } };
      if (index === 1 && reads[index] === 3) await held;
      return { json: skillDetail(id, names[index]) };
    });
  }
  await page.goto(`/compare?ids=${ids.join(",")}`);
  const table = page.locator("table").filter({ hasText: "並排比較" });
  await expect(table.getByRole("columnheader")).toHaveText(`比較項目 ${names.join(" ")}`);
  await page.getByRole("button", { name: "更新比較資料" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "有 1 個小工具讀取失敗" })).toBeVisible();
  await expect(table.getByRole("columnheader")).toHaveText("比較項目 摘要工具 翻譯工具");
  await expect(table.getByRole("link", { name: "失敗項目", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "再試一次" }).focus();
  await page.press("Enter");
  try {
    await page.eventually(() => expect(reads).toEqual([2, 3, 2]));
    await expect(table).toHaveCount(0);
    await expect(page.locator("#skill-comparison-heading")).toBeFocused();
    await expect(page.getByRole("status").filter({ hasText: "載入中" })).toBeVisible();
  } finally {
    releaseRead();
  }
  await expect(table.getByRole("columnheader")).toHaveText(`比較項目 ${names.join(" ")}`);
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(reads).toEqual([2, 3, 2]);
  expect(await page.axeViolations("main")).toEqual([]);
});

for (const dependency of ["self", "candidates", "versions"] as const) {
  test(`comparison can retry unreadable ${dependency} without losing its ID draft`, async () => {
    const page = app();
    let fail = true;
    const path =
      dependency === "self"
        ? `**/runs/${RUN}`
        : dependency === "candidates"
          ? "**/runs?*"
          : `**/skills/${SKILL}/versions`;
    page.route(path, () =>
      fail ? { status: 503, json: { error: "service unavailable" } } : undefined,
    );
    await page.goto(`/runs/${RUN}/compare`);
    const what =
      dependency === "self"
        ? "目前這次試跑"
        : dependency === "candidates"
          ? "可比較的試跑紀錄"
          : "候選試跑紀錄的 Version 編號";
    await expect(page.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
    await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
    const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
    await field.fill("尚未送出的 ID");
    fail = false;
    await page.getByRole("button", { name: "再試一次" }).focus();
    await page.press("Enter");
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /^與這一次比較（建立於 .+）：v2，/ }),
    ).toBeVisible();
    await expect(field).toHaveValue("尚未送出的 ID");
    await expect(page.locator("#run-comparison-heading")).toBeFocused();
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
