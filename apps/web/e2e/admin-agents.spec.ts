import { expect, test } from "@playwright/test";
import { AGENT_REPORT_RUN } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test("the agent workbench opens one decision at a time on a narrow screen", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await stubPlatform(page);
  await page.goto("/admin/agents");

  const workbench = page.getByRole("navigation", { name: "平台 Agent 工作區" });
  await expect(workbench).toContainText("待核准 1 件");
  await expect(workbench).toContainText("待辦 2 件");
  const brake = page.locator("#admin-agent-brake");
  const proposals = page.locator("#admin-agent-proposals");
  await expect(brake.getByRole("button", { name: "拉下 Agent 煞車" })).toBeVisible();
  await expect(proposals).toContainText("提案清單上次取得於");
  expect((await brake.boundingBox())!.y).toBeLessThan((await proposals.boundingBox())!.y);
  await page.getByRole("link", { name: "打開這個提案" }).click();
  await expect(page.getByRole("heading", { name: "這個提案" })).toBeVisible();
  await expect(workbench).toHaveCount(0);
  await expect(page.getByRole("button", { name: "核准並執行" })).toBeVisible();

  await page.getByRole("link", { name: "回到提案" }).click();
  await expect(workbench).toBeVisible();
  await page.getByRole("link", { name: "打開這件事" }).click();
  await expect(page.getByRole("heading", { name: "這件事" })).toBeVisible();
  await expect(workbench).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("a linked agent run shows its report before the collapsed raw steps", async ({ page }) => {
  await stubPlatform(page);
  await page.goto(`/admin/agents?run=${AGENT_REPORT_RUN}`);

  await expect(page.getByRole("heading", { name: "這次執行", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "需要注意：1 項" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "平台 Agent 工作區" })).toHaveCount(0);
  await expect(page.locator(".agent-step-raw")).toHaveCount(2);
  await expect(page.locator(".agent-step-raw[open]")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "回到執行紀錄" })).toBeVisible();
});

test("an older agent run link still shows its report outside the recent list", async ({ page }) => {
  await stubPlatform(page);
  await page.route("**/admin/agents/runs", (route) =>
    route.fulfill({ json: { runs: [], total: 73 } }),
  );
  await page.goto(`/admin/agents?run=${AGENT_REPORT_RUN}`);

  await expect(page.getByRole("heading", { name: "需要注意：1 項" })).toBeVisible();
  await expect(page.getByText("分割表輪替從來沒有成功過，已經超過兩個週期。")).toBeVisible();
});

test("the desktop workbench shows decisions and findings side by side without horizontal overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await stubPlatform(page);
  await page.goto("/admin/agents");
  const proposals = page.locator("#admin-agent-proposals");
  const findings = page.locator("#admin-agent-findings");
  await expect(proposals.getByRole("link", { name: "打開這個提案" })).toBeVisible();
  await expect(findings.getByRole("link", { name: "打開這件事" })).toBeVisible();
  const proposalBox = (await proposals.boundingBox())!;
  const findingBox = (await findings.boundingBox())!;
  expect(findingBox.x).toBeGreaterThan(proposalBox.x);
  expect(Math.abs(findingBox.y - proposalBox.y)).toBeLessThan(24);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("destructive approval discloses scope before the final decision", async ({ page }) => {
  await stubPlatform(page);
  await page.goto("/admin/agents");
  await page.getByRole("link", { name: "打開這個提案" }).click();

  const ask = page.getByRole("button", { name: "核准並執行" });
  await expect(ask).toBeDisabled();
  await page.getByLabel("理由（必填，會寫進動作紀錄）").first().fill("run overdue maintenance");
  await ask.click();
  const confirm = page.getByRole("button", { name: "確認核准這個提案" });
  await expect(confirm).toBeFocused();
  await expect(confirm).toHaveAttribute("aria-describedby", "admin-proposal-approve-scope");
  await expect(page.locator("#admin-proposal-approve-scope")).toContainText(
    "會刪除的 Trace 事件：1834 筆",
  );
  await expect(page.locator("#admin-proposal-approve-scope")).toContainText("其他待審提案不受影響");
  await page.getByRole("button", { name: "取消" }).click();
  await expect(ask).toBeFocused();
  await expect(confirm).toHaveCount(0);
});
