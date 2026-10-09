import { test, expect, type Page, type TestInfo } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import {
  ADMIN_AGENTS,
  ADMIN_AGENT_FINDING,
  ADMIN_AGENT_PROPOSAL,
  ADMIN_AGENT_RUNS,
  ADMIN_ACCOUNT,
  ADMIN_COST_STATISTICS,
  ADMIN_EXPOSURE_CASE,
  ADMIN_LEDGER,
  ADMIN_MODEL_BUDGETS,
  ADMIN_ROSTERS,
  ADMIN_SKILLS,
  AGENT_FINDING,
  AGENT_FAILED_RUN,
  AGENT_PROPOSAL,
  AGENT_REPORT_RUN,
  ARTIFACT,
  CATALOG,
  OTHER_RUN,
  PUBLICATION,
  PUBLISHER,
  RUN,
  RUNS,
  SEARCH,
  SKILL,
  SKILL_B,
  SKILL_VERSIONS,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { PHONE_ROUTES, ROUTES } from "./routes";
import { stubPlatform } from "./stub";

async function stubCreationContinuations(page: Page) {
  await stubPlatform(page);
  await page.route("**/me", async (route) => {
    const { body, status } = platformResponse(route.request().url());
    await route.fulfill({
      status,
      json: {
        ...(body as object),
        features: { generate_skill: true, creation_skill: true },
      },
    });
  });
  await page.route("**/creation-sessions", async (route) => {
    const sessions = [
      ["11111111-1111-4111-8111-111111111111", "整理採購文件", "waiting_input"],
      ["22222222-2222-4222-8222-222222222222", "檢查摘要品質", "working"],
      ["33333333-3333-4333-8333-333333333333", "修正流程圖", "needs_reupload"],
    ].map(([id, brief, state], index) => ({
      id,
      revision: 1,
      state,
      snapshot: { brief },
      created_at: "2026-09-28T10:00:00Z",
      updated_at: `2026-09-28T1${3 - index}:00:00Z`,
      expires_at: "2099-09-29T14:00:00Z",
      deadline: "2099-09-28T14:00:00Z",
    }));
    await route.fulfill({ json: sessions });
  });
}

async function verifyCreationContinuationLayout(page: Page, testInfo: TestInfo) {
  for (const [name, width] of [
    ["desktop", 1280],
    ["phone", 375],
  ] as const) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/workspace");
    await expect(page.getByRole("heading", { name: "繼續進行" })).toBeVisible();
    await expect(page.locator(".workspace-home h2")).toHaveText([
      "需要留意",
      "繼續進行",
      "執行中",
      "發佈成果",
      "你的資產",
    ]);
    const publication = page.locator(".workspace-home-section").filter({
      has: page.getByRole("heading", { name: "發佈成果" }),
    });
    await expect(publication.getByRole("link", { name: "v2" })).toHaveAttribute(
      "href",
      `/skills/${SKILL}/versions/${VERSION}`,
    );
    await expect(publication.getByRole("link", { name: "管理發佈" })).toHaveAttribute(
      "href",
      `/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`,
    );
    await expect(publication.getByText("已列入 Catalog", { exact: true })).toBeVisible();
    await expect(publication.getByRole("link", { name: "查看全部發佈" })).toHaveAttribute(
      "href",
      "/workspace/downloads",
    );
    await expect(page.locator(".workspace-home-grid .workspace-home-list > li")).toHaveCount(3);
    await expect(page.getByRole("link", { name: "開啟會話" })).toHaveAttribute(
      "href",
      "/workspace/creations?session=11111111-1111-4111-8111-111111111111",
    );
    const pageWidth = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);
    if (name === "desktop") {
      const cards = await page
        .locator(".workspace-home-grid > .workspace-home-section")
        .evaluateAll((elements) =>
          elements.map((element) => element.getBoundingClientRect().height),
        );
      expect(cards[0]).toBeGreaterThan((cards[1] ?? 0) + 100);
    }
    await page.screenshot({
      path: testInfo.outputPath(`workspace-home-${name}.png`),
      fullPage: true,
    });
  }
}

async function verifyReducedMotion(page: Page) {
  await stubPlatform(page);
  await page.goto("/");
  const card = page.locator(".catalog-skill-card").first();
  await expect(card).toBeVisible();

  const normal = await page.evaluate(() => {
    const value = getComputedStyle(document.documentElement).getPropertyValue("--dur-fast").trim();
    return value.endsWith("ms") ? Number.parseFloat(value) : Number.parseFloat(value) * 1000;
  });
  expect(normal).toBe(120);
  const normalTransitions = await card.evaluate((element) =>
    getComputedStyle(element).transitionDuration.split(", "),
  );
  expect(normalTransitions.some((duration) => duration !== "0s")).toBe(true);

  await page.emulateMedia({ reducedMotion: "reduce" });
  const reduced = await page.evaluate(() => {
    const durationMs = (property: string) => {
      const value = getComputedStyle(document.documentElement).getPropertyValue(property).trim();
      return value.endsWith("ms") ? Number.parseFloat(value) : Number.parseFloat(value) * 1000;
    };
    return { fast: durationMs("--dur-fast"), regular: durationMs("--dur") };
  });
  const transitionDurations = await card.evaluate((element) =>
    getComputedStyle(element).transitionDuration.split(", "),
  );

  expect(reduced).toEqual({ fast: 0, regular: 0 });
  expect(transitionDurations.every((duration) => duration === "0s")).toBe(true);
}

test.describe("QA-008 composite pixels", () => {
  test("the catalogue opens as one scannable product wall", async ({ page }, testInfo) => {
    await stubPlatform(page);
    const fourth = {
      ...CATALOG.results[1],
      skill_id: "44444444-4444-4444-8444-444444444444",
      name: "Meeting Brief",
      summary: "把會議內容整理成行動摘要",
    };
    await page.route("**/api/skills/catalog**", async (route) => {
      await route.fulfill({
        json: { ...CATALOG, results: [...CATALOG.results, fourth], total: 4 },
      });
    });
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/");

    const cards = page.locator(".catalog-gallery > .catalog-skill-card");
    await expect(cards).toHaveCount(4);
    const desktop = await cards.evaluateAll((elements) =>
      elements.map((element) => {
        const box = element.getBoundingClientRect();
        return { top: box.top, bottom: box.bottom, height: box.height };
      }),
    );
    await page.screenshot({ path: testInfo.outputPath("catalog-desktop.png"), fullPage: true });
    expect(new Set(desktop.map((box) => Math.round(box.top))).size).toBe(1);
    expect(Math.max(...desktop.map((box) => box.height))).toBeLessThanOrEqual(320);
    expect(Math.max(...desktop.map((box) => box.bottom))).toBeLessThanOrEqual(900);
    await page.emulateMedia({ colorScheme: "dark" });
    await page.screenshot({
      path: testInfo.outputPath("catalog-desktop-dark.png"),
      fullPage: true,
    });

    await page.emulateMedia({ colorScheme: "light" });
    await page.setViewportSize({ width: 375, height: 900 });
    const phoneWidth = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(phoneWidth.scroll).toBeLessThanOrEqual(phoneWidth.client);
    await page.screenshot({ path: testInfo.outputPath("catalog-phone.png"), fullPage: true });
    await page.emulateMedia({ colorScheme: "dark" });
    await page.screenshot({ path: testInfo.outputPath("catalog-phone-dark.png"), fullPage: true });
  });

  test("desktop platform chrome stays distinct from the work canvas", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/workspace");
    await expect(page.locator(".app-sidebar .app-nav")).toBeVisible();

    const frame = await page.evaluate(() => {
      const header = document.querySelector<HTMLElement>(".app-header")!;
      const title = document.querySelector<HTMLElement>(".app-title")!;
      const sidebar = document.querySelector<HTMLElement>(".app-sidebar")!;
      const main = document.querySelector<HTMLElement>("main")!;
      return {
        header: getComputedStyle(header).backgroundColor,
        title: getComputedStyle(title).backgroundColor,
        sidebar: getComputedStyle(sidebar).backgroundColor,
        sidebarRight: sidebar.getBoundingClientRect().right,
        mainLeft: main.getBoundingClientRect().left,
        sidebarHeight: sidebar.getBoundingClientRect().height,
        headerHeight: header.getBoundingClientRect().height,
      };
    });

    expect(frame.sidebar).toBe(frame.title);
    expect(frame.sidebar).not.toBe(frame.header);
    expect(frame.sidebarRight).toBeLessThanOrEqual(frame.mainLeft);
    expect(frame.sidebarHeight + frame.headerHeight).toBeGreaterThanOrEqual(900);
  });

  for (const [route, where] of [
    ["/?q=pdf", "search results, both .notice bars"],
    ["/policy", "the retention table"],
  ] as const) {
    test(`color-contrast decides and passes: ${where}`, async ({ page }) => {
      await stubPlatform(page);
      await page.goto(route);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      if (route === "/?q=pdf") {
        await expect(page.locator(".notice")).toHaveCount(2);
      }

      const results = await new AxeBuilder({ page }).withRules(["color-contrast"]).analyze();

      expect(
        results.incomplete.filter((r) => r.id === "color-contrast"),
        "color-contrast came back incomplete — this tier decided nothing",
      ).toEqual([]);
      expect(
        results.passes.some((r) => r.id === "color-contrast"),
        "color-contrast never ran at all",
      ).toBe(true);
      expect(results.violations).toEqual([]);
    });
  }

  test("the focus ring is actually painted", async ({ page }) => {
    await stubPlatform(page);
    await page.goto("/");
    await expect(page.locator(".app-nav a").first()).toBeVisible();

    let outline: { width: string; style: string } | null = null;
    for (let i = 0; i < 6 && outline === null; i++) {
      await page.keyboard.press("Tab");
      outline = await page.evaluate(() => {
        const el = document.activeElement;
        if (!el || el === document.body) return null;
        const s = getComputedStyle(el);
        return { width: s.outlineWidth, style: s.outlineStyle };
      });
    }

    expect(outline, "six presses of Tab focused nothing in the page").not.toBeNull();
    expect(outline!.style).not.toBe("none");
    expect(parseFloat(outline!.width)).toBeGreaterThan(0);
  });

  test("reduced motion removes shared interaction transitions", async ({ page }) => {
    await verifyReducedMotion(page);
  });
});

async function verifyVersionLinksOnPhone(page: Page, testInfo: TestInfo) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/lab/test-cases/${TEST_CASE}`);

  const versionLinks = page.locator('[data-role="evidence"]').getByRole("link", {
    name: "查看這次的版本",
  });
  await expect(versionLinks).toHaveCount(2);
  await expect(versionLinks.first()).toHaveAttribute(
    "href",
    `/skills/${SKILL}/versions/${VERSION}`,
  );
  const compareLinks = page.locator('[data-role="evidence"]').getByRole("link", {
    name: /試跑紀錄開始比較/,
  });
  await expect(compareLinks).toHaveCount(2);
  await expect(compareLinks.first()).toHaveAttribute("href", `/runs/${RUN}/compare`);
  const pageWidth = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);
  await page.screenshot({
    path: testInfo.outputPath("test-case-version-links-phone.png"),
    fullPage: true,
  });
}

async function verifyVersionEvidenceOnPhone(page: Page, testInfo: TestInfo) {
  await stubPlatform(page);
  let requestedVersion = "";
  const creationSession = "44444444-4444-4444-8444-444444444444";
  await page.route("**/me", async (route) => {
    const { body, status } = platformResponse(route.request().url());
    await route.fulfill({
      status,
      json: { ...(body as object), features: { creation_skill: true } },
    });
  });
  await page.route("**/creation-sessions?*", async (route) => {
    await route.fulfill({
      json: [
        {
          id: creationSession,
          revision: 7,
          state: "saved",
          snapshot: { brief: "整理採購文件並產生摘要" },
          created_at: "2026-09-27T09:00:00Z",
          updated_at: "2026-09-28T09:30:00Z",
          expires_at: "2099-09-27T09:00:00Z",
          deadline: "2026-09-27T10:00:00Z",
        },
      ],
    });
  });
  await page.route("**/runs?*", async (route) => {
    requestedVersion = new URL(route.request().url()).searchParams.get("skill_version_id") ?? "";
    await route.fulfill({ json: RUNS });
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/skills/${SKILL}/versions/${VERSION}`);

  const evidence = page.getByRole("heading", { name: "驗證證據" }).locator("..");
  await expect(evidence.locator(".download-item")).toHaveCount(2);
  expect(requestedVersion).toBe(VERSION);

  const creation = page.getByRole("heading", { name: "Studio 歷程" }).locator("..");
  const creationLink = creation.getByRole("link", { name: /整理採購文件並產生摘要/ });
  await expect(creationLink).toHaveAttribute(
    "href",
    `/workspace/creations?session=${creationSession}`,
  );
  await creationLink.focus();
  await expect(creationLink).toBeFocused();
  await expect(creationLink).toBeInViewport();

  const runLink = evidence.getByRole("link", { name: "查看試跑結果" }).first();
  const testCaseLink = evidence.getByRole("link", { name: "開啟這次的測試題" }).first();
  await expect(runLink).toHaveAttribute("href", `/runs/${RUN}`);
  await expect(testCaseLink).toHaveAttribute(
    "href",
    `/lab/test-cases/${TEST_CASE}?version=${VERSION}`,
  );
  await runLink.focus();
  await expect(runLink).toBeFocused();
  await testCaseLink.focus();
  await expect(testCaseLink).toBeFocused();

  const deliverables = page.getByRole("heading", { name: "這一版的交付套件" }).locator("..");
  await expect(deliverables.locator(".download-item")).toHaveCount(2);
  await expect(deliverables).toContainText("可下載");
  await expect(deliverables).toContainText("已過期,不再提供下載");
  const artifactLink = deliverables.getByRole("link", { name: "查看交付紀錄" }).first();
  await expect(artifactLink).toHaveAttribute("href", `/workspace/downloads?artifact=${ARTIFACT}`);
  await artifactLink.focus();
  await expect(artifactLink).toBeFocused();
  await expect(artifactLink).toBeInViewport();

  const pageWidth = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);
  await page.screenshot({
    path: testInfo.outputPath("version-evidence-phone.png"),
    fullPage: true,
  });
}

