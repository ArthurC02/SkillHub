import { expect, test } from "@playwright/test";
import {
  ADMIN_AGENT_FINDINGS,
  ADMIN_AGENT_PROPOSALS,
  AGENT_REPORT_RUN,
} from "../src/testing/fixtures/platform";
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

test("an older finding stays reachable after loading more and returning from detail", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await stubPlatform(page);
  const later = {
    ...ADMIN_AGENT_FINDINGS.findings[0],
    id: "5c1d2e3f-4a5b-4c6d-8e7f-90a1b2c3d4e6",
    title: "較早的待辦",
  };
  await page.route(/\/admin\/agents\/findings(?:\?.*)?$/, (route) => {
    const cursor = new URL(route.request().url()).searchParams.get("cursor");
    return route.fulfill({
      json: cursor
        ? { findings: [later], counts: ADMIN_AGENT_FINDINGS.counts }
        : { ...ADMIN_AGENT_FINDINGS, next_cursor: "later" },
    });
  });
  await page.route(`**/admin/agents/findings/${later.id}`, (route) =>
    route.fulfill({ json: { finding: later, events: [] } }),
  );
  await page.goto("/admin/agents");

  const inbox = page.locator("#admin-agent-findings");
  await inbox.getByRole("button", { name: "載入更多待辦" }).click();
  await expect(inbox.getByText("較早的待辦")).toBeVisible();
  await inbox.getByRole("link", { name: "打開這件事" }).last().click();
  await expect(page.getByRole("heading", { name: "這件事" })).toBeVisible();
  await page.getByRole("link", { name: "回到待辦" }).click();
  await expect(inbox.getByText("較早的待辦")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("the proposal queue keeps its decision count and page across navigation and reload", async ({
  page,
}) => {
  await stubPlatform(page);
  const base = ADMIN_AGENT_PROPOSALS.proposals[0];
  await page.route(/\/admin\/agents\/proposals\?/, (route) => {
    const url = new URL(route.request().url());
    const view = url.searchParams.get("view");
    const offset = Number(url.searchParams.get("offset"));
    if (view === "closed") {
      return route.fulfill({ json: { proposals: [{ ...base, status: "expired" }], total: 1 } });
    }
    const proposals = Array.from({ length: 20 }, (_, index) => ({
      ...base,
      id:
        offset === 20 && index === 0
          ? base.id
          : `00000000-0000-4000-8000-${String(offset + index).padStart(12, "0")}`,
      reason: offset === 20 && index === 0 ? "第二頁仍有待核准提案。" : base.reason,
    }));
    return route.fulfill({ json: { proposals, total: 101 } });
  });
  await page.goto("/admin/agents");
  await expect(page.getByRole("navigation", { name: "平台 Agent 工作區" })).toContainText(
    "待核准 101 件",
  );
  const queue = page.locator("#admin-agent-proposals");
  await expect(queue.locator(".download-item")).toHaveCount(20);
  await queue.getByRole("button", { name: "下一頁" }).click();
  await expect(page).toHaveURL(/proposal_offset=20/);
  await expect(queue.getByText("第二頁仍有待核准提案。")).toBeVisible();
  await queue.getByText("第二頁仍有待核准提案。").locator("..").getByRole("link").click();
  await expect(page.getByRole("heading", { name: "這個提案" })).toBeVisible();
  await page.getByRole("link", { name: "回到提案" }).click();
  await expect(page).toHaveURL(/proposal_offset=20/);
  await expect(queue.getByText("第二頁仍有待核准提案。")).toBeVisible();
  await page.reload();
  await expect(queue.getByText("第二頁仍有待核准提案。")).toBeVisible();
  await queue.getByLabel("查看提案清單").selectOption("closed");
  await expect(page).toHaveURL(/proposal_view=closed/);
  await expect(queue).toContainText("最近七天結案：共 1 件");
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
