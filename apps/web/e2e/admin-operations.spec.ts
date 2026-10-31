import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import {
  ADMIN_AGENT_PROPOSAL,
  ADMIN_AGENT_PROPOSALS,
  ADMIN_DISPATCH,
  ADMIN_EXPOSURE_CASE,
  ADMIN_MODEL_BUDGETS,
  ADMIN_RUN_SETTINGS,
  ADMIN_SKILLS,
  AGENT_PROPOSAL,
  PUBLICATION,
  PUBLISHER,
  SKILL,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import type { RunSettings } from "../src/features/admin/admin.service";

const RUN_SETTING_DRAFTS = [
  {
    name: "spend ceiling",
    key: "model_budget",
    field: "credits",
    next: 260,
    original: "650",
    edited: "300",
    selector: "#admin-settings-run-budget-value",
    note: "#admin-settings-run-budget-note",
    heading: "每次試跑的模型花費上限",
    summary: "調整花費上限",
    submit: "改上限",
    reset: "回到部署預設 650 點",
    view: "檢視目前的花費上限",
    endpoint: "/admin/settings/runs/model-budget",
    warning: "設定已變更；草稿改為目前的 260 點，請確認後再送出。",
    receipt: "已改為 260 點，新建立的試跑就照這個上限。",
  },
  {
    name: "token rate",
    key: "tokens_per_minute",
    field: "limit",
    next: 20000,
    original: "200000",
    edited: "30000",
    selector: "#admin-settings-run-tpm-value",
    note: "#admin-settings-run-tpm-note",
    heading: "試跑模型金鑰的每分鐘 tokens 上限",
    summary: "調整每分鐘 tokens 上限",
    submit: "改每分鐘上限",
    reset: "回到部署預設每分鐘 200,000 tokens",
    view: "檢視目前的每分鐘 tokens 上限",
    endpoint: "/admin/settings/runs/tokens-per-minute",
    warning: "設定已變更；草稿改為每分鐘 20,000 tokens，請確認後再送出。",
    receipt: "已改為每分鐘 20,000 tokens，之後發出的金鑰就照這個值。",
  },
] as const;

async function controlRunSetting(
  page: Page,
  scope: (typeof RUN_SETTING_DRAFTS)[number],
  overridden: boolean,
) {
  const snapshot = (value: number, reason?: string): RunSettings => ({
    ...ADMIN_RUN_SETTINGS,
    [scope.key]: { ...ADMIN_RUN_SETTINGS[scope.key], [scope.field]: value, reason },
  });
  let current: RunSettings = overridden
    ? snapshot(scope.next, "temporary restriction")
    : ADMIN_RUN_SETTINGS;
  const writes: { method: string; body: Record<string, unknown> }[] = [];
  let heldRead: Promise<void> | undefined;
  await page.route("**/admin/settings/runs", async (route) => {
    if (heldRead) await heldRead;
    return route.fulfill({ json: current });
  });
  await page.route(`**${scope.endpoint}`, (route) => {
    const method = route.request().method();
    const body = route.request().postDataJSON();
    writes.push({ method, body });
    current =
      method === "DELETE" ? ADMIN_RUN_SETTINGS : snapshot(Number(body[scope.field]), body.note);
    return route.fulfill({ json: current });
  });
  const refresh = page.getByRole("button", { name: /^重新整理(?:設定|中…)$/ });
  return {
    writes,
    custom: (reason: string) => {
      current = snapshot(scope.next, reason);
    },
    defaults: () => {
      current = ADMIN_RUN_SETTINGS;
    },
    defaultOverride: () => {
      current = snapshot(Number(scope.original), "explicit default-sized override");
    },
    reread: async () => {
      let release!: () => void;
      heldRead = new Promise<void>((resolve) => {
        release = resolve;
      });
      const response = page.waitForResponse((reply) =>
        reply.url().endsWith("/admin/settings/runs"),
      );
      try {
        await refresh.click();
        await expect(refresh).toHaveText("重新整理中…");
        await expect(refresh).toBeDisabled();
      } finally {
        release();
        await response;
        heldRead = undefined;
      }
      await expect(refresh).toBeEnabled();
      await expect(refresh).toHaveText("重新整理設定");
    },
  };
}

for (const scope of RUN_SETTING_DRAFTS) {
  for (const width of [375, 1280]) {
    for (const phase of ["refresh", "restore", "same-value restore", "saved"] as const) {
      test(`run setting ${scope.name} draft stays current through ${phase} at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        const { writes, reread, custom, defaults, defaultOverride } = await controlRunSetting(
          page,
          scope,
          phase === "restore",
        );
        if (phase === "same-value restore") defaultOverride();
        await page.goto("/admin/settings");
        await expect(page.locator(".auth-name")).toHaveText("tester");
        const region = page.getByRole("region", { name: scope.heading, exact: true });
        await region.getByText(scope.summary, { exact: true }).click();
        const input = page.locator(scope.selector);
        const refresh = page.getByRole("button", { name: "重新整理設定", exact: true });
        if (phase === "refresh") {
          custom("another operator changed the setting");
          await reread();
          await expect(input).toHaveValue(String(scope.next));
          await expect(region.getByText(scope.warning, { exact: true })).toBeVisible();
          await input.fill(scope.edited);
          await reread();
          await expect(refresh).toBeEnabled();
          await expect(input).toHaveValue(scope.edited);
          defaults();
          await reread();
          await expect(input).toHaveValue(scope.original);
          custom("changed back to the preceding value");
          await reread();
          await expect(input).toHaveValue(String(scope.next));
          await input.fill("");
          await reread();
          await expect(refresh).toBeEnabled();
          await expect(input).toHaveValue("");
          expect(writes).toEqual([]);
        } else if (phase === "restore" || phase === "same-value restore") {
          if (phase === "same-value restore") await input.fill(scope.edited);
          await page
            .locator(scope.note.replace("-note", "-clear-note"))
            .fill("restore deployment defaults");
          await region.getByRole("button", { name: scope.reset, exact: true }).click();
          const receipt = region.getByRole("status").filter({ hasText: "已回到部署預設" });
          await expect(receipt).toBeFocused();
          await expect(receipt).toBeInViewport({ ratio: 1 });
          await expect(input).toHaveValue(scope.original);
          const editor = region.locator("details").filter({ has: input });
          if ((await editor.getAttribute("open")) === null) await editor.locator("summary").click();
          await expect(input).toBeVisible();
          await page.locator(scope.note).fill("confirm the displayed deployment default");
          await region.getByRole("button", { name: scope.submit, exact: true }).click();
          await expect(region.getByRole("status").filter({ hasText: "已改為" })).toBeFocused();
          expect(writes).toEqual([
            { method: "DELETE", body: { note: "restore deployment defaults" } },
            {
              method: "PUT",
              body: {
                [scope.field]: Number(scope.original),
                note: "confirm the displayed deployment default",
              },
            },
          ]);
          await region.scrollIntoViewIfNeeded();
          const report = await new AxeBuilder({ page })
            .include(`section.admin-settings-policy:has(${scope.selector})`)
            .analyze();
          await writeFile(info.outputPath("run-setting-draft-a11y.json"), JSON.stringify(report));
          expect(report.violations).toEqual([]);
          expect(report.incomplete).toEqual([]);
        } else {
          await input.fill(String(scope.next));
          await page.locator(scope.note).fill("lower the limit while measuring");
          await region.getByRole("button", { name: scope.submit, exact: true }).click();
          await expect
            .poll(() => writes)
            .toEqual([
              {
                method: "PUT",
                body: { [scope.field]: scope.next, note: "lower the limit while measuring" },
              },
            ]);
          await expect(region.getByText(scope.receipt, { exact: true })).toBeFocused();
          await expect(input).toHaveValue(String(scope.next));
          await expect(region.getByText(scope.warning, { exact: true })).toHaveCount(0);
          defaults();
          await reread();
          await region.getByRole("button", { name: scope.view, exact: true }).click();
          await expect(input).toHaveValue(scope.original);
          expect(writes).toEqual([
            {
              method: "PUT",
              body: { [scope.field]: scope.next, note: "lower the limit while measuring" },
            },
          ]);
        }
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await region.screenshot({ path: info.outputPath("run-setting-draft.png") });
      });
    }
  }
}

for (const width of [375, 1280]) {
  test(`exposure refresh replaces stale approval drafts at ${width}px`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    let reads = 0;
    let finishRead!: () => void;
    const held = new Promise<void>((resolve) => {
      finishRead = resolve;
    });
    await page.route(
      `**/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`,
      async (route) => {
        reads += 1;
        if (reads === 1) return route.fulfill({ json: ADMIN_EXPOSURE_CASE });
        await held;
        return route.fulfill({
          json: {
            ...ADMIN_EXPOSURE_CASE,
            sequence: 3,
            approval: {
              allowed: false,
              refusal: { reason: "snapshot_pending", error: "這一版的搜尋內容還在補充" },
            },
            snapshot: {
              ...ADMIN_EXPOSURE_CASE.snapshot,
              enriched: false,
              enriched_summary: "新版本摘要",
              digest: "sha256:snap-cc",
            },
          },
        });
      },
    );
    await page.goto(
      `/admin/exposure?publication=${encodeURIComponent(`${PUBLISHER}/${PUBLICATION}`)}`,
    );
    const approval = page.getByRole("radio", { name: "核准", exact: true });
    await approval.check();
    await page.locator("#admin-exposure-review-note").fill("old snapshot decision");
    await expect(page.getByRole("button", { name: "送出核准" })).toBeEnabled();
    const refresh = page.getByRole("button", { name: "重新整理審核資料" });
    await refresh.focus();
    await page.keyboard.press("Enter");
    try {
      await expect(
        page.getByRole("status").filter({ hasText: "載入這一筆的曝光審核資料中" }),
      ).toBeVisible();
      await expect(approval).toHaveCount(0);
      await expect(page.getByRole("button", { name: "送出核准" })).toHaveCount(0);
      await expect(
        page.getByText("把 PDF 整理成重點摘要，附上引用頁碼。", { exact: true }),
      ).toHaveCount(0);
      await expect(page.getByText("審核資料上次取得於", { exact: false })).toHaveCount(0);
      await page.keyboard.press("Enter");
      expect(reads).toBe(2);
      await page.screenshot({ path: testInfo.outputPath(`exposure-refreshing-${width}.png`) });
    } finally {
      finishRead();
    }
    await expect(page.getByText("新版本摘要", { exact: true })).toBeVisible();
    await expect(approval).toBeDisabled();
    await expect(approval).not.toBeChecked();
    await expect(page.locator("#admin-exposure-review-note")).toHaveValue("");
    await expect(page.getByRole("button", { name: "送出審核結論" })).toBeDisabled();
    await expect(page.getByText("這一版的搜尋內容還在補充", { exact: true })).toBeVisible();
    await expect(refresh).toBeFocused();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    const accessibility = await new AxeBuilder({ page }).include("main").analyze();
    expect(accessibility.violations).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath(`exposure-recovered-${width}.png`),
      fullPage: true,
    });
  });
}

test("governance refresh and recovery withhold stale decisions on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  let finishRead!: () => void;
  let held = new Promise<void>((resolve) => {
    finishRead = resolve;
  });
  await page.route("**/admin/skills?*", async (route) => {
    if (route.request().isNavigationRequest()) return route.fallback();
    reads += 1;
    if (reads === 1) return route.fulfill({ json: ADMIN_SKILLS });
    await held;
    return reads === 2
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({
          json: {
            skills: [
              {
                ...ADMIN_SKILLS.skills[0],
                takedown_at: "2026-09-11T10:00:00Z",
                takedown_reason: "DMCA notice",
              },
            ],
          },
        });
  });
  await page.goto(`/admin/skills?q=${SKILL}`);
  const actions = page.getByRole("heading", { name: "對「PDF Summariser」的動作" });
  await expect(actions).toBeVisible();
  await page.locator("#admin-skill-takedown summary").click();
  await page.locator("#admin-takedown-reason").fill("old decision");
  await page.getByRole("button", { name: "下架", exact: true }).click();
  await expect(page.getByRole("button", { name: "確認下架" })).toBeVisible();
  const refresh = page.getByRole("button", { name: "重新整理治理狀態" });
  await refresh.focus();
  await page.keyboard.press("Enter");
  try {
    await expect(
      page.getByRole("status").filter({ hasText: "載入小工具治理狀態中" }),
    ).toBeVisible();
    await expect(actions).toHaveCount(0);
    await expect(page.getByRole("button", { name: "確認下架" })).toHaveCount(0);
    await expect(page.getByText("未下架", { exact: true })).toHaveCount(0);
    await expect(page.getByText("治理狀態上次取得於", { exact: false })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "重新整理中…" })).toBeDisabled();
    await page.keyboard.press("Enter");
    expect(reads).toBe(2);
    await page.screenshot({ path: testInfo.outputPath("governance-refreshing-phone.png") });
  } finally {
    finishRead();
  }
  await expect(page.getByRole("alert")).toContainText("請重新整理治理狀態後再操作");
  await expect(refresh).toBeFocused();
  await page.screenshot({ path: testInfo.outputPath("governance-read-failed-phone.png") });
  held = new Promise<void>((resolve) => {
    finishRead = resolve;
  });
  await page.keyboard.press("Enter");
  try {
    await expect.poll(() => reads).toBe(3);
    await expect(page.getByRole("alert")).toHaveCount(0);
    await expect(actions).toHaveCount(0);
    await expect(
      page.getByRole("status").filter({ hasText: "載入小工具治理狀態中" }),
    ).toBeVisible();
  } finally {
    finishRead();
  }
  await expect(page.getByText("已下架", { exact: true })).toBeVisible();
  await expect(page.getByText("DMCA notice", { exact: false })).toBeVisible();
  await expect(page.locator("#admin-skill-takedown")).toHaveCount(0);
  await expect(page.locator("#admin-skill-lift-takedown")).toBeVisible();
  await expect(page.getByRole("button", { name: "確認下架" })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("governance-recovered-phone.png"),
    fullPage: true,
  });
});

test("the home priority conclusion waits for every refreshed source on a phone", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await stubPlatform(page);
  await page.route("**/admin/dispatch", (route) =>
    route.fulfill({ json: { dispatching: true, halts: [] } }),
  );
  await page.route("**/admin/exposure-reviews", (route) =>
    route.fulfill({ json: { publications: [] } }),
  );
  await page.route("**/admin/agents/findings", (route) =>
    route.fulfill({
      json: {
        findings: [],
        counts: { open: 0, acknowledged: 0, resolved: 0, dismissed: 0, recovered: 0 },
      },
    }),
  );
  let reads = 0;
  let finishRead!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRead = resolve;
  });
  await page.route("**/admin/agents/proposals?*", async (route) => {
    reads += 1;
    if (reads === 2) {
      await held;
      return route.fulfill({ json: ADMIN_AGENT_PROPOSALS });
    }
    if (reads === 3) {
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    }
    return route.fulfill({ json: { proposals: [], total: 0 } });
  });
  await page.goto("/admin");
  const priorities = page.getByRole("region", { name: "目前需留意" });
  const summary = priorities.getByRole("status");
  await expect(summary).toHaveText("這四項目前沒有需留意的狀態。");
  const refresh = priorities.getByRole("button", { name: "重新整理狀態" });
  try {
    await refresh.focus();
    await page.keyboard.press("Enter");
    await expect(summary).toHaveAttribute("aria-busy", "true");
    await expect(summary).toHaveText("正在更新四項狀態；完成後再判斷是否有待處理事項。");
    await expect(priorities.getByRole("button", { name: "更新中…" })).toBeDisabled();
    await expect(priorities.getByRole("link")).toHaveCount(4);
    await expect(summary).toBeInViewport();
    await page.screenshot({ path: testInfo.outputPath("home-priorities-refreshing-phone.png") });
  } finally {
    finishRead();
  }
  await expect(summary).toHaveAttribute("aria-busy", "false");
  await expect(summary).toHaveText("目前有 1 類狀態需留意。");
  await refresh.click();
  await expect(priorities.getByRole("alert")).toContainText("1 項狀態無法取得");
  await expect(summary).toHaveCount(0);
  await expect(priorities.getByRole("link", { name: "平台 Agent 提案 無法取得" })).toBeVisible();
  await refresh.click();
  await expect(summary).toHaveText("這四項目前沒有需留意的狀態。");
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(reads).toBe(4);
});

test("platform policies lead with both current settings and reveal an editor by keyboard", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 900 });
  await stubPlatform(page);
  await page.route("**/admin/settings/runs/continuation-rounds", async (route) => {
    expect(route.request().postDataJSON()).toEqual({ rounds: 5, note: "longer interviews" });
    await route.fulfill({
      json: { ...ADMIN_RUN_SETTINGS, continuation_rounds: { rounds: 5, min: 1, max: 10 } },
    });
  });
  await page.goto("/admin/settings");
  await expect(page.getByText("關閉：一個判定（預設）", { exact: true })).toBeInViewport();
  await expect(page.getByText("目前：3 輪（可設 1–10 輪）", { exact: true })).toBeInViewport();
  await expect(page.locator("#admin-settings-rounds-value")).toBeHidden();
  const edit = page.getByText("調整回答輪數上限", { exact: true });
  await edit.focus();
  await page.keyboard.press("Enter");
  await page.locator("#admin-settings-rounds-value").fill("5");
  await page.locator("#admin-settings-rounds-note").fill("longer interviews");
  await page.getByRole("button", { name: "改輪數", exact: true }).click();
  await expect(page.locator("#admin-settings-rounds-result")).toBeFocused();
  await expect(page.locator("#admin-settings-rounds-result")).toContainText("已改為 5 輪");
  const report = await new AxeBuilder({ page }).analyze();
  expect(report.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

for (const policy of ["judge panel", "answer rounds"] as const) {
  test(`a changed ${policy} can be reviewed after a failed reread without losing keyboard context`, async ({
    page,
  }, testInfo) => {
    const judge = policy === "judge panel";
    const id = judge ? "admin-settings-judge-panel" : "admin-settings-rounds";
    const editLabel = judge ? "調整評審團" : "調整回答輪數上限";
    const reviewLabel = judge ? "檢視目前的評審團設定" : "檢視目前的回答輪數";
    let unavailable = false;
    let writes = 0;
    await page.setViewportSize({ width: 375, height: 900 });
    await stubPlatform(page);
    await page.route(judge ? "**/admin/settings" : "**/admin/settings/runs", (route) => {
      if (route.request().isNavigationRequest() || !unavailable) return route.fallback();
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    });
    await page.route(
      judge ? "**/admin/settings/judge-panel" : "**/admin/settings/runs/continuation-rounds",
      (route) => {
        writes += 1;
        expect(route.request().postDataJSON()).toEqual(
          judge ? { enabled: true, note: "first decision" } : { rounds: 5, note: "first decision" },
        );
        return route.fulfill({
          json: judge
            ? { judge_panel: { enabled: true } }
            : { ...ADMIN_RUN_SETTINGS, continuation_rounds: { rounds: 5, min: 1, max: 10 } },
        });
      },
    );
    await page.goto("/admin/settings");
    await page.getByText(editLabel, { exact: true }).focus();
    await page.keyboard.press("Enter");
    if (!judge) await page.locator(`#${id}-value`).fill("5");
    await page.locator(`#${id}-note`).fill("first decision");
    await page.getByRole("button", { name: judge ? "開啟評審團" : "改輪數", exact: true }).click();
    const result = page.locator(`#${id}-result`);
    await expect(result).toBeFocused();
    await page.getByRole("button", { name: "重新整理設定" }).click();
    await expect(result).toContainText("目前設定尚未確認");
    await expect(page.locator(`#${id}-note`)).toHaveCount(0);
    await result.scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath(`${policy}-unverified-phone.png`) });

    unavailable = true;
    await page.getByRole("button", { name: "重新整理設定" }).click();
    await expect(page.getByRole("alert")).toContainText(
      judge ? "暫時無法讀取平台設定" : "暫時無法讀取試跑設定",
    );
    await expect(page.getByRole("button", { name: reviewLabel })).toHaveCount(0);
    unavailable = false;
    await page.getByRole("button", { name: judge ? "再讀取平台設定" : "再讀取試跑設定" }).click();
    await page.getByRole("button", { name: reviewLabel }).focus();
    await page.keyboard.press("Enter");
    const heading = page.locator(`#${id}-heading`);
    await expect(heading).toBeFocused();
    await expect(heading).toBeInViewport();
    await expect(result).toHaveCount(0);
    await page.getByText(editLabel, { exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(page.locator(`#${id}-note`)).toBeVisible();
    await expect(page.locator(`#${id}-note`)).toHaveValue("");
    await expect(page.locator(judge ? `#${id}-note` : `#${id}-value`)).toHaveValue(
      judge ? "" : "3",
    );
    expect(writes).toBe(1);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    const accessibility = await new AxeBuilder({ page }).include("main").analyze();
    expect(accessibility.violations).toEqual([]);
    await page.screenshot({ path: testInfo.outputPath(`${policy}-recovered-phone.png`) });
  });
}

test("an Agent proposal read can recover in place on a phone", async ({ page }, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  let finishRetry!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRetry = resolve;
  });
  await page.route(`**/admin/agents/proposals/${AGENT_PROPOSAL}`, async (route) => {
    reads += 1;
    if (reads === 1) {
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    }
    await held;
    return route.fulfill({ json: ADMIN_AGENT_PROPOSAL });
  });
  await page.goto(`/admin/agents?proposal=${AGENT_PROPOSAL}`);

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取這個提案");
  await expect(page.getByRole("button", { name: "再試一次" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "會發生什麼" })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("agent-proposal-read-failed-phone.png") });
  try {
    await page.getByRole("button", { name: "再試一次" }).click();
    await expect(page.getByRole("status")).toContainText("載入這個提案中");
    await expect(page.getByRole("button", { name: "再試一次" })).toHaveCount(0);
    expect(reads).toBe(2);
  } finally {
    finishRetry();
  }
  await expect(page.getByRole("heading", { name: "會發生什麼" })).toBeVisible();
  await expect(page.getByRole("button", { name: "再試一次" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "這個提案" })).toBeFocused();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("agent-proposal-recovered-phone.png") });
});