async function verifyBundleContinuationOnPhone(page: Page, testInfo: TestInfo) {
  const olderVersion = SKILL_VERSIONS.versions[1];
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto(`/skills/${SKILL}/versions/${olderVersion.version_id}`);

  const continuation = page.getByRole("link", { name: "加入 Bundle" });
  await expect(continuation).toHaveAttribute(
    "href",
    `/workspace/downloads?bundleVersion=${olderVersion.version_id}`,
  );
  await continuation.click();

  const member = page.getByRole("combobox", { name: "PDF Summariser" });
  await expect(member).toBeFocused();
  await expect(member).toHaveValue(olderVersion.version_id);
  await expect(page.getByText("從 v1 接續建立 Bundle。")).toBeVisible();

  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
  await page.screenshot({
    path: testInfo.outputPath("version-bundle-continuation-phone.png"),
    fullPage: true,
  });
}

async function verifyArtifactContinuationOnPhone(page: Page) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto(`/workspace/downloads?artifact=${ARTIFACT}`);

  const target = page.locator('[aria-current="location"]');
  await expect(target).toBeFocused();
  await expect(target).toBeInViewport();
  await expect(target).toContainText("pdf-summariser-v2.zip");
  await expect(target).toContainText("續接位置");
}

async function verifyPublicationAudienceOnPhone(page: Page, testInfo: TestInfo) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto(`/skills/${SKILL}/versions/${VERSION}`);

  await expect(page.getByRole("heading", { name: "Catalog 曝光" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "交付對象" })).toBeVisible();
  await expect(page.getByText("任何人都能閱讀", { exact: true })).toBeVisible();
  await expect(page.getByText("目前提供套件", { exact: true })).toBeVisible();
  await expect(page.getByText("已列入 Catalog", { exact: true })).toBeVisible();
  await expect(page.getByText("任何人都能從搜尋與 Catalog 找到這個 Release。")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("version-catalog-exposure-phone.png"),
    fullPage: true,
  });

  const continuation = page.getByRole("link", { name: "在發佈與交付中查看這一筆" });
  await expect(continuation).toHaveAttribute(
    "href",
    `/workspace/downloads?publication=${PUBLISHER}%2F${PUBLICATION}`,
  );
  await continuation.click();

  const target = page.locator('[aria-current="location"]');
  await expect(target).toBeFocused();
  await expect(target).toBeInViewport();
  await expect(target).toContainText(PUBLICATION);
  await expect(target).toContainText("續接位置");
  await expect(target.getByRole("heading", { name: "交付對象" })).toBeVisible();
  await expect(target).toContainText("任何人都能閱讀");
  await expect(target).toContainText("目前提供套件");

  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
}

async function verifyPublishingWorkspaceMapOnPhone(page: Page, testInfo: TestInfo) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto("/workspace/downloads");

  const map = page.getByRole("navigation", { name: "發佈工作區導覽" });
  await expect(map.getByRole("link")).toHaveCount(3);
  await expect(map.getByRole("link", { name: /單一小工具/ })).toHaveAttribute(
    "href",
    "#skill-publications",
  );
  await expect(map.getByRole("link", { name: /Bundle/ })).toHaveAttribute(
    "href",
    "#bundle-workspace",
  );
  await expect(map.getByRole("link", { name: /交付/ })).toHaveAttribute(
    "href",
    "#delivery-history",
  );

  await map.getByRole("link", { name: /Bundle/ }).click();
  await expect(page.locator("#bundle-workspace")).toBeInViewport();
  await page.screenshot({
    path: testInfo.outputPath("publishing-workspace-map-phone.png"),
    fullPage: true,
  });
}

async function verifyCreationDecisionOnPhone(page: Page, testInfo: TestInfo) {
  const sessionID = "44444444-4444-4444-8444-444444444444";
  const session = {
    id: sessionID,
    revision: 7,
    state: "waiting_confirmation",
    snapshot: {
      messages: Array.from({ length: 12 }, (_, index) => ({
        role: index % 2 === 0 ? "user" : "assistant",
        content: `第 ${index + 1} 則創作訊息，用來保留完整會話脈絡。`,
        created_at: "2026-09-29T08:00:00Z",
      })),
      brief: "把每週客服紀錄整理成可追蹤摘要",
      brief_confirmed: false,
      acceptance_criteria: ["每個問題都有負責人", "列出下一步與期限"],
      diagram_understanding: "",
      diagram_confirmed: false,
      references: [],
      pending_action: "confirm_brief",
      budget_credits: 500,
      reserved_credits: 0,
      spent_credits: 130,
      usage_unknown: false,
      steps: 3,
      tool_calls: 0,
    },
    created_at: "2026-09-29T07:00:00Z",
    updated_at: "2026-09-29T08:00:00Z",
    expires_at: "2099-09-30T08:00:00Z",
    deadline: "2099-09-29T09:00:00Z",
  };
  const otherSession = {
    ...session,
    id: "55555555-5555-4555-8555-555555555555",
    revision: 3,
    state: "working",
    snapshot: {
      ...session.snapshot,
      brief: "把內部規範整理成可重用檢查表",
      pending_action: null,
    },
    updated_at: "2026-09-29T07:30:00Z",
  };
  await stubPlatform(page);
  await page.addInitScript(() =>
    Object.defineProperty(window, "EventSource", { value: undefined, configurable: true }),
  );
  await page.route("**/me", async (route) => {
    const { body, status } = platformResponse(route.request().url());
    await route.fulfill({
      status,
      json: { ...(body as object), features: { generate_skill: true, creation_skill: true } },
    });
  });
  await page.route("**/creation-sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/limits")) {
      await route.fulfill({
        json: {
          min_budget_credits: 130,
          max_budget_credits: 6500,
          max_steps: 20,
          max_tool_calls: 10,
          call_timeout_seconds: 120,
          session_timeout_seconds: 3600,
          retention_seconds: 604800,
        },
      });
      return;
    }
    await route.fulfill({ json: path.endsWith(sessionID) ? session : [session, otherSession] });
  });
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto(`/workspace/creations?session=${sessionID}`);

  await expect(page.getByRole("heading", { name: "和 Agent 一起創作小工具" })).toBeVisible();
  await verifyCreationWorklistOnPhone(page, testInfo, sessionID, session.updated_at);

  const workbench = page.locator(".creation-workbench");
  await expect(workbench).toBeInViewport();
  await expect(workbench.getByText("目前待決定")).toBeVisible();
  await expect(workbench.getByText("確認任務與成功條件")).toBeVisible();
  await expect(workbench.locator('[aria-current="step"]')).toHaveCount(1);
  await expect(page.locator("#creation-message")).toBeInViewport();

  const target = page.locator("#creation-brief-decision");
  await workbench.getByRole("link", { name: "前往這一步" }).click();
  await expect(target).toBeInViewport();
  await target.focus();
  await expect(target).toBeFocused();

  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
  await page.screenshot({
    path: testInfo.outputPath("creation-decision-phone.png"),
    fullPage: true,
  });

  const accessibility = await new AxeBuilder({ page }).include(".creation-shell").analyze();
  expect(accessibility.violations).toEqual([]);

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(`/workspace/creations?session=${sessionID}`);
  await verifyCreationWorklistOnDesktop(page);
  await expect(page.locator(".creation-workbench")).toBeInViewport();
  await expect(page.locator(".creation-journey > li")).toHaveCount(4);
  await page.screenshot({
    path: testInfo.outputPath("creation-decision-desktop.png"),
    fullPage: true,
  });
}

async function verifyCreationWorklistOnPhone(
  page: Page,
  testInfo: TestInfo,
  sessionID: string,
  updatedAt: string,
) {
  const sessionToggle = page.getByRole("button", { name: "創作清單 · 2" });
  await sessionToggle.click();
  const sessionRail = page.locator(".creation-sessions");
  const currentSession = sessionRail.locator(`[data-session="${sessionID}"]`);
  await expect(sessionRail).toBeInViewport();
  await expect(page.locator(".creation-current")).toBeHidden();
  await expect(currentSession).toHaveAttribute("aria-current", "page");
  await expect(currentSession.locator("time")).toHaveAttribute("datetime", updatedAt);
  await expect(sessionRail.getByText("正在創作")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("creation-worklist-phone.png"),
    fullPage: true,
  });
  await currentSession.click();
  await expect(sessionRail).toBeHidden();
  await expect(page.locator(".creation-current")).toBeVisible();
  await expect(page.locator("#creation-workspace")).toBeFocused();
  await expect(sessionToggle).toHaveAttribute("aria-expanded", "false");
}

async function verifyCreationWorklistOnDesktop(page: Page) {
  const desktopRail = page.locator(".creation-sessions");
  const desktopCurrent = page.locator(".creation-current");
  await expect(desktopRail).toBeInViewport();
  await expect(desktopCurrent).toBeInViewport();
  const [railBox, currentBox] = await Promise.all([
    desktopRail.boundingBox(),
    desktopCurrent.boundingBox(),
  ]);
  expect(railBox).not.toBeNull();
  expect(currentBox).not.toBeNull();
  expect(railBox!.x + railBox!.width).toBeLessThanOrEqual(currentBox!.x);
}

async function verifyRunWorkbench(page: Page, testInfo: TestInfo) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(`/runs/${RUN}`);

  const decision = page.locator("#run-decision");
  const rail = page.getByRole("complementary", { name: "試跑紀錄操作與區段導覽" });
  await expect(decision.getByRole("heading", { name: "任務判定" })).toBeVisible();
  await expect(rail.getByRole("navigation", { name: "試跑結果導覽" })).toBeVisible();

  const layout = await page.evaluate(() => {
    const box = (selector: string) => {
      const rect = document.querySelector(selector)!.getBoundingClientRect();
      return { top: rect.top, left: rect.left, right: rect.right, width: rect.width };
    };
    return {
      decision: box("#run-decision"),
      trace: box("#run-trace"),
      artifacts: box("#run-artifacts"),
      rail: box(".run-workspace-rail"),
    };
  });

  expect(layout.decision.top).toBeLessThan(layout.trace.top);
  expect(layout.trace.left).toBeLessThan(layout.rail.left);
  expect(layout.trace.right).toBeLessThanOrEqual(layout.rail.left);
  expect(layout.trace.width).toBe(layout.artifacts.width);
  await page.screenshot({
    path: testInfo.outputPath("run-workbench-desktop.png"),
    fullPage: true,
  });

  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator("#run-decision")).toBeVisible();
  const phone = await page.evaluate(() => {
    const top = (selector: string) => document.querySelector(selector)!.getBoundingClientRect().top;
    return {
      decision: top("#run-decision"),
      rail: top(".run-workspace-rail"),
      trace: top("#run-trace"),
      clientWidth: document.documentElement.clientWidth,
      scrollWidth: document.documentElement.scrollWidth,
    };
  });
  expect(phone.decision).toBeLessThan(phone.rail);
  expect(phone.rail).toBeLessThan(phone.trace);
  expect(phone.scrollWidth).toBeLessThanOrEqual(phone.clientWidth);
  await page.screenshot({ path: testInfo.outputPath("run-workbench-phone.png"), fullPage: true });
}

async function verifyModelTimeoutChoicesOnPhone(page: Page) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin/model-budgets");

  const judge = page.locator("li.download-item").first();
  await expect(judge.getByText("目前：90 秒（已調整）")).toBeVisible();
  await expect(judge.getByText("程式預設：130 秒", { exact: false })).toBeVisible();
  await expect(judge.locator("#admin-budget-judge-run-seconds")).not.toBeVisible();

  await judge.locator("#admin-budget-judge-run-set summary").focus();
  await page.keyboard.press("Enter");
  await expect(judge.locator("#admin-budget-judge-run-seconds")).toBeVisible();
  await expect(judge.getByText("程式預設：130 秒", { exact: false })).toBeVisible();
}

