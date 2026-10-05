import { test, expect, type Page, type TestInfo } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import {
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

async function verifyCreationConversationLayout(page: Page, testInfo: TestInfo) {
  await stubCreationContinuations(page);
  await page.route("**/me/credits", (route) =>
    route.fulfill({
      json: {
        balance_credits: 500,
        debt_floor_credits: -50,
        estimated_session: { low_credits: 30, high_credits: 65, sample_size: 40, estimated: false },
        can_start: true,
      },
    }),
  );
  await page.route("**/creation-sessions/limits", (route) =>
    route.fulfill({
      json: {
        min_budget_credits: 130,
        max_budget_credits: 6500,
        max_steps: 20,
        max_tool_calls: 10,
        call_timeout_seconds: 120,
        session_timeout_seconds: 3600,
        retention_seconds: 604800,
      },
    }),
  );
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/workspace/creations");

  await expect(page.locator(".creation-shell")).toBeVisible();
  await expect(page.locator(".app-search")).toBeHidden();
  await expect(page.locator(".app-sidebar")).toBeHidden();
  await expect(page.locator(".studio-session-rail")).toBeVisible();
  await expect(page.locator(".composer")).toBeInViewport();
  await expect(page.locator(".composer-dock .notice.danger")).toHaveCount(0);
  const desktop = await page.evaluate(() => ({
    railX: document.querySelector(".studio-session-rail")?.getBoundingClientRect().x ?? -1,
    threadHeight: document.querySelector(".creation-stream")?.getBoundingClientRect().height ?? 0,
  }));
  expect(desktop.railX).toBeLessThan(4);
  expect(desktop.threadHeight).toBeGreaterThan(400);
  await page.screenshot({ path: testInfo.outputPath("creation-empty-desktop.png") });

  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto("/workspace/creations");
  await expect(page.locator(".app-search")).toBeHidden();
  await expect(page.locator(".app-sidebar")).toBeHidden();
  await expect(page.locator(".creation-current")).toBeVisible();
  await expect(page.locator(".studio-session-rail")).toBeHidden();
  await expect(page.locator("#creation-message")).toBeInViewport();
  await expect(page.getByRole("button", { name: "開始創作" })).toBeVisible();
  const budget = page.locator(".budget-picker");
  await expect(budget.locator("summary")).toContainText("請選擇");
  await budget.locator("summary").click();
  await budget
    .locator(".quick-replies > label")
    .filter({ hasText: /^500 點$/ })
    .click();
  await expect(budget.locator("summary")).toContainText("500 點");
  await expect(page.locator("#creation-message")).toHaveAttribute(
    "placeholder",
    "描述任務或回覆 Agent",
  );
  await budget.locator("summary").click();
  await page.screenshot({ path: testInfo.outputPath("creation-empty-phone.png") });
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
  await expect(evidence.locator(".surface-card")).toHaveCount(2);
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

async function verifyDraftSwitchOnPhone(
  page: Page,
  testInfo: TestInfo,
  sessionID: string,
  otherSessionID: string,
) {
  const message = page.locator("#creation-message");
  await message.fill("尚未送出的草稿");
  await page.getByRole("button", { name: "創作清單 · 2" }).click();
  await page.locator(`[data-session="${otherSessionID}"]`).click();
  await expect(page.getByRole("alert").filter({ hasText: "未送出的內容" })).toBeVisible();
  await expect(page.getByRole("button", { name: "繼續編輯" })).toBeFocused();
  await expect(page).toHaveURL(new RegExp(`session=${sessionID}$`));
  await page.screenshot({ path: testInfo.outputPath("creation-draft-confirm-phone.png") });
  await page.getByRole("button", { name: "繼續編輯" }).click();
  await expect(message).toHaveValue("尚未送出的草稿");
  await message.fill("");
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

  await verifyDraftSwitchOnPhone(page, testInfo, sessionID, otherSession.id);

  const workbench = page.locator(".creation-workbench");
  await expect(workbench).toBeInViewport();
  await expect(workbench.getByText("目前待決定")).toBeVisible();
  await expect(workbench.getByText("確認任務與成功條件")).toBeVisible();
  await expect(workbench.locator('[aria-current="step"]')).toHaveCount(1);
  await expect(page.locator("#creation-message")).toBeInViewport();
  const phoneHeights = await page.evaluate(() => ({
    progress: document.querySelector(".creation-workbench")?.getBoundingClientRect().height ?? 0,
    conversation: document.querySelector(".creation-stream")?.getBoundingClientRect().height ?? 0,
  }));
  expect(phoneHeights.conversation).toBeGreaterThan(phoneHeights.progress);

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
  const desktopHeights = await page.evaluate(() => ({
    progress: document.querySelector(".creation-workbench")?.getBoundingClientRect().height ?? 0,
    conversation: document.querySelector(".creation-stream")?.getBoundingClientRect().height ?? 0,
  }));
  expect(desktopHeights.conversation).toBeGreaterThan(desktopHeights.progress);
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

  test("Studio opens into the conversation with one thread rail", async ({ page }, testInfo) => {
    await verifyCreationConversationLayout(page, testInfo);
  });

  for (const [name, url] of PHONE_ROUTES) {
    test(`the page does not scroll sideways at 375px: ${name}`, async ({ page }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto(url);
      await expect(page.locator(".app-nav a").first()).toBeVisible();

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

test("手機橫向導覽只在真的溢位時顯示提示", async ({ page }) => {
  test.slow();
  await stubPlatform(page);

  for (const width of [375, 383, 391, 400, 503, 640]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/library");
    const nav = page.locator(".app-nav");
    const cue = nav.locator(":scope > .nav-scroll-cue");
    const overflows = await nav.evaluate((element) => element.scrollWidth > element.clientWidth);
    if (overflows) {
      await expect(cue, `${width}px 有溢位卻沒提示`).toBeVisible();
      if (width === 375) {
        await nav.evaluate((element) => {
          element.scrollLeft = element.scrollWidth;
          element.dispatchEvent(new Event("scroll"));
        });
        await expect(cue, "捲到最右邊後提示沒有消失").toBeHidden();
      }
    } else {
      await expect(cue, `${width}px 沒有溢位卻仍顯示提示`).toBeHidden();
    }
  }

  await page.setViewportSize({ width: 375, height: 900 });
  await page.goto("/admin");
  const adminNav = page.locator(".category-nav");
  const adminCue = adminNav.locator(":scope > .nav-scroll-cue");
  await expect(adminCue).toBeVisible();
  const edges = await adminNav.evaluate((element) => {
    const hint = element.querySelector(".nav-scroll-cue")!;
    return {
      overflows: element.scrollWidth > element.clientWidth,
      hintRight: Math.round(hint.getBoundingClientRect().right),
      navRight: Math.round(element.getBoundingClientRect().right),
    };
  });
  expect(edges.overflows, "後台導覽沒有溢位，提示沒有用途").toBe(true);
  expect(Math.abs(edges.hintRight - edges.navRight), "後台提示沒有黏在右緣").toBeLessThanOrEqual(2);
});

test("舊資產清單網址保留建立錨點並導向 Library", async ({ page }) => {
  await stubPlatform(page);
  await page.goto("/workspace/skills#create");

  await expect(page).toHaveURL(/\/library#create$/);
  await expect(page.getByRole("heading", { level: 1, name: "資產庫" })).toBeVisible();
  await expect(page.locator("#create")).toBeVisible();
});

test("empty Library cards stay still when hovering their non-link surface", async ({ page }) => {
  await stubPlatform(page);
  await page.route(/\/skills\?/, (route) =>
    route.fulfill({ json: { skills: [], total: 0, limit: 24, truncated: false } }),
  );
  await page.goto("/library");

  const card = page.locator(".create-cards > li").first();
  await expect(card).toBeVisible();
  await card.getByRole("heading", { name: "匯入現成的套件" }).hover();
  await expect(card).toHaveCSS("transform", "none");
  await card.getByRole("link", { name: "匯入小工具" }).hover();
  await expect(card).toHaveCSS("transform", "none");
});

test("Library card lift follows the detail link instead of the whole card", async ({ page }) => {
  await stubPlatform(page);
  await page.goto("/library");

  const card = page.locator(".skill-card").first();
  await expect(card).toBeVisible();
  await card.locator(".skill-card-verification").hover();
  await expect(card).toHaveCSS("transform", "none");
  await card.locator(".skill-card-link").hover();
  await expect
    .poll(() =>
      card.evaluate((element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).m42),
    )
    .toBe(-4);
  await card.locator(".skill-card-verification").hover();
  await expect(card).toHaveCSS("transform", "none");
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
    ["admin section", "/admin/accounts", "帳號與點數"],
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
        if (Math.min(box.bottom, first.bottom) - Math.max(box.top, first.top) <= 4) continue;
        const gap = first.left - box.right;
        if (gap < 6) bad.push(`${gap.toFixed(1)}px after 「${pill.textContent?.trim()}」`);
      }
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