test("retrying a failed proposal list does not expose stale decisions", async ({ page }) => {
  await stubPlatform(page);
  let reads = 0;
  let finishRetry!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRetry = resolve;
  });
  await page.route("**/admin/agents/proposals?*", async (route) => {
    reads += 1;
    if (reads === 2) {
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    }
    if (reads === 3) await held;
    return route.fulfill({ json: ADMIN_AGENT_PROPOSALS });
  });
  await page.goto("/admin/agents");
  const proposals = page.locator("#admin-agent-proposals");
  await expect(proposals.getByText("打開這個提案")).toBeVisible();
  await proposals.getByRole("button", { name: "重新整理" }).click();
  await expect(proposals.getByRole("alert")).toContainText("暫時無法讀取提案");
  await expect(proposals.getByText("打開這個提案")).toHaveCount(0);

  try {
    await proposals.getByRole("button", { name: "再試一次" }).click();
    await expect.poll(() => reads).toBe(3);
    await expect(proposals.getByText("打開這個提案")).toHaveCount(0);
    await expect(proposals.getByRole("button", { name: "重新讀取中…" })).toBeDisabled();
  } finally {
    finishRetry();
  }
  await expect(proposals.getByText("打開這個提案")).toBeVisible();
});

test("a release in progress blocks another dispatch decision on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let releaseRequest = 0;
  let finishRelease!: () => void;
  const held = new Promise<void>((resolve) => {
    finishRelease = resolve;
  });
  await page.route("**/admin/dispatch/halt", async (route) => {
    if (route.request().method() !== "DELETE") return route.fallback();
    releaseRequest += 1;
    await held;
    await route.fulfill({ status: 204, body: "" });
  });
  await page.goto("/admin/dispatch");
  await expect(page.locator("#admin-dispatch-lift")).toBeVisible();
  expect(
    await page
      .locator("#admin-dispatch-lift")
      .evaluate((element) => element.getBoundingClientRect().top),
  ).toBeLessThan(
    await page
      .locator("#admin-dispatch-declare")
      .evaluate((element) => element.getBoundingClientRect().top),
  );
  await page.screenshot({
    path: testInfo.outputPath("dispatch-recovery-first-phone.png"),
    fullPage: true,
  });
  await page.locator("#admin-halt-lift-target").selectOption("pool");
  await page.locator("#admin-halt-lift-note").fill("incident resolved");
  await page.locator("#admin-halt-declare-note").fill("new incident");
  await expect(page.locator("#admin-halt-declare-scope")).toContainText("本次停止範圍：整個叢集");
  await page.getByRole("button", { name: "恢復派送" }).click();
  await page.getByRole("button", { name: "確認恢復派送" }).click();

  try {
    await expect.poll(() => releaseRequest).toBe(1);
    await expect(page.getByRole("button", { name: "停止派送" })).toBeDisabled();
    await expect(page.getByText("正在解除煞車，完成後才能停止派送。")).toBeVisible();
    await expect(page.getByRole("button", { name: "停止派送" })).toHaveAttribute(
      "aria-describedby",
      "admin-halt-declare-why",
    );
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: testInfo.outputPath("dispatch-release-phone.png"),
    });
  } finally {
    finishRelease();
  }
  await expect(page.getByRole("button", { name: "停止派送" })).toBeEnabled();
});