async function verifyAdminSkillActionsOnPhone(page: Page) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/skills?q=${SKILL}`);

  const choices = page.locator("main details[id^='admin-skill-'] summary");
  await expect(choices).toHaveCount(3);
  await expect(page.locator("#admin-takedown-consequences")).toBeVisible();
  await expect(page.locator("#admin-redistribution-value")).not.toBeVisible();

  await choices.nth(1).focus();
  await page.keyboard.press("Enter");
  await expect(page.locator("#admin-redistribution-value")).toBeVisible();
  await expect(page.locator("#admin-redistribution-value")).toHaveValue("");
  await page.locator("#admin-redistribution-note").fill("授權狀態已審查");
  await expect(page.getByRole("button", { name: "送出判定" })).toBeDisabled();
  await page.locator("#admin-redistribution-value").selectOption("blocked");
  await expect(page.getByRole("button", { name: "送出判定" })).toBeEnabled();
  await expect(page.locator("#admin-takedown-consequences")).toBeVisible();
}

async function verifyAuditContextOnPhone(page: Page) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin/audit-log");

  const target = page.locator('tbody tr:first-child [data-label="對象"]');
  await expect(target.getByText("工作區：", { exact: false })).toBeVisible();
  await expect(target.getByText("ws-2")).toBeVisible();
  await expect(page.getByRole("button", { name: "重新整理", exact: true })).toBeVisible();
}

async function verifyAdminPrioritiesReachTheirQueues(page: Page) {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });

  for (const [label, target] of [
    ["平台 Agent 提案", "admin-agent-proposals"],
    ["平台 Agent 待辦", "admin-agent-findings"],
  ]) {
    await page.goto("/admin");
    const priorities = page.locator('[aria-label="目前需留意"]');
    await expect(priorities).toContainText("已取得 4/4 項狀態；其中最早取得於");
    await priorities.getByRole("link", { name: new RegExp(label) }).click();
    await expect(page).toHaveURL(new RegExp(`#${target}$`));
    await expect(page.locator(`#${target} h2`)).toBeInViewport();
  }
}

test("admin priority links reach their work queues on a phone", async ({ page }) =>
  verifyAdminPrioritiesReachTheirQueues(page));

test("admin priorities disclose a partial read failure and recover on a phone", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/admin/agents/proposals**", async (route) => {
    reads += 1;
    const { body, status } = platformResponse(route.request().url());
    await route.fulfill({
      status: reads === 1 ? 503 : status,
      json: reads === 1 ? { error: "service unavailable" } : body,
    });
  });

  await page.goto("/admin");
  const priorities = page.locator('[aria-label="目前需留意"]');
  await expect(priorities.getByRole("alert")).toHaveText(
    "1 項狀態無法取得；請重新整理後再判斷是否還有待處理事項。",
  );
  await expect(priorities).toContainText("已取得 3/4 項狀態；其中最早取得於");
  await expect(priorities.getByRole("link", { name: /平台 Agent 提案/ })).toContainText("無法取得");
  const accessibility = await new AxeBuilder({ page })
    .include('[aria-label="目前需留意"]')
    .analyze();
  expect(accessibility.violations).toEqual([]);

  await priorities.getByRole("button", { name: "重新整理狀態" }).click();
  await expect(priorities.getByRole("alert")).toHaveCount(0);
  await expect(priorities).toContainText("已取得 4/4 項狀態；其中最早取得於");
  await expect(priorities.getByRole("link", { name: /平台 Agent 提案/ })).toContainText(
    "1 件待核准",
  );
  expect(reads).toBe(2);
});

test("exposure review moves keyboard focus between the queue and case", async ({ page }) => {
  await stubPlatform(page);
  await page.goto("/admin/exposure");
  const pageHeading = page.getByRole("heading", { level: 1, name: "曝光審核" });
  const queueHeading = page.getByRole("heading", { level: 2, name: "待審清單" });
  await expect(pageHeading).toBeFocused();
  const review = page.getByRole("link", { name: `審核 ${PUBLISHER}/${PUBLICATION}` });
  await review.focus();
  await review.press("Enter");

  const caseHeading = page.getByRole("heading", {
    level: 2,
    name: `審這一筆：${PUBLISHER}/${PUBLICATION}`,
  });
  await expect(caseHeading).toBeFocused();
  await page.getByRole("link", { name: "返回待審清單" }).press("Enter");
  await expect(queueHeading).toBeFocused();
});

test("admin exposure queue can recover from a failed refresh on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/admin/exposure-reviews", async (route) => {
    reads += 1;
    const { body, status } = platformResponse(route.request().url());
    await route.fulfill({
      status: reads === 2 ? 503 : status,
      json:
        reads === 2 ? { error: "service unavailable" } : reads === 3 ? { publications: [] } : body,
    });
  });

  await page.goto("/admin/exposure");
  await expect(page.getByText("待審：共 1 筆。")).toBeVisible();
  await expect(page.getByText("這份待審清單上次取得於", { exact: false })).toBeVisible();
  const review = page.getByRole("link", { name: `審核 ${PUBLISHER}/${PUBLICATION}` });
  await expect(review).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("admin-exposure-queue-phone.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "暫時無法讀取待審清單" })).toBeVisible();
  await expect(review).toHaveCount(0);
  await page.getByRole("button", { name: "再試一次" }).click();
  await expect(page.getByText("沒有等待審核的發佈物：0 筆。")).toBeVisible();
  expect(reads).toBe(3);
  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
});

test("admin dispatch can retry an unreadable status without enabling release", async ({ page }) => {
  await stubPlatform(page);
  let reads = 0;
  await page.route("**/admin/dispatch", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    reads += 1;
    return route.fulfill(
      reads === 1
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: { dispatching: true, halts: [] } },
    );
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin/dispatch");
  await expect(page.getByRole("alert")).toContainText("暫時無法讀取派送狀態");
  await expect(page.getByRole("button", { name: "停止派送" })).toBeVisible();
  await expect(page.getByRole("button", { name: "恢復派送" })).toHaveCount(0);
  await page.getByRole("button", { name: "重新整理派送狀態" }).click();
  await expect(page.getByText("沒有生效中的煞車。")).toBeVisible();
  expect(reads).toBe(2);
});

test("an Agent owner remains visible without widening the phone page", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin/agents");

  const owner = page.locator("#admin-agent-controls li.download-item").first();
  await expect(owner.getByText("負責營運者：", { exact: false })).toBeVisible();
  await expect(owner.locator("code")).toHaveText("22222222-2222-2222-2222-222222222222");
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
});

test("admin account lookup keeps a grant tied to the submitted email on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route("**/admin/accounts?*", async (route) => {
    const requested = new URL(route.request().url()).searchParams.get("email");
    await route.fulfill({
      json:
        requested === "other@example.com"
          ? {
              ...ADMIN_ACCOUNT,
              email: "other@example.com",
              display_name: "封測者乙",
              workspace_id: "ws-3",
            }
          : ADMIN_ACCOUNT,
    });
  });
  await page.route("**/admin/credits/ws-3", (route) =>
    route.fulfill({ json: { ...ADMIN_LEDGER, workspace_id: "ws-3" } }),
  );

  await page.goto("/admin/accounts");
  const email = page.getByLabel("Email");
  await email.fill("member@example.com");
  await page.getByRole("button", { name: "查詢", exact: true }).click();
  await expect(page.getByRole("heading", { name: "封測者甲" })).toBeVisible();
  await page.locator("#admin-grant-amount").fill("50");
  await page.locator("#admin-grant-note").fill("first account");
  await expect(page.getByRole("button", { name: "授予", exact: true })).toBeEnabled();

  await email.fill("other@example.com");
  await expect(page.getByText("Email 已變更；按「查詢」載入新帳號。")).toBeVisible();
  await expect(page.getByRole("heading", { name: "封測者甲" })).toHaveCount(0);
  await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("admin-account-new-query-phone.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "查詢", exact: true }).click();
  await expect(page.getByRole("heading", { name: "封測者乙" })).toBeFocused();
  await expect(page.locator("#admin-grant-amount")).toHaveValue("");
  await expect(page.locator("#admin-grant-note")).toHaveValue("");
  await expect(page.getByRole("button", { name: "授予", exact: true })).toBeDisabled();
  await expect(page.getByText("目前餘額", { exact: false })).toBeInViewport({ ratio: 1 });
  await page.screenshot({
    path: testInfo.outputPath("admin-account-result-phone.png"),
    fullPage: true,
  });
  await page.getByText("帳號識別資料與建立時間").click();
  await expect(page.getByText("ws-3", { exact: true })).toBeVisible();
  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
});

test("a completed credit grant needs a changed draft before another submission", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let grants = 0;
  await page.route("**/admin/credits/ws-2", (route) =>
    route.fulfill({
      json: {
        ...ADMIN_LEDGER,
        balance_credits: grants ? 170 : 120,
        entries: grants
          ? [
              {
                ...ADMIN_LEDGER.entries[1],
                delta_credits: 50,
                created_at: "2026-10-09T09:00:00Z",
              },
              ...ADMIN_LEDGER.entries,
            ]
          : ADMIN_LEDGER.entries,
      },
    }),
  );
  await page.route("**/admin/credits/ws-2/grants", (route) => {
    grants += 1;
    return route.fulfill({ json: { balance_credits: 170, amount_credits: 50 } });
  });
  await page.goto("/admin/accounts");
  await page.getByLabel("Email").fill("member@example.com");
  await page.getByRole("button", { name: "查詢", exact: true }).click();
  await expect(page.getByRole("heading", { name: "封測者甲" })).toBeVisible();
  await page.locator("#admin-grant-amount").fill("50");
  await page.locator("#admin-grant-note").fill("beta reward");
  const grant = page.getByRole("button", { name: "授予", exact: true });
  await grant.click();
  await expect(page.getByRole("status")).toContainText("已授予 50 點，餘額現在是 170 點。");
  await expect(grant).toBeDisabled();
  await expect(page.locator("#admin-grant-why")).toContainText("再送出，會建立另一筆操作");
  expect(grants).toBe(1);
  await page.screenshot({
    path: testInfo.outputPath("admin-grant-completed-phone.png"),
    fullPage: true,
  });
  await page.locator("#admin-grant-note").fill("second grant");
  await expect(grant).toBeEnabled();
});

test("admin governance keeps actions tied to the submitted search on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route("**/admin/skills?q=other", (route) =>
    route.fulfill({
      json: {
        skills: [
          {
            ...ADMIN_SKILLS.skills[0],
            skill_id: SKILL_B,
            name: "Other Tool",
            workspace_id: "ws-3",
          },
        ],
      },
    }),
  );
  await page.goto(`/admin/skills?q=${SKILL}`);
  await expect(page.getByRole("heading", { name: "對「PDF Summariser」的動作" })).toBeVisible();

  await page.getByLabel("小工具 ID 或名稱").fill("other");
  await expect(page.getByRole("heading", { name: "對「PDF Summariser」的動作" })).toHaveCount(0);
  await expect(page.locator("#admin-skill-takedown")).toHaveCount(0);
  await expect(page.getByText("查詢條件已變更；按「查詢」載入新小工具。")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("admin-governance-new-query-phone.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "查詢", exact: true }).click();
  await expect(page.getByRole("heading", { name: "對「Other Tool」的動作" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "對「PDF Summariser」的動作" })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("下架完成後把焦點交給更新的治理結果", async ({ page }, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let takenDown = false;
  await page.route(`**/admin/skills/${SKILL}/takedown`, (route) => {
    takenDown = true;
    return route.fulfill({ json: { skill_id: SKILL, taken_down: true } });
  });
  await page.route(`**/admin/skills?q=${SKILL}`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return route.fulfill({
      json: {
        skills: ADMIN_SKILLS.skills.map((skill) => ({
          ...skill,
          takedown_at: takenDown ? "2026-10-09T08:00:00Z" : null,
          takedown_reason: takenDown ? "授權問題" : null,
        })),
      },
    });
  });
  await page.goto(`/admin/skills?q=${SKILL}`);

  await page.locator("#admin-skill-takedown summary").click();
  await page.getByLabel("下架理由（必填，會寫進動作紀錄）").fill("授權問題");
  await page.getByRole("button", { name: "下架", exact: true }).click();
  await page.getByRole("button", { name: "確認下架" }).click();

  await expect(page.getByText("已下架", { exact: true })).toBeVisible();
  await expect(page.locator("#admin-takedown-result")).toHaveText("「PDF Summariser」已下架。");
  await expect(page.locator("#admin-takedown-result")).toBeFocused();
  await page.screenshot({ path: testInfo.outputPath("admin-takedown-result-phone.png") });

  await page.reload();
  await expect(page.getByRole("status").filter({ hasText: "已下架" })).toBeVisible();
  await expect(page.getByRole("heading", { level: 1, name: "小工具治理" })).toBeFocused();
});

test("下架成功但重讀失敗時仍保留這次的完成結果", async ({ page }) => {
  await stubPlatform(page);
  let takenDown = false;
  await page.route(`**/admin/skills/${SKILL}/takedown`, (route) => {
    takenDown = true;
    return route.fulfill({ json: { skill_id: SKILL, taken_down: true } });
  });
  await page.route(`**/admin/skills?q=${SKILL}`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return takenDown
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ADMIN_SKILLS });
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/skills?q=${SKILL}`);
  await page.locator("#admin-skill-takedown summary").click();
  await page.getByLabel("下架理由（必填，會寫進動作紀錄）").fill("授權問題");
  await page.getByRole("button", { name: "下架", exact: true }).click();
  await page.getByRole("button", { name: "確認下架" }).click();
  await expect(page.getByRole("alert")).toContainText("暫時無法讀取小工具");
  await expect(page.locator("#admin-takedown-result")).toHaveText("「PDF Summariser」已下架。");
  await expect(page.locator("#admin-takedown-result")).toBeFocused();
  await expect(page.locator("#admin-skill-takedown")).toHaveCount(0);
});

test("受限展示完成後顯示結果，反向操作從空白理由開始", async ({ page }, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let restricted = false;
  await page.route(`**/admin/skills/${SKILL}/restriction`, (route) => {
    restricted = true;
    return route.fulfill({ json: { skill_id: SKILL, access_restriction: "license-review" } });
  });
  await page.route(`**/admin/skills?q=${SKILL}`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return route.fulfill({
      json: {
        skills: ADMIN_SKILLS.skills.map((skill) => ({
          ...skill,
          access_restriction: restricted ? "license-review" : null,
        })),
      },
    });
  });
  await page.goto(`/admin/skills?q=${SKILL}`);
  await page.locator("#admin-skill-restriction summary").click();
  await page.locator("#admin-restriction-note").fill("授權審查");
  await page.getByRole("button", { name: "設定受限", exact: true }).click();

  await expect(page.locator("#admin-governance-result")).toHaveText(
    "「PDF Summariser」已設定受限展示。",
  );
  await expect(page.locator("#admin-governance-result")).toBeFocused();
  await expect(page.locator("#admin-governance-result")).toBeInViewport({ ratio: 1 });
  const resultTop = await page
    .locator("#admin-governance-result")
    .evaluate((element) => element.getBoundingClientRect().top);
  const headerBottom = await page
    .locator(".app-header")
    .evaluate((element) => element.getBoundingClientRect().bottom);
  expect(resultTop).toBeGreaterThanOrEqual(headerBottom);
  await expect(page.getByRole("button", { name: "解除受限", exact: true })).toBeDisabled();
  await expect(page.locator("#admin-restriction-note")).toBeEmpty();
  await page.screenshot({
    path: testInfo.outputPath("admin-restriction-result-phone.png"),
    fullPage: true,
  });
  await expect(page.locator("#admin-governance-result")).toHaveText(
    "「PDF Summariser」已設定受限展示。",
  );

  await page.locator("#admin-restriction-note").fill("授權已確認");
  await expect(page.locator("#admin-governance-result")).toHaveCount(0);
});

test("再散布判定成功但治理狀態重讀失敗時保留完成結果", async ({ page }) => {
  await stubPlatform(page);
  let changed = false;
  await page.route(`**/admin/skills/${SKILL}/redistribution`, (route) => {
    changed = true;
    return route.fulfill({ json: { skill_id: SKILL, redistribution: "blocked" } });
  });
  await page.route(`**/admin/skills?q=${SKILL}`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return changed
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ADMIN_SKILLS });
  });
  await page.goto(`/admin/skills?q=${SKILL}`);
  await page.locator("#admin-skill-redistribution summary").click();
  await page.locator("#admin-redistribution-note").fill("禁止再散布");
  await page.locator("#admin-redistribution-value").selectOption("blocked");
  await page.getByRole("button", { name: "送出判定" }).click();

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取小工具");
  await expect(page.locator("#admin-governance-result")).toHaveText(
    "「PDF Summariser」的再散布判定已改為「禁止再散布」。",
  );
  await expect(page.locator("#admin-governance-result")).toBeFocused();
  await expect(page.locator("#admin-skill-redistribution")).toHaveCount(0);
});

test("admin route focus highlights the heading without outlining the entire content column", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.goto(`/admin/skills?q=${SKILL}`);
  const heading = page.getByRole("heading", { level: 1, name: "小工具治理" });
  await expect(heading).toBeFocused();
  const headingWidth = await heading.evaluate((element) => element.getBoundingClientRect().width);
  const pageWidth = await heading.evaluate(
    (element) => element.parentElement!.getBoundingClientRect().width,
  );
  expect(headingWidth).toBeLessThan(pageWidth / 2);
});

test("admin rosters put beta access first and hide stale membership after a failed refresh", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/admin/rosters", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    reads += 1;
    return route.fulfill(
      reads === 2
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: ADMIN_ROSTERS },
    );
  });
  await page.goto("/admin/rosters");
  const beta = page.getByRole("heading", { name: "封測准入" });
  await expect(beta).toBeInViewport({ ratio: 1 });
  await expect(page.getByText("每一個登入的帳號都算受邀", { exact: false })).toBeVisible();
  await expect(page.getByText("目前 1 位。")).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("admin-rosters-phone.png"), fullPage: true });

  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  await expect(page.getByText("先前載入的名冊已隱藏", { exact: false })).toBeVisible();
  await expect(page.getByText("每一個登入的帳號都算受邀", { exact: false })).toHaveCount(0);
  await expect(page.getByText("目前 1 位。")).toHaveCount(0);
  await page.getByRole("button", { name: "再試一次" }).click();
  await expect(page.getByText("每一個登入的帳號都算受邀", { exact: false })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("admin cost statistics distinguish a micro-dollar charge from zero", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.route("**/admin/cost-statistics", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    return route.fulfill({
      json: {
        statistics: [
          {
            ...ADMIN_COST_STATISTICS.statistics[1],
            p50_usd_micros: 0,
            p90_usd_micros: 1,
            p95_usd_micros: 49,
            max_usd_micros: 50,
          },
        ],
      },
    });
  });
  await page.goto("/admin/cost-statistics");
  const row = page.locator("tbody tr");
  await expect(row.locator('[data-label="p50"]')).toHaveText("$0.0000");
  await expect(row.locator('[data-label="p90"]')).toHaveText("$0.000001");
  await expect(row.locator('[data-label="p95"]')).toHaveText("$0.000049");
  await expect(row.locator('[data-label="最大"]')).toHaveText("$0.00005");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("admin cost statistics hide stale figures after a failed refresh", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let reads = 0;
  await page.route("**/admin/cost-statistics", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    reads += 1;
    return route.fulfill(
      reads === 2
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: ADMIN_COST_STATISTICS },
    );
  });
  await page.goto("/admin/cost-statistics");
  await expect(page.getByRole("table", { name: "每一種呼叫最新的統計窗（美元）" })).toBeVisible();
  await expect(page.getByText("成本統計清單上次取得於", { exact: false })).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("admin-cost-statistics-phone.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "重新整理", exact: true }).click();
  await expect(page.getByText("先前載入的數字已隱藏", { exact: false })).toBeVisible();
  await expect(page.getByRole("table")).toHaveCount(0);
  await page.getByRole("button", { name: "再試一次" }).click();
  await expect(page.getByRole("table", { name: "每一種呼叫最新的統計窗（美元）" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test.describe("QA-008 real layout", () => {
  test("Run result keeps judgment first and turns evidence into a desktop workbench", async ({
    page,
  }, testInfo) => {
    await verifyRunWorkbench(page, testInfo);
  });

  test("test case history keeps each Run's Version and comparison in reach on a phone", async ({
    page,
  }, testInfo) => {
    await verifyVersionLinksOnPhone(page, testInfo);
  });

  test("workspace home keeps creation continuation in the platform priority order", async ({
    page,
  }, testInfo) => {
    await stubCreationContinuations(page);
    await verifyCreationContinuationLayout(page, testInfo);
  });

  test("a version keeps its exact Run evidence readable and reachable on a phone", async ({
    page,
  }, testInfo) => {
    await verifyVersionEvidenceOnPhone(page, testInfo);
  });

  test("a publishing continuation brings the exact artifact into view", async ({ page }) => {
    await verifyArtifactContinuationOnPhone(page);
  });

  test("a version hands its exact Publication to the publishing workspace", async ({
    page,
  }, testInfo) => {
    await verifyPublicationAudienceOnPhone(page, testInfo);
  });

  test("a version keeps its exact Bundle member selection usable on a phone", async ({
    page,
  }, testInfo) => {
    await verifyBundleContinuationOnPhone(page, testInfo);
  });

  test("the publishing workspace map keeps each product lane reachable on a phone", async ({
    page,
  }, testInfo) => {
    await verifyPublishingWorkspaceMapOnPhone(page, testInfo);
  });

  test("Studio keeps the current decision and its evidence reachable on a phone", async ({
    page,
  }, testInfo) => {
    await verifyCreationDecisionOnPhone(page, testInfo);
  });

  test("admin skill actions reveal their forms by keyboard without hiding takedown consequences", async ({
    page,
  }) => verifyAdminSkillActionsOnPhone(page));

  test("model timeout choices keep the effective and default seconds visible on a phone", async ({
    page,
  }) => verifyModelTimeoutChoicesOnPhone(page));

  test("audit rows show their workspace and refresh control on a phone", async ({ page }) =>
    verifyAuditContextOnPhone(page));

  for (const [name, url] of PHONE_ROUTES) {
    test(`the page does not scroll sideways at 375px: ${name}`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);

      const doc = await page.evaluate(() => {
        const limit = document.documentElement.clientWidth;

        function overflowsViewport(el: Element) {
          return (
            el.getBoundingClientRect().right > limit + 0.5 || el.scrollWidth > el.clientWidth + 0.5
          );
        }

        const over = Array.from(document.querySelectorAll("*")).filter(overflowsViewport);

        function isAncestorOfAnother(el: Element) {
          function contains(other: Element) {
            return other !== el && el.contains(other);
          }
          return over.some(contains);
        }

        function isDeepest(el: Element) {
          return !isAncestorOfAnother(el);
        }

        function describeCulprit(el: Element) {
          const box = el.getBoundingClientRect();
          const cls = typeof el.className === "string" ? el.className.trim() : "";
          const at = `${el.tagName.toLowerCase()}${cls ? "." + cls.split(/\s+/).join(".") : ""}`;
          return `${at} w=${Math.round(box.width)} right=${Math.round(box.right)} scrollWidth=${el.scrollWidth}`;
        }

        return {
          scrollWidth: document.documentElement.scrollWidth,
          clientWidth: limit,
          // Deepest only: an ancestor of an overflowing element overflows too,
          // and a list led by html/body would name nothing useful.
          culprits: over.filter(isDeepest).slice(0, 5).map(describeCulprit),
        };
      });
      expect(
        doc.scrollWidth,
        `the page scrolls horizontally: ${doc.scrollWidth}px inside ${doc.clientWidth}px` +
          ` — past the edge: ${doc.culprits.join(" | ") || "(no element found)"}`,
      ).toBeLessThanOrEqual(doc.clientWidth);
    });
  }

  test("a wider native file widget does not push the page sideways: skill-version", async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    await page.waitForSelector('.version-upload input[type="file"]');
    await page.addStyleTag({
      content: '.version-upload input[type="file"] { font-size: 20px }',
    });
    const doc = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
      file: Math.round(
        document.querySelector('.version-upload input[type="file"]')!.getBoundingClientRect().width,
      ),
    }));
    expect(
      doc.scrollWidth,
      `a wider file widget (${doc.file}px) pushed the page to ${doc.scrollWidth}px`,
    ).toBeLessThanOrEqual(doc.clientWidth);
  });
});

test("手機平台框架維持兩列頁首與單列導覽（設計 §4.5）", async ({ page }) => {
  await stubPlatform(page);
  await page.route("**/me", async (route) => {
    const { body, status } = platformResponse(route.request().url());
    await route.fulfill({
      status,
      json: {
        ...(body as object),
        display_name: "名稱很長的封測使用者".repeat(8),
        operator: true,
      },
    });
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/library");
  await expect(page.locator(".app-nav a").first()).toBeVisible();
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        let frames = 0;
        function tick() {
          frames++;
          if (frames < 2) requestAnimationFrame(tick);
          else resolve();
        }
        requestAnimationFrame(tick);
      }),
  );

  const chrome = await page.evaluate(() => {
    const header = document.querySelector(".app-header")!;
    const nav = document.querySelector(".app-sidebar .app-nav")!;
    const box = (node: Element) => {
      const r = node.getBoundingClientRect();
      return { top: r.top, bottom: r.bottom };
    };
    return {
      headerHeight: Math.round(header.getBoundingClientRect().height),
      title: box(header.querySelector(".app-title")!),
      auth: box(header.querySelector("[data-auth-controls]")!),
      navWidth: Math.round(nav.getBoundingClientRect().width),
      viewportWidth: document.documentElement.clientWidth,
      documentWidth: document.documentElement.scrollWidth,
      navHeights: Array.from(nav.querySelectorAll("a"), (link) =>
        Math.round(link.getBoundingClientRect().height),
      ),
      navTops: Array.from(nav.querySelectorAll("a"), (link) =>
        Math.round(link.getBoundingClientRect().top),
      ),
    };
  });

  expect(
    chrome.headerHeight,
    `375px 下頁首高 ${chrome.headerHeight}px：品牌列與搜尋列之外又多了一列`,
  ).toBeLessThanOrEqual(130);
  expect(
    chrome.title.bottom > chrome.auth.top && chrome.auth.bottom > chrome.title.top,
    `標題與身分沒有在同一列上——頁首的第一列又被一個 auto 留白推開了：` +
      `標題 ${Math.round(chrome.title.top)}–${Math.round(chrome.title.bottom)}、` +
      `身分 ${Math.round(chrome.auth.top)}–${Math.round(chrome.auth.bottom)}（頁首高 ${chrome.headerHeight}px）`,
  ).toBe(true);
  expect(
    chrome.navWidth,
    `長帳號名稱把主要導覽壓到只剩 ${chrome.navWidth}px`,
  ).toBeGreaterThanOrEqual(350);
  expect(chrome.documentWidth, "長帳號名稱把頁面撐出視窗").toBeLessThanOrEqual(
    chrome.viewportWidth,
  );
  expect(
    Math.min(...chrome.navHeights),
    `手機導覽的最小點按高度只有 ${Math.min(...chrome.navHeights)}px`,
  ).toBeGreaterThanOrEqual(40);
  expect(new Set(chrome.navTops).size, "主要導覽不再是單列橫向 rail").toBe(1);
});

test("手機主要導覽只在真的溢位時顯示提示", async ({ page }) => {
  test.slow();
  await stubPlatform(page);

  const overflowingWidths: number[] = [];
  const fittingWidths: number[] = [];
  let scrolledToEnd = false;
  for (const width of [320, 340, 360, 375, 383, 391, 400, 503, 640]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/library");
    const nav = page.locator(".app-nav");
    const cue = nav.locator(":scope > .nav-scroll-cue");
    await expect(nav.locator("a").first()).toBeVisible();
    await expect(page.locator("[data-loading]")).toHaveCount(0);
    const overflows = await nav.evaluate((element) => element.scrollWidth > element.clientWidth);
    if (overflows) {
      overflowingWidths.push(width);
      await expect(cue, `${width}px 有溢位卻沒提示`).toBeVisible();
      if (!scrolledToEnd) {
        await nav.evaluate((element) => {
          element.scrollLeft = element.scrollWidth;
          element.dispatchEvent(new Event("scroll"));
        });
        await expect(cue, "捲到最右邊後提示沒有消失").toBeHidden();
        scrolledToEnd = true;
      }
    } else {
      fittingWidths.push(width);
      await expect(cue, `${width}px 沒有溢位卻仍顯示提示`).toBeHidden();
    }
  }
  expect(overflowingWidths, "沒有任何寬度溢位，有溢位的那一支從沒跑到").not.toEqual([]);
  expect(fittingWidths, "沒有任何寬度放得下，沒溢位的那一支從沒跑到").not.toEqual([]);
  expect(scrolledToEnd, "沒有任何寬度溢位，捲到最右邊的檢查從沒跑到").toBe(true);
});

test("後台導覽按工作群組展開，窄螢幕也找得到每個目的地", async ({ page }) => {
  await stubPlatform(page);

  for (const width of [320, 375, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/admin/agents");
    const nav = page.getByRole("navigation", { name: "後台" });
    await expect(nav.locator("summary").first()).toHaveText("治理");
    await expect(nav.locator("summary").last()).toHaveText("營運 · 平台 Agent");
    await expect(nav.getByRole("link", { name: "後台首頁" })).toBeVisible();
    await expect(nav.getByRole("link", { name: "帳號與點數" })).toBeHidden();
    await expect(nav.locator('a[href="/admin/agents"]')).toBeHidden();
    await nav.locator("summary").first().focus();
    await page.keyboard.press("Enter");
    await nav.locator("summary").last().focus();
    await page.keyboard.press("Enter");
    const groupLayout = await nav
      .locator(".admin-nav-links")
      .first()
      .evaluate((element) => getComputedStyle(element).flexWrap);
    expect(groupLayout, `${width}px 後台入口未換行`).toBe("wrap");

    for (const name of [
      "帳號與點數",
      "小工具治理",
      "名冊",
      "曝光審核",
      "派送煞車",
      "動作紀錄",
      "模型呼叫逾時",
      "成本統計",
      "趨勢",
      "平台 Agent",
    ]) {
      await expect(nav.getByRole("link", { name, exact: true })).toBeVisible();
    }
    await expect(nav.getByRole("link", { name: "平台 Agent" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    const offscreenLinks = await nav.locator(".admin-nav-links a").evaluateAll((links) =>
      links
        .filter((link) => {
          const bounds = link.getBoundingClientRect();
          return bounds.left < 0 || bounds.right > document.documentElement.clientWidth;
        })
        .map((link) => link.textContent?.trim()),
    );
    expect(offscreenLinks, `${width}px 有後台入口落在畫面外`).toEqual([]);
    const size = await page.evaluate(() => ({
      pageWidth: document.documentElement.scrollWidth,
      viewportWidth: document.documentElement.clientWidth,
    }));
    expect(size.pageWidth, `${width}px 後台導覽把頁面撐出視窗`).toBeLessThanOrEqual(
      size.viewportWidth,
    );
    await nav.getByRole("link", { name: "帳號與點數" }).click();
    await expect(page.getByRole("heading", { level: 1, name: "帳號與點數" })).toBeVisible();
    await expect(nav.locator("summary").first()).toHaveText("治理 · 帳號與點數");
  }
});

test("後台目前群組在手機首屏保持緊湊且入口可見", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });

  for (const [route, label] of [
    ["/admin/skills", "小工具治理"],
    ["/admin/agents", "平台 Agent"],
  ]) {
    await page.goto(route);
    const nav = page.getByRole("navigation", { name: "後台" });
    await expect(nav.locator("summary").filter({ hasText: label })).toBeVisible();
    const navHeight = await nav.evaluate((element) => element.getBoundingClientRect().height);
    expect(navHeight, `${route} 的後台導覽佔用過多首屏`).toBeLessThan(160);
  }
});

test("從後台首頁下方入口切換頁面後，焦點與畫面都回到新頁標題", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin");

  await page.locator(".admin-home-list").getByRole("link", { name: "平台 Agent" }).click();

  const heading = page.getByRole("heading", { level: 1, name: "平台 Agent" });
  await expect(heading).toBeFocused();
  await expect(heading).toBeInViewport();
});

test("後台首頁的待辦捷徑仍落在指定區塊，不跳回頁面標題", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin");

  await page.getByRole("link", { name: /平台 Agent 待辦/ }).click();

  await expect(page).toHaveURL(/\/admin\/agents#admin-agent-findings$/);
  await expect(page.locator("#admin-agent-findings")).toBeInViewport();
  await expect(page.getByRole("heading", { level: 1, name: "平台 Agent" })).not.toBeFocused();
});

test("未核對的 Agent 日報在手機上不會被呈現成平台已確認的判斷", async ({ page }, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?run=${AGENT_FAILED_RUN}`);

  await expect(
    page.getByRole("status").filter({ hasText: "不是平台確認的維運事實" }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "未核對的原稿 · 需要注意：1 項" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "需要注意：1 項", exact: true })).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("agent-report-unverified-phone.png"),
    fullPage: true,
  });
});