test("a release result stays reachable when dispatch status cannot be reread on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/admin/dispatch", (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    reads += 1;
    return route.fulfill(
      reads === 1
        ? { json: ADMIN_DISPATCH }
        : reads === 2
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: { dispatching: true, halts: [] } },
    );
  });
  await page.route("**/admin/dispatch/halt", (route) => route.fulfill({ status: 204, body: "" }));
  await page.goto("/admin/dispatch");
  await page.locator("#admin-halt-lift-target").selectOption("pool");
  await page.locator("#admin-halt-lift-note").fill("incident resolved");
  await page.getByRole("button", { name: "恢復派送" }).click();
  await page.getByRole("button", { name: "確認恢復派送" }).click();

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取派送狀態");
  const result = page.locator("#admin-dispatch-lift-result");
  await expect(result).toContainText("暫勿假定已恢復派送");
  await expect(result).toHaveClass(/notice-warning/);
  await expect(result).toBeFocused();
  await expect(page.getByText("sandbox escape suspected on node-2")).toHaveCount(0);
  await expect(page.locator('section[aria-labelledby="admin-dispatch-lift"]')).toHaveCount(1);
  await expect(page.locator('section[aria-labelledby="admin-dispatch-lift"]')).toBeHidden();
  await expect(page.locator("#admin-halt-lift-target")).toBeDisabled();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("dispatch-release-reread-failed-phone.png") });

  await page.getByRole("button", { name: "重新整理派送狀態" }).click();
  await expect(page.getByText("沒有生效中的煞車。")).toBeVisible();
  await expect(page.locator("#admin-dispatch-lift")).toHaveCount(0);
  await expect(result).toHaveClass(/notice-success/);
  await expect(result).toContainText("已不在生效中的煞車清單");
});