test("舊資產清單網址保留建立錨點並導向 Library", async ({ page }) => {
  await stubPlatform(page);
  await page.goto("/workspace/skills#create");

  await expect(page).toHaveURL(/\/library#create$/);
  await expect(page.getByRole("heading", { level: 1, name: "資產庫" })).toBeVisible();
  await expect(page.locator("#create")).toBeVisible();
});

test("Library cards surface owner verification and the exact validation journey on a phone", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto("/library");

  const card = page.locator(".skill-card").first();
  await expect(card.getByText("工作區驗證：", { exact: true })).toBeVisible();
  await expect(card.getByText("已掃描", { exact: true })).toBeVisible();
  const validation = card.getByRole("link", { name: "測試題與試跑" });
  await expect(validation).toHaveAttribute("href", `/lab/test-cases?skill=${SKILL}`);
  await validation.focus();
  await expect(validation).toBeFocused();
  const width = await card.evaluate((element) => ({
    client: element.clientWidth,
    scroll: element.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
  await page.screenshot({
    path: testInfo.outputPath("library-validation-phone.png"),
    fullPage: true,
  });
});

test("the validation journey keeps the same Skill workbench in reach", async ({ page }) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });

  const urls = [
    `/skills/${SKILL}`,
    `/skills/${SKILL}/files`,
    `/skills/${SKILL}/versions/${VERSION}`,
    `/skills/${SKILL}/package?version=${VERSION}`,
    `/lab/test-cases?skill=${SKILL}&version=${VERSION}`,
    `/lab/test-cases/${TEST_CASE}?version=${VERSION}`,
    `/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}`,
    `/runs/${RUN}`,
    `/runs/${RUN}/compare?against=${OTHER_RUN}`,
  ];

  for (const url of urls) {
    await page.goto(url);
    const nav = page.getByRole("navigation", { name: "這個小工具的工作台" });
    await expect(nav, url).toBeVisible();
    await expect(nav.locator("a, button"), url).toHaveCount(4);
    await expect(nav.getByRole("link", { name: "總覽" }), url).toHaveAttribute(
      "href",
      `/skills/${SKILL}`,
    );
    await expect(nav.getByRole("link", { name: "版本與發佈" }), url).toBeVisible();
  }
});

test.describe("QA-008 real layout: 桌面版頁首與導覽對齊", () => {
  for (const width of [1440, 1280]) {
    test(`頁首橫貫視窗，品牌對齊側欄、搜尋對齊內容：${width}px（設計 §4.5）`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/library");
      await expect(page.locator(".app-title")).toBeVisible();

      const m = await page.evaluate(() => {
        const x = (sel: string) =>
          Math.round(document.querySelector(sel)!.getBoundingClientRect().x);
        return {
          headerWidth: Math.round(
            document.querySelector(".app-header")!.getBoundingClientRect().width,
          ),
          viewport: document.documentElement.clientWidth,
          title: x(".app-title"),
          nav: x(".app-sidebar-label"),
          search: x(".app-search"),
          h1: x("main h1"),
        };
      });

      expect(
        m.headerWidth,
        `頁首只有 ${m.headerWidth}px 而視窗是 ${m.viewport}px：它又縮回欄寬裡了`,
      ).toBe(m.viewport);
      expect(m.title, `品牌左緣 ${m.title} 對不上側欄導覽的 ${m.nav}`).toBe(m.nav);
      expect(m.search, `搜尋左緣 ${m.search} 對不上內容的 ${m.h1}`).toBe(m.h1);
    });
  }

  test("有內容的資產庫把新增方式收成一條入口帶（設計 §4.3）", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/library");
    const hub = page.locator(".create-hub-compact");
    await expect(hub).toBeVisible();
    await expect(hub.locator(".create-cards")).toHaveCount(0);

    const layout = await hub.evaluate((element) => ({
      links: element.querySelectorAll(".create-links a").length,
      overflows: element.scrollWidth > element.clientWidth,
      minimumTarget: Math.min(
        ...Array.from(element.querySelectorAll(".create-links a"), (link) =>
          Math.round(link.getBoundingClientRect().height),
        ),
      ),
    }));

    expect(layout.links, "新增方式沒有完整列出").toBeGreaterThanOrEqual(2);
    expect(layout.overflows, "入口帶把資產庫撐出自己的寬度").toBe(false);
    expect(layout.minimumTarget, "新增入口的最小點按高度不足 40px").toBeGreaterThanOrEqual(40);

    await page.setViewportSize({ width: 375, height: 900 });
    const mobile = await hub.evaluate((element) => {
      const heading = element.querySelector("h2")!.getBoundingClientRect();
      const links = Array.from(element.querySelectorAll(".create-links a"), (link) =>
        link.getBoundingClientRect(),
      );
      return {
        headingBottom: Math.round(heading.bottom),
        firstLinkTop: Math.round(links[0].top),
        linkLefts: links.map((link) => Math.round(link.left)),
        overflows: element.scrollWidth > element.clientWidth,
      };
    });
    expect(mobile.headingBottom, "手機入口帶的標題沒有排在路徑之前").toBeLessThanOrEqual(
      mobile.firstLinkTop,
    );
    expect(new Set(mobile.linkLefts).size, "手機入口沒有沿同一條左緣排列").toBe(1);
    expect(mobile.overflows, "手機入口帶發生橫向溢位").toBe(false);
  });

  test("工作區導覽是一條列，標題與連結同高（設計 §4.3）", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/library");
    await expect(page.locator(".workspace-index")).toBeVisible();

    const rows = await page.evaluate(() => {
      const box = (s: string) => {
        const r = document.querySelector(s)!.getBoundingClientRect();
        return { top: r.top, bottom: r.bottom };
      };
      return { label: box(".workspace-index h2"), links: box(".workspace-index .chip-row") };
    });

    const overlap =
      Math.min(rows.label.bottom, rows.links.bottom) - Math.max(rows.label.top, rows.links.top);
    expect(overlap, "標題被推到連結上面一列去了").toBeGreaterThan(0);

    const links = await page.evaluate(() =>
      [...document.querySelectorAll(".workspace-index a")].map((a) => {
        const cs = getComputedStyle(a);
        return {
          text: a.textContent ?? "",
          height: Math.round(a.getBoundingClientRect().height),
          framed: cs.borderStyle !== "none" || cs.backgroundColor !== "rgba(0, 0, 0, 0)",
        };
      }),
    );

    expect(links.length, "一條連結都沒有量到").toBeGreaterThan(0);
    for (const l of links) {
      expect(l.framed, `「${l.text}」被畫成按鈕了`).toBe(false);
      expect(l.height, `「${l.text}」的命中區只有 ${l.height}px`).toBeGreaterThanOrEqual(40);
    }
  });
});

test("long Agent finding evidence stays within the phone viewport", async ({ page }, testInfo) => {
  const longValue = "x".repeat(500);
  await stubPlatform(page);
  await page.route(`**/admin/agents/findings/${AGENT_FINDING}`, (route) =>
    route.fulfill({
      json: {
        ...ADMIN_AGENT_FINDING,
        events: ADMIN_AGENT_FINDING.events.map((event, index) =>
          index === ADMIN_AGENT_FINDING.events.length - 1
            ? { ...event, evidence: { ...event.evidence, long_value: longValue } }
            : event,
        ),
      },
    }),
  );
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?finding=${AGENT_FINDING}`);

  await expect(page.getByRole("listitem").filter({ hasText: "long_value" })).toContainText(
    longValue,
  );
  const width = await page.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client);
  await page.screenshot({
    path: testInfo.outputPath("admin-finding-long-mobile.png"),
    fullPage: true,
  });
});

test("a finding opens the run that reported its visible evidence", async ({ page }, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?finding=${AGENT_FINDING}`);
  const source = page.getByRole("link", { name: "查看產生這份依據的執行" });
  await expect(source).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("admin-finding-source-mobile.png"),
    fullPage: true,
  });
  await source.click();
  await expect(page).toHaveURL(`http://localhost:4173/admin/agents?run=${AGENT_REPORT_RUN}`);
  await expect(page.getByRole("heading", { name: "這次執行", exact: true })).toBeFocused();
  await expect(page.getByText("需要注意：1 項")).toBeVisible();
});

test("a finding does not show old evidence when its latest report has none", async ({
  page,
}, testInfo) => {
  const latestRun = "11111111-1111-4111-8111-111111111111";
  await stubPlatform(page);
  await page.route(`**/admin/agents/findings/${AGENT_FINDING}`, (route) =>
    route.fulfill({
      json: {
        ...ADMIN_AGENT_FINDING,
        events: [
          ADMIN_AGENT_FINDING.events[0],
          { ...ADMIN_AGENT_FINDING.events[1], run_id: latestRun, evidence: undefined },
        ],
      },
    }),
  );
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?finding=${AGENT_FINDING}`);
  await expect(page.getByText("未測量：目前沒有可核對的引用值。")).toBeVisible();
  await expect(
    page.getByText("/maintenance_jobs/rotate-partitions/overdue_ratio ＝ 2.4"),
  ).toHaveCount(0);
  await expect(page.getByRole("link", { name: "查看最近一次回報的執行" })).toHaveAttribute(
    "href",
    `/admin/agents?run=${latestRun}`,
  );
  await page.screenshot({ path: testInfo.outputPath("admin-finding-missing-evidence-mobile.png") });
});

test("a proposal opens its proposing run before an operator decides", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.route(`**/admin/agents/runs/${ADMIN_AGENT_PROPOSAL.run_id}`, (route) =>
    route.fulfill({ json: { ...ADMIN_AGENT_RUNS.runs[1], id: ADMIN_AGENT_PROPOSAL.run_id } }),
  );
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?proposal=${AGENT_PROPOSAL}`);
  const source = page.getByRole("link", { name: "查看提出這個提案的執行" });
  await expect(source).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("admin-proposal-source-mobile.png"),
    fullPage: true,
  });
  await source.click();
  await expect(page).toHaveURL(
    `http://localhost:4173/admin/agents?run=${ADMIN_AGENT_PROPOSAL.run_id}`,
  );
  await expect(page.getByRole("heading", { name: "這次執行", exact: true })).toBeFocused();
  await expect(page.getByText("需要注意：1 項")).toBeVisible();
});

test("a completed Agent finding move keeps keyboard focus on its result", async ({
  page,
}, testInfo) => {
  let acknowledged = false;
  await stubPlatform(page);
  await page.route(`**/admin/agents/findings/${AGENT_FINDING}/status`, (route) => {
    acknowledged = true;
    return route.fulfill({ status: 204 });
  });
  await page.route(`**/admin/agents/findings/${AGENT_FINDING}`, (route) =>
    route.fulfill({
      json: {
        ...ADMIN_AGENT_FINDING,
        finding: {
          ...ADMIN_AGENT_FINDING.finding,
          status: acknowledged ? "acknowledged" : "open",
        },
      },
    }),
  );
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?finding=${AGENT_FINDING}`);
  await page.locator("#admin-finding-note").fill("checking the job");
  await page.getByRole("button", { name: "我來處理" }).click();
  await expect(page.locator("#admin-finding-result")).toBeFocused();
  await expect(page.getByRole("button", { name: "我來處理" })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("admin-finding-result-mobile.png") });
});

test("an Agent finding move keeps its result when detail refresh fails", async ({
  page,
}, testInfo) => {
  let moved = false;
  await stubPlatform(page);
  await page.route(`**/admin/agents/findings/${AGENT_FINDING}/status`, (route) => {
    moved = true;
    return route.fulfill({ status: 204 });
  });
  await page.route(`**/admin/agents/findings/${AGENT_FINDING}`, (route) =>
    moved
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ADMIN_AGENT_FINDING }),
  );
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?finding=${AGENT_FINDING}`);
  await page.locator("#admin-finding-note").fill("checking the job");
  await page.getByRole("button", { name: "我來處理" }).click();

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取這件事");
  await expect(page.locator("#admin-finding-result")).toContainText("最新狀態尚未確認");
  await expect(page.getByRole("button", { name: "重新整理這件事" })).toBeVisible();
  await expect(page.locator("#admin-finding-result")).toBeFocused();
  await expect(page.locator("#admin-finding-note")).toHaveCount(0);
  const resultTop = await page
    .locator("#admin-finding-result")
    .evaluate((element) => element.getBoundingClientRect().top);
  const headerBottom = await page
    .locator(".app-header")
    .evaluate((element) => element.getBoundingClientRect().bottom);
  expect(resultTop).toBeGreaterThanOrEqual(headerBottom);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("admin-finding-read-failure-phone.png") });
});

test("an Agent brake command stays visible when its status refresh fails", async ({
  page,
}, testInfo) => {
  let engaged = false;
  await stubPlatform(page);
  await page.route("**/admin/agents/brake", (route) => {
    engaged = true;
    return route.fulfill({ status: 200, json: {} });
  });
  await page.route("**/admin/agents", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    return engaged
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ADMIN_AGENTS });
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin/agents");
  await page.locator("#admin-agent-brake-engage-note").fill("incident");
  await page.getByRole("button", { name: "拉下 Agent 煞車" }).click();

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取 Agent 控制");
  await expect(page.locator("#admin-agent-brake-result")).toContainText("最新狀態尚未確認");
  await expect(page.locator("#admin-agent-brake-result")).toBeFocused();
  await expect(page.locator('nav[aria-label="平台 Agent 工作區"]')).toContainText("煞車 待確認");
  await expect(page.getByRole("button", { name: "拉下 Agent 煞車" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重新整理 Agent 狀態" })).toBeVisible();
  const resultTop = await page
    .locator("#admin-agent-brake-result")
    .evaluate((element) => element.getBoundingClientRect().top);
  const headerBottom = await page
    .locator(".app-header")
    .evaluate((element) => element.getBoundingClientRect().bottom);
  expect(resultTop).toBeGreaterThanOrEqual(headerBottom);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("admin-agent-brake-read-failure-phone.png") });
});

test("an Agent switch keeps its result when status refresh fails", async ({ page }, testInfo) => {
  let switched = false;
  await stubPlatform(page);
  await page.route("**/admin/agents/daily-report/enabled", (route) => {
    switched = true;
    return route.fulfill({ status: 200, json: {} });
  });
  await page.route("**/admin/agents", (route) => {
    if (route.request().resourceType() === "document") return route.continue();
    return switched
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ADMIN_AGENTS });
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin/agents");
  await page.locator("#admin-agent-daily-report-note").fill("incident");
  await page.getByRole("button", { name: "停用 daily-report" }).click();

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取 Agent 控制");
  await expect(page.locator("#admin-agent-daily-report-result")).toContainText(
    "已送出停用 daily-report；最新狀態尚未確認。",
  );
  await expect(page.locator("#admin-agent-daily-report-result")).toBeFocused();
  await expect(page.getByRole("button", { name: "停用 daily-report" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重新整理此 Agent" })).toBeVisible();
  const resultTop = await page
    .locator("#admin-agent-daily-report-result")
    .evaluate((element) => element.getBoundingClientRect().top);
  const headerBottom = await page
    .locator(".app-header")
    .evaluate((element) => element.getBoundingClientRect().bottom);
  expect(resultTop).toBeGreaterThanOrEqual(headerBottom);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("admin-agent-switch-read-failure-phone.png") });
});

test("a completed Agent proposal decision keeps keyboard focus on its result", async ({
  page,
}, testInfo) => {
  let rejected = false;
  await stubPlatform(page);
  await page.route(`**/admin/agents/proposals/${AGENT_PROPOSAL}/decision`, (route) => {
    rejected = true;
    return route.fulfill({ status: 204 });
  });
  await page.route(`**/admin/agents/proposals/${AGENT_PROPOSAL}`, (route) =>
    route.fulfill({
      json: { ...ADMIN_AGENT_PROPOSAL, status: rejected ? "rejected" : "proposed" },
    }),
  );
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/agents?proposal=${AGENT_PROPOSAL}`);
  await page.locator("#admin-proposal-reject-note").fill("not needed now");
  await page.getByRole("button", { name: "駁回", exact: true }).click();
  await expect(page.locator("#admin-proposal-result")).toBeFocused();
  await expect(page.getByRole("button", { name: "駁回", exact: true })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("admin-proposal-result-mobile.png") });
});

test("a confirmed proposal decision remains visible when its detail refresh fails", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let decided = false;
  await page.route(`**/admin/agents/proposals/${AGENT_PROPOSAL}/decision`, (route) => {
    decided = true;
    return route.fulfill({ status: 204 });
  });
  await page.route(`**/admin/agents/proposals/${AGENT_PROPOSAL}`, (route) =>
    decided
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ADMIN_AGENT_PROPOSAL }),
  );
  await page.goto(`/admin/agents?proposal=${AGENT_PROPOSAL}`);
  await page.locator("#admin-proposal-reject-note").fill("not needed now");
  await page.getByRole("button", { name: "駁回", exact: true }).click();

  await expect(page.getByRole("alert")).toContainText("暫時無法讀取這個提案");
  await expect(page.locator("#admin-proposal-result")).toContainText("立刻補跑「輪替分割表」");
  await expect(page.locator("#admin-proposal-result")).toContainText("已駁回。");
  await expect(page.locator("#admin-proposal-result")).toBeFocused();
  await expect(page.locator("#admin-proposal-reject-note")).toHaveCount(0);
  const resultTop = await page
    .locator("#admin-proposal-result")
    .evaluate((element) => element.getBoundingClientRect().top);
  const headerBottom = await page
    .locator(".app-header")
    .evaluate((element) => element.getBoundingClientRect().bottom);
  expect(resultTop).toBeGreaterThanOrEqual(headerBottom);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("admin-proposal-read-failure-phone.png") });
});

test("exposure review keeps the decision evidence visible and folds technical identifiers", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/exposure?publication=${PUBLISHER}%2F${PUBLICATION}`);

  await expect(page.getByText("目前未曝光：搜尋與目錄看不到它。")).toBeVisible();
  await expect(page.getByText("把 PDF 整理成重點摘要，附上引用頁碼。")).toBeVisible();
  await expect(page.locator('[data-role="evidence"] code')).toHaveText("sha256:aa");
  const identifiers = page.locator("details").filter({ hasText: "審核序號：2" });
  await expect(identifiers).toHaveCount(1);
  await expect(identifiers.locator("summary")).toHaveText("版本識別與審核序號");
  await expect(identifiers.getByText(/審核序號：2/)).toBeHidden();
  await identifiers.locator("summary").click();
  await expect(identifiers.getByText(/審核序號：2/)).toBeVisible();
  await expect(identifiers.locator("code")).toHaveText("sha256:aa");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("an exposure decision keeps its result after the server advances the review sequence", async ({
  page,
}, testInfo) => {
  const updated = { ...ADMIN_EXPOSURE_CASE, sequence: 3, exposed: true };
  let current: typeof updated = ADMIN_EXPOSURE_CASE;
  await stubPlatform(page);
  await page.route(`**/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`, (route) => {
    if (route.request().method() === "POST") current = updated;
    return route.fulfill({ json: current });
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/exposure?publication=${PUBLISHER}%2F${PUBLICATION}`);
  await page.getByRole("radio", { name: "核准" }).check();
  await page.locator("#admin-exposure-review-note").fill("看過了，符合規範");
  await page.getByRole("button", { name: "送出核准" }).click();
  await expect(page.getByText("目前曝光中：搜尋與目錄看得到它。")).toBeVisible();
  await expect(page.locator("#admin-exposure-result")).toHaveText("這筆曝光審核已核准。");
  await expect(page.locator("#admin-exposure-result")).toBeFocused();
  await page.screenshot({ path: testInfo.outputPath("exposure-review-result-mobile.png") });
});

test("an exposure decision does not show the old exposure state while rereading", async ({
  page,
}, testInfo) => {
  const updated = {
    ...ADMIN_EXPOSURE_CASE,
    sequence: 3,
    exposed: true,
    history: [
      { ...ADMIN_EXPOSURE_CASE.history[0], sequence: 3, reason: "看過了，符合規範" },
      ...ADMIN_EXPOSURE_CASE.history,
    ],
  };
  let reviewed = false;
  let rereads = 0;
  let finishReread!: () => void;
  const held = new Promise<void>((resolve) => {
    finishReread = resolve;
  });
  await stubPlatform(page);
  await page.route(`**/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`, async (route) => {
    if (route.request().method() === "POST") {
      reviewed = true;
      return route.fulfill({ json: updated });
    }
    if (reviewed) {
      rereads += 1;
      await held;
    }
    return route.fulfill({ json: reviewed ? updated : ADMIN_EXPOSURE_CASE });
  });
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/exposure?publication=${PUBLISHER}%2F${PUBLICATION}`);
  await page.getByRole("radio", { name: "核准" }).check();
  await page.locator("#admin-exposure-review-note").fill("看過了，符合規範");
  await page.getByRole("button", { name: "送出核准" }).click();

  try {
    await expect.poll(() => rereads).toBeGreaterThan(0);
    await expect(page.locator("#admin-exposure-result")).toBeVisible();
    await expect(page.getByText("目前曝光中：搜尋與目錄看得到它。")).toBeVisible();
    await expect(page.getByText("目前未曝光：搜尋與目錄看不到它。")).toHaveCount(0);
    await expect(page.getByText("核准：看過了，符合規範")).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("exposure-review-readback-phone.png"),
      fullPage: true,
    });
  } finally {
    finishReread();
  }
  await expect(page.getByText("目前曝光中：搜尋與目錄看得到它。")).toBeVisible();
});

test("exposure review explains an unavailable approval beside its control on mobile", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.route(`**/admin/publications/${PUBLISHER}/${PUBLICATION}/exposure`, (route) =>
    route.fulfill({
      json: {
        ...ADMIN_EXPOSURE_CASE,
        approval: {
          allowed: false,
          refusal: {
            reason: "redistribution_not_allowed",
            error:
              "可散布判定不是 allowed：要先以既有的可散布判定動詞附授權證據判成 allowed，才能核准曝光",
          },
        },
      },
    }),
  );
  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto(`/admin/exposure?publication=${PUBLISHER}%2F${PUBLICATION}`);

  const approved = page.getByRole("radio", { name: "核准" });
  await expect(approved).toBeDisabled();
  await expect(approved).toHaveAttribute("aria-describedby", "admin-exposure-approval-why");
  await expect(page.locator("#admin-exposure-approval-why")).toBeVisible();
  await expect(page.getByRole("radio", { name: "撤銷" })).toBeEnabled();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath("exposure-approval-block-mobile.png"),
    fullPage: true,
  });
});