test("restoring a model timeout also resets its editable draft on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let restored = false;
  await page.route("**/admin/model-budgets", (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return route.fulfill({
      json: {
        budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
          budget.kind === "judge-run" && restored
            ? { ...budget, seconds: null, reason: null, set_at: null }
            : budget,
        ),
      },
    });
  });
  await page.route("**/admin/model-budgets/judge-run", (route) => {
    if (route.request().method() !== "DELETE") return route.fallback();
    restored = true;
    return route.fulfill({ status: 200, json: {} });
  });
  await page.goto("/admin/model-budgets");
  await page.locator("#admin-budget-judge-run-set summary").click();
  await expect(page.locator("#admin-budget-judge-run-seconds")).toHaveValue("90");
  await page.locator("#admin-budget-judge-run-clear summary").click();
  await page.locator("#admin-budget-judge-run-clear-note").fill("restore the default");
  await page.getByRole("button", { name: "把 評估判定 改回預設" }).click();

  await expect(page.getByText("目前：預設 130 秒")).toBeVisible();
  await expect(page.locator("#admin-budget-judge-run-seconds")).toHaveValue("130");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("model-timeout-default-phone.png"),
  });
});

test("a refreshed model timeout replaces a stale editable draft on a phone", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let currentSeconds = 90;
  await page.route("**/admin/model-budgets", (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return route.fulfill({
      json: {
        budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
          budget.kind === "judge-run" ? { ...budget, seconds: currentSeconds } : budget,
        ),
      },
    });
  });
  await page.goto("/admin/model-budgets");
  await page.locator("#admin-budget-judge-run-set summary").click();
  await page.locator("#admin-budget-judge-run-seconds").fill("100");

  currentSeconds = 120;
  await page.getByRole("button", { name: "重新整理" }).click();
  await expect(page.getByText("目前：120 秒（已調整）")).toBeVisible();
  await expect(page.locator("#admin-budget-judge-run-seconds")).toHaveValue("120");
  await expect(page.getByText("設定已變更；草稿改為最新的 120 秒，請確認後再送出。")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("a credit correction describes its deduction on a phone", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route("**/admin/credits/f2222222-2222-4222-8222-222222222222/grants", (route) =>
    route.fulfill({
      json: {
        workspace_id: "f2222222-2222-4222-8222-222222222222",
        balance_credits: 90,
        amount_credits: -30,
      },
    }),
  );
  await page.goto("/admin/accounts");
  await page.getByLabel("Email", { exact: true }).fill("member@example.com");
  await page.getByLabel("Email", { exact: true }).press("Enter");
  await page.locator("#admin-grant-amount").fill("-30");
  await page.locator("#admin-grant-note").fill("corrects an over-grant");

  await expect(page.getByRole("heading", { name: "更正點數" })).toBeVisible();
  await page.getByRole("button", { name: "扣減點數" }).click();
  await expect(page.getByText("已扣減 30 點，餘額現在是 90 點。")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