test("a model timeout change keeps the same kind's restore action unavailable", async ({
  page,
}) => {
  await stubPlatform(page);
  let releaseSet!: () => void;
  let started = 0;
  const held = new Promise<void>((resolve) => {
    releaseSet = resolve;
  });
  await page.route("**/admin/model-budgets/judge-run", async (route) => {
    if (route.request().method() !== "PUT") return route.fallback();
    started += 1;
    await held;
    await route.fulfill({ status: 200, json: {} });
  });
  await page.goto("/admin/model-budgets");
  await page.locator("#admin-budget-judge-run-set summary").click();
  await page.locator("#admin-budget-judge-run-clear summary").click();
  await page.locator("#admin-budget-judge-run-seconds").fill("100");
  await page.locator("#admin-budget-judge-run-note").fill("調整等待時間");
  await page.locator("#admin-budget-judge-run-clear-note").fill("恢復預設");
  await page.getByRole("button", { name: "改 評估判定 的秒數" }).click();

  try {
    await expect.poll(() => started).toBe(1);
    await expect(
      page.locator('#admin-budget-judge-run-clear button[type="submit"]'),
    ).toBeDisabled();
    await expect(page.getByText("這一種呼叫正在調整秒數，完成後才能恢復預設。")).toBeVisible();
    await expect(page.locator("#admin-budget-judge-run-seconds")).toHaveAttribute("readonly", "");
  } finally {
    releaseSet();
  }
  await expect(page.locator("#admin-budget-result")).toContainText(
    "重新讀取仍顯示 90 秒；請重新整理確認。",
  );
  await expect(page.locator("#admin-budget-result")).toBeFocused();
});

test("a completed timeout change remains reachable when rereading fails", async ({
  page,
}, testInfo) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let changed = false;
  await page.route("**/admin/model-budgets", (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    return changed
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: ADMIN_MODEL_BUDGETS });
  });
  await page.route("**/admin/model-budgets/judge-run", (route) => {
    changed = true;
    return route.fulfill({ json: {} });
  });
  await page.goto("/admin/model-budgets");
  await page.locator("#admin-budget-judge-run-set summary").click();
  await page.locator("#admin-budget-judge-run-seconds").fill("100");
  await page.locator("#admin-budget-judge-run-note").fill("調整等待時間");
  await page.getByRole("button", { name: "改 評估判定 的秒數" }).click();

  await expect(
    page.getByRole("alert").filter({ hasText: "暫時無法讀取模型呼叫逾時" }),
  ).toBeVisible();
  const result = page.locator("#admin-budget-result");
  await expect(result).toContainText("目前設定暫時無法重新讀取，請稍後核對。");
  await expect(result).toHaveClass(/notice-warning/);
  await expect(result).toBeFocused();
  await expect(page.getByText("目前：90 秒（已調整）")).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const accessibility = await new AxeBuilder({ page }).include("main").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("admin-budget-set-reread-failed-phone.png") });
});

for (const refreshFails of [false, true]) {
  test(`restoring a model timeout keeps its result reachable ${refreshFails ? "when rereading fails" : "after the form disappears"}`, async ({
    page,
  }, testInfo) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let restored = false;
    await page.route("**/admin/model-budgets", (route) => {
      if (route.request().resourceType() === "document") return route.fallback();
      if (restored && refreshFails) {
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      }
      return route.fulfill({
        json: {
          budgets: ADMIN_MODEL_BUDGETS.budgets.map((budget) =>
            restored && budget.kind === "judge-run"
              ? { ...budget, seconds: null, reason: null, set_at: null }
              : budget,
          ),
        },
      });
    });
    await page.route("**/admin/model-budgets/judge-run", (route) => {
      restored = true;
      return route.fulfill({ json: {} });
    });
    await page.goto("/admin/model-budgets");
    await page.locator("#admin-budget-judge-run-clear summary").click();
    await page.locator("#admin-budget-judge-run-clear-note").fill("恢復平台預設");
    await page.getByRole("button", { name: "把 評估判定 改回預設" }).click();

    const result = page.locator("#admin-budget-result");
    if (refreshFails) {
      await expect(
        page.getByRole("alert").filter({ hasText: "暫時無法讀取模型呼叫逾時" }),
      ).toBeVisible();
      await expect(result).toContainText("目前設定暫時無法重新讀取，請稍後核對。");
      await expect(page.getByText("目前：90 秒（已調整）")).toHaveCount(0);
    } else {
      await expect(page.getByText("目前：預設 130 秒")).toBeVisible();
      await expect(result).toContainText("目前顯示程式預設 130 秒；下次呼叫將使用此設定。");
    }
    await expect(page.locator("#admin-budget-judge-run-clear")).toHaveCount(0);
    await expect(result).toBeFocused();
    await page.screenshot({ path: testInfo.outputPath("admin-budget-restored-phone.png") });
  });
}

test.describe("QA-008 real layout: 表格與段落寬度", () => {
  test("a comparison table scrolls inside its own container", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto(`/compare?ids=${SKILL},${SKILL_B}`);
    await expect(page.locator(".compare-table")).toBeVisible();

    const scroller = await page
      .locator(".table-scroll")
      .first()
      .evaluate((el) => ({ scrollWidth: el.scrollWidth, clientWidth: el.clientWidth }));
    expect(scroller.scrollWidth, "nothing to scroll — the table squeezed instead").toBeGreaterThan(
      scroller.clientWidth,
    );

    const stickyRowHeader = await page
      .locator(".table-scroll")
      .first()
      .evaluate((element) => {
        element.scrollLeft = element.scrollWidth;
        const rowHeader = element.querySelector('tbody th[scope="row"]');
        return {
          scrollerLeft: element.getBoundingClientRect().left,
          rowHeaderLeft: rowHeader?.getBoundingClientRect().left ?? Number.NaN,
        };
      });
    expect(
      Math.abs(stickyRowHeader.rowHeaderLeft - stickyRowHeader.scrollerLeft),
      "the row label left the viewport while comparing later products",
    ).toBeLessThanOrEqual(2);
  });

  for (const [name, url] of [
    ["policy events", "/policy"],
    ["admin audit log", "/admin/audit-log"],
    ["admin cost statistics", "/admin/cost-statistics"],
  ] as const) {
    test(`${name} becomes labelled cards instead of a squeezed table`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto(url);

      const table = page.locator("table.responsive-table");
      await expect(table).toBeVisible();
      const layout = await table.evaluate((element) => {
        const row = element.querySelector("tbody tr");
        const rowHeader = row?.querySelector('th[scope="row"]');
        const cells = Array.from(element.querySelectorAll("tbody tr:first-child > *"));
        const scroll = element.closest(".table-scroll");
        return {
          rowDisplay: row ? getComputedStyle(row).display : "missing",
          rowWidth: row?.getBoundingClientRect().width ?? 0,
          rowHeaderWidth: rowHeader?.getBoundingClientRect().width ?? 0,
          labels: cells.map((cell) => cell.getAttribute("data-label")),
          labelContent: cells.map((cell) => getComputedStyle(cell, "::before").content),
          scrollWidth: scroll?.scrollWidth ?? 0,
          clientWidth: scroll?.clientWidth ?? 0,
        };
      });

      expect(layout.rowDisplay).toBe("flex");
      expect(Math.abs(layout.rowHeaderWidth - layout.rowWidth)).toBeLessThanOrEqual(2);
      expect(layout.labels.length).toBeGreaterThan(1);
      expect(layout.labels.every(Boolean)).toBe(true);
      expect(layout.labelContent.every((label) => label !== "none" && label !== '""')).toBe(true);
      expect(layout.scrollWidth).toBeLessThanOrEqual(layout.clientWidth + 1);
    });
  }

  test("no paragraph is wider than the §4.5 measure", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator("h1")).toBeVisible();

    const over = await page.evaluate(() => {
      const bad: string[] = [];
      for (const el of Array.from(document.querySelectorAll("main p, main li, main dd"))) {
        if (el.closest("table")) continue;
        const text = (el.textContent || "").replace(/\s+/g, "");
        if (text.length < 20) continue;
        const em = parseFloat(getComputedStyle(el).fontSize);
        const range = document.createRange();
        range.selectNodeContents(el);
        const w = Math.max(...Array.from(range.getClientRects()).map((r) => r.width), 0);
        if (w > 40 * em + 1) bad.push(`${Math.round(w / em)}em: ${text.slice(0, 20)}`);
      }
      return bad;
    });
    expect(over, `wider than 40em: ${over.join(" / ")}`).toEqual([]);
  });

  test("a primary action link is a control-sized target", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator("a.action").first()).toBeVisible();

    const boxes = await page.evaluate(() =>
      Array.from(document.querySelectorAll("a.action")).map((a) => ({
        text: (a.textContent || "").trim().slice(0, 14),
        height: Math.round(a.getBoundingClientRect().height),
        border: getComputedStyle(a).borderTopWidth,
      })),
    );
    expect(boxes.length, "no primary action on the skill page").toBeGreaterThan(0);
    for (const b of boxes) {
      expect(
        b.height,
        `「${b.text}」 is ${b.height}px tall — a line, not a control`,
      ).toBeGreaterThanOrEqual(32);
      expect(b.border, `「${b.text}」 has no box`).not.toBe("0px");
    }
  });
});

test.describe("QA-008 real layout: 選中狀態與資料完整性", () => {
  for (const [name, url, label] of [
    ["catalog category", "/?category=documents", "文件（"],
    ["trend range", "/admin/trends?days=7", "7 天"],
  ] as const) {
    test(`the current ${name} chip is visibly selected`, async ({ page }) => {
      await stubPlatform(page);
      await page.goto(url);

      const chip = page.locator('.chip[aria-current="page"]', { hasText: label });
      await expect(chip).toHaveCount(1);
      await expect(chip).toBeVisible();
      const selected = await chip.evaluate((el) => {
        const style = getComputedStyle(el);
        const probe = document.createElement("div");
        probe.style.background = "var(--code-bg)";
        document.body.appendChild(probe);
        const selectedBackground = getComputedStyle(probe).backgroundColor;
        probe.remove();
        return {
          background: style.backgroundColor,
          selectedBackground,
          weight: Number(style.fontWeight),
        };
      });
      expect(selected.background).toBe(selected.selectedBackground);
      expect(selected.weight).toBeGreaterThanOrEqual(600);
    });
  }

  test("the current admin page is visibly selected in the collapsed group", async ({ page }) => {
    await stubPlatform(page);
    await page.goto("/admin/accounts");

    const current = page.locator('.admin-nav-group > summary[aria-current="page"]');
    await expect(current).toHaveText("治理 · 帳號與點數");
    await expect(current).toBeVisible();
    const selected = await current.evaluate((element) => {
      const probe = document.createElement("div");
      probe.style.background = "var(--code-bg)";
      document.body.appendChild(probe);
      const background = getComputedStyle(probe).backgroundColor;
      probe.remove();
      return {
        background,
        selectedBackground: getComputedStyle(element).backgroundColor,
        weight: Number(getComputedStyle(element).fontWeight),
      };
    });
    expect(selected.selectedBackground).toBe(selected.background);
    expect(selected.weight).toBeGreaterThanOrEqual(600);
  });

  test("the account visual fixture is a complete success state", async ({ page }) => {
    await stubPlatform(page);
    await page.goto("/workspace/account");

    await expect(page.getByText("目前餘額 120 點")).toBeVisible();
    await expect(page.getByText("還沒有任何點數進出。這裡是空的代表沒有發生過")).toBeVisible();
    await expect(
      page.getByText("帳號、小工具、版本、試跑紀錄、Trace、評估與打包下載會刪除"),
    ).toBeVisible();
    await expect(page.locator('main [role="alert"]')).toHaveCount(0);
    await expect(page.locator("main")).not.toContainText("not found");
  });

  test("a disclaimer beside a badge is not fused to it", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/?q=pdf+%E6%91%98%E8%A6%81");
    await expect(page.locator(".search-result").first()).toBeVisible();

    const fused = await page.evaluate(() => {
      const bad: string[] = [];
      let checked = 0;
      for (const pill of Array.from(document.querySelectorAll(".badge"))) {
        let note: Element | null = pill.nextElementSibling;
        while (note && !note.classList.contains("note")) {
          if (!note.classList.contains("tip")) break;
          note = note.nextElementSibling;
        }
        if (!note?.classList.contains("note")) continue;
        const range = document.createRange();
        range.selectNodeContents(note);
        const first = Array.from(range.getClientRects()).find((r) => r.width > 0);
        const box = pill.getBoundingClientRect();
        if (!first) continue;
        checked++;
        if (Math.min(box.bottom, first.bottom) - Math.max(box.top, first.top) <= 4) continue;
        const gap = first.left - box.right;
        if (gap < 6) bad.push(`${gap.toFixed(1)}px after 「${pill.textContent?.trim()}」`);
      }
      if (checked === 0) bad.push("no badge on this page had a qualifier laid out beside it");
      return bad;
    });
    expect(fused, `fused to the badge: ${fused.join(" / ")}`).toEqual([]);
  });

  test("a fact and its qualifier are not the same colour on a result card", async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto("/?q=pdf+%E6%91%98%E8%A6%81");
    await expect(page.locator(".search-result").first()).toBeVisible();

    const same = await page.evaluate(() => {
      const bad: string[] = [];
      const dds = Array.from(document.querySelectorAll(".search-result .result-facets dd"));
      let checked = 0;
      for (const dd of dds) {
        const note = dd.querySelector(".note");
        if (!note) continue;
        checked++;
        const value = getComputedStyle(dd).color;
        const qualifier = getComputedStyle(note).color;
        if (value === qualifier)
          bad.push(`「${(dd.textContent ?? "").trim().slice(0, 20)}」 both ${value}`);
      }
      if (checked === 0) bad.push("no facet row on this page carried a qualifier");
      return bad;
    });
    expect(same, `fact and qualifier share a colour: ${same.join(" / ")}`).toEqual([]);
  });
});

test.describe("QA-008 real layout: 全站唯一主要動作", () => {
  test("at most one filled primary action per page, and only on a.action/button.action", async ({
    page,
  }) => {
    test.slow();
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });

    const bad: string[] = [];
    let routesWithOne = 0;

    for (const [name, url] of ROUTES) {
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);

      const found = await page.evaluate(() => {
        const probe = document.createElement("div");
        document.body.appendChild(probe);
        const unpainted = getComputedStyle(probe).backgroundColor;
        probe.style.backgroundColor = "var(--cta)";
        const cta = getComputedStyle(probe).backgroundColor;
        probe.remove();

        const filled: { tag: string; cls: string; text: string; action: boolean }[] = [];
        for (const el of Array.from(document.body.querySelectorAll("*"))) {
          if (getComputedStyle(el).backgroundColor !== cta) continue;
          const tag = el.tagName.toLowerCase();
          filled.push({
            tag,
            cls: el.getAttribute("class") ?? "",
            text: (el.textContent ?? "").trim().slice(0, 16),
            action: (tag === "a" || tag === "button") && el.classList.contains("action"),
          });
        }
        return { cta, unpainted, filled };
      });

      if (!found.cta || found.cta === found.unpainted) {
        bad.push(
          `${name}: --cta resolved to 「${found.cta}」 — nothing was measured on this route`,
        );
        continue;
      }

      if (found.filled.length > 1) {
        const which = found.filled.map((f) => `<${f.tag}.${f.cls}>「${f.text}」`).join(" + ");
        bad.push(`${name}: ${found.filled.length} filled actions — ${which}`);
      }
      if (found.filled.length === 1) routesWithOne++;

      for (const f of found.filled) {
        if (f.cls.split(/\s+/).includes("badge")) {
          bad.push(`${name}: a .badge is filled 「${f.text}」 — 填色屬於動作，不屬於主張`);
        } else if (!f.action) {
          bad.push(`${name}: <${f.tag} class="${f.cls}">「${f.text}」 carries the --cta fill`);
        }
      }
    }

    if (routesWithOne === 0) {
      bad.push("no route had a filled primary action at all — this test proved nothing");
    }

    expect(bad, `§4.6.3 一頁一個主要動作: ${bad.join(" / ")}`).toEqual([]);
  });
});

test.describe("QA-008 the real Tab key", () => {
  test("tab order never goes backwards through the document", async ({ page }) => {
    await stubPlatform(page);
    await page.goto("/");
    await expect(page.locator(".app-nav a").first()).toBeVisible();

    await page.evaluate(() => {
      document.querySelectorAll("*").forEach((el, i) => el.setAttribute("data-dom-index", `${i}`));
    });

    const seen: number[] = [];
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press("Tab");
      const at = await page.evaluate(() => {
        const el = document.activeElement;
        if (!el || el === document.body) return null;
        const raw = el.getAttribute("data-dom-index");
        return raw === null ? null : Number(raw);
      });
      if (at === null) continue;
      if (seen.includes(at)) break;
      seen.push(at);
    }

    expect(seen.length, "Tab reached nothing in the page at all").toBeGreaterThan(2);
    const sorted = [...seen].sort((x, y) => x - y);
    expect(seen, `focus jumped backwards: ${seen.join(" → ")}`).toEqual(sorted);
  });

  test("mobile platform chrome focus follows its visual rows", async ({ page, browserName }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto("/library");
    await expect(page.locator(".app-nav a").first()).toBeVisible();

    const title = page.locator(".app-title");
    await title.focus();
    const tops = [Math.round((await title.boundingBox())!.y)];
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press("Tab");
      const top = await page.evaluate(() => {
        const active = document.activeElement;
        if (!(active instanceof HTMLElement) || !active.closest(".app-header, .app-sidebar"))
          return null;
        return active.getBoundingClientRect().top;
      });
      if (top === null) break;
      tops.push(top);
    }

    const domTops = await page
      .locator(".app-header a, .app-header input, .app-header button, .app-sidebar .app-nav a")
      .evaluateAll((elements) => elements.map((element) => element.getBoundingClientRect().top));
    expect(domTops.length, "the platform chrome has too few focus stops").toBeGreaterThan(7);
    expect(domTops, `DOM focus order jumps rows: ${domTops.join(" → ")}`).toEqual(
      [...domTops].sort((a, b) => a - b),
    );
    if (browserName === "webkit") {
      expect(tops.length, "WebKit did not move focus inside the header at all").toBeGreaterThan(1);
      return;
    }
    expect(
      tops.length,
      "the platform chrome exposed too few focus stops to prove row order",
    ).toBeGreaterThan(3);
    expect(tops, `focus jumped to an earlier visual row: ${tops.join(" → ")}`).toEqual(
      [...tops].sort((a, b) => a - b),
    );
  });
});

test.describe("the text budget and the fourth disclosure, in a real engine", () => {
  const TEACHING_FLAT: Record<string, number> = {
    policy: 95,
    "skill-detail": 78,
    "skill-version": 32,
    packaging: 61,
    "run-preflight": 84,
    "lab-run-redirect": 84,
    "dataset-upload": 16,
    "lab-datasets-redirect": 16,
    "lab-test-cases": 37,
    "lab-test-case-detail": 319,
    "run-trace": 51,
    "workspace-account": 42,
    "workspace-downloads": 107,
    "workspace-runs": 18,
    library: 21,
    "workspace-skills-redirect": 21,
  };

  test("flat teaching text: ≤100 runes a block, and never more than the day it was measured", async ({
    page,
  }) => {
    test.slow();
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });

    const bad: string[] = [];
    const measured: Record<string, number> = {};
    for (const [name, url] of ROUTES) {
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);
      const blocks = await page.evaluate(() =>
        Array.from(document.querySelectorAll('[data-role="teaching"]')).map((el) => ({
          flat: !el.closest("[hidden]") && el.getClientRects().length > 0,
          runes: [...(el.textContent ?? "").replace(/\s+/g, "")].length,
          head: (el.textContent ?? "").trim().slice(0, 16),
        })),
      );
      const flat = blocks.filter((b) => b.flat);
      measured[name] = flat.reduce((sum, b) => sum + b.runes, 0);
      for (const b of flat) {
        if (b.runes > 100) {
          bad.push(
            `${name}: a ${b.runes}-rune teaching block 「${b.head}」 — §2.13 單一 D 區塊 ≤100`,
          );
        }
      }
      const cap = TEACHING_FLAT[name] ?? 0;
      if (measured[name] > cap) {
        bad.push(
          `${name}: ${measured[name]} flat teaching runes, over the ${cap} measured on 2026-09-04 — the ratchet only moves down`,
        );
      }
    }
    if (Object.values(measured).every((n) => n === 0)) {
      bad.push("no route has any data-role=teaching — the marks are gone or the scan broke");
    }
    expect(
      bad,
      `§2.13 D 類預算: ${bad.join(" / ")}\nmeasured: ${JSON.stringify(measured)}`,
    ).toEqual([]);
  });

  test("flat teaching and identifiers never outweigh visible evidence, reasons, and caveats", async ({
    page,
  }) => {
    test.slow();
    await stubPlatform(page);
    await page.setViewportSize({ width: 1280, height: 900 });

    const bad: string[] = [];
    for (const [name, url] of ROUTES) {
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();
      await expect(page.locator("[data-loading]")).toHaveCount(0);
      const measured = await page.evaluate(() => {
        const runes = (role: string) => {
          const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
          let total = 0;
          for (let node = walker.nextNode(); node; node = walker.nextNode()) {
            const parent = node.parentElement;
            if (!parent || parent.closest("[hidden]") || parent.getClientRects().length === 0)
              continue;
            if (parent.closest("[data-role]")?.getAttribute("data-role") !== role) continue;
            total += [...(node.textContent ?? "").replace(/\s+/g, "")].length;
          }
          return total;
        };
        return {
          teaching: runes("teaching"),
          evidence: runes("evidence"),
          reason: runes("reason"),
          caveat: runes("caveat"),
        };
      });
      const basis = measured.evidence + measured.reason + measured.caveat;
      if (measured.teaching > basis) {
        bad.push(
          `${name}: ${measured.teaching} flat D/F runes, but only ${basis} A/B/C runes (${JSON.stringify(measured)})`,
        );
      }
    }

    expect(bad, `§2.13 D＋F ≤ A＋B＋C: ${bad.join(" / ")}`).toEqual([]);
  });
});

test.describe("catalog search product visibility", () => {
  for (const viewport of [
    { name: "desktop", width: 1280, height: 900 },
    { name: "phone", width: 375, height: 900 },
  ] as const) {
    test(`search results put the first product in the ${viewport.name} viewport`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.route("**/api/skills/search?*", (route) =>
        route.fulfill({
          json: { ...SEARCH, degraded: false, degraded_reason: undefined, partial_index: false },
        }),
      );
      await page.setViewportSize(viewport);
      await page.goto("/?q=pdf+%E6%91%98%E8%A6%81");

      const firstResultHeader = page.locator(".search-results > li .result-card-head").first();
      await expect(firstResultHeader).toBeVisible();
      const position = await firstResultHeader.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return { top: rect.top, bottom: rect.bottom, viewportHeight: window.innerHeight };
      });
      expect(
        position.top,
        "the first product header starts above the viewport",
      ).toBeGreaterThanOrEqual(0);
      expect(
        position.bottom,
        `the first product header ends at ${position.bottom}px in a ${position.viewportHeight}px viewport`,
      ).toBeLessThanOrEqual(position.viewportHeight);
    });
  }
});

test.describe("the fourth disclosure: a Tip in a real engine", () => {
  test("a Tip opens without moving a neighbour, and Escape closes it", async ({ page }) => {
    await stubPlatform(page);
    await page.route(`**/runs/${RUN}/trace`, (route) => {
      const { body, status } = platformResponse(route.request().url());
      return route.fulfill({ status, json: { ...(body as object), status: "running" } });
    });
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(`/runs/${RUN}`);

    const trigger = page.locator("button.tip-trigger");
    await expect(trigger).toHaveCount(1);
    await expect(trigger).toContainText("為什麼可以關掉這一頁");
    const content = page.locator("p.tip-content");
    await expect(content).toBeHidden();

    const box = await trigger.boundingBox();
    expect(box?.height ?? 0, "the trigger is smaller than the 24px floor").toBeGreaterThanOrEqual(
      24,
    );

    // Document coordinates (+ scrollY), not viewport-relative: opening the
    // trigger scrolls it into view, which would shift every viewport-relative
    // top. Skip elements without a layout box because their top is always zero.
    const positions = () =>
      page.evaluate(() =>
        Array.from(document.querySelectorAll("main *"))
          .filter((el) => !el.closest("[data-tip]") && el.getClientRects().length > 0)
          .map((el) => Math.round(el.getBoundingClientRect().top + window.scrollY)),
      );
    const before = await positions();
    const heightBefore = await page.evaluate(() => document.documentElement.scrollHeight);

    await trigger.click();
    await expect(content).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(await positions(), "opening the Tip moved something else on the page").toEqual(before);
    expect(
      await page.evaluate(() => document.documentElement.scrollHeight),
      "opening the Tip changed the page height",
    ).toBe(heightBefore);

    await page.keyboard.press("Escape");
    await expect(content).toBeHidden();
    await expect(trigger).toBeFocused();
  });
});
