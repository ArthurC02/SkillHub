import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  RUN,
  SKILL,
  SKILL_VERSIONS,
  SUGGESTIONS,
  platformResponse,
} from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const NEW_VERSION = "99999999-1111-4111-8111-111111111111";
const APPLY_BUTTON = "以已接受的 1 項建議建立新版本";
const UNKNOWN_OUTCOME =
  "無法確認是否已建立新版本。請先核對版本清單與建議套用狀態，再決定是否重試。";

const regionOf = (page: App) => page.getByRole("region", { name: "改善建議", exact: true });
const resultAlert = (page: App) =>
  regionOf(page).getByRole("alert", { name: "建立版本結果", exact: true });

async function signedIn(page: App) {
  await page.goto(`/runs/${RUN}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
}

async function verifyVersions(page: App, saved: boolean) {
  await page.getByRole("link", { name: "核對小工具版本", exact: true }).click();
  await expect(page.getByRole("heading", { name: "PDF Summariser", exact: true })).toBeVisible();
  await expect(
    page.getByText(saved ? "共 3 版，最新 v3" : "共 2 版，最新 v2", { exact: false }),
  ).toBeVisible();
}

const APPLY_RECEIPTS: Record<string, unknown> = {
  server500: { error: "internal error" },
  server503: { error: "service unavailable" },
  incomplete: {
    skill_id: SKILL,
    version_id: NEW_VERSION,
    version_number: 3,
    applied_suggestion_ids: ["s1"],
    rejected_suggestions: [],
  },
  null: null,
  valid: {
    skill_id: SKILL,
    version_id: NEW_VERSION,
    version_number: 3,
    content_hash: "hash-new",
    duplicate: false,
    applied_suggestion_ids: ["s1"],
    rejected_suggestions: [],
  },
};

function stubApplyOutcome(page: App, transport: string, onBody: (body: unknown) => void) {
  if (transport === "malformed") {
    const harnessFetch = globalThis.fetch;
    vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
      if (!String(input).endsWith(`/skills/${SKILL}/versions/from-suggestions`))
        return harnessFetch(input, init);
      onBody(JSON.parse(String(init?.body)));
      return new Response("{", { status: 201, headers: { "Content-Type": "application/json" } });
    });
    return;
  }
  page.route(`**/skills/${SKILL}/versions/from-suggestions`, ({ body }) => {
    onBody(body);
    if (transport === "network") return "network-error";
    return {
      status: transport === "server500" ? 500 : transport === "server503" ? 503 : 201,
      json: APPLY_RECEIPTS[transport],
    };
  });
}

test("a complete apply receipt exposes the created version and preserves the original Run for comparison", async () => {
  const page = app();
  const writes: unknown[] = [];
  stubApplyOutcome(page, "valid", (body) => writes.push(body));
  await signedIn(page);
  await expect(page.getByRole("button", { name: APPLY_BUTTON, exact: true })).toHaveCount(1);
  await page.getByRole("button", { name: APPLY_BUTTON, exact: true }).click();
  const region = regionOf(page);
  await expect(region.getByRole("status")).toContainText("已建立新版本 #3");
  expect(
    region.getByRole("link", { name: "開啟剛建立的版本", exact: true }).attribute("href"),
  ).toBe(`/skills/${SKILL}/versions/${NEW_VERSION}`);
  const rerun = region.getByRole("link", { name: "以新版本重跑這個測試題", exact: true });
  const url = new URL(rerun.attribute("href") as string, "http://127.0.0.1:4173");
  expect(url.searchParams.get("version")).toBe(NEW_VERSION);
  expect(url.searchParams.get("against")).toBe(RUN);
  await expect(region.getByRole("alert")).toHaveCount(0);
  await expect(
    region.getByRole("button", { name: "重新確認建議套用狀態", exact: true }),
  ).toHaveCount(0);
  expect(writes).toEqual([{ evaluation_id: "eval-2", suggestion_ids: ["s1"] }]);
  expect(await page.axeViolations("body")).toEqual([]);
});

for (const blockedSession of [false, true]) {
  test(`${blockedSession ? "session-blocked" : "rejected"} apply keeps its known outcome`, async () => {
    const page = app();
    let checking = false;
    let writes = 0;
    page.route("**/me", () =>
      checking
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: platformResponse("/me").body },
    );
    page.route(`**/skills/${SKILL}/versions/from-suggestions`, () => {
      writes += 1;
      return {
        status: 422,
        json: {
          error: "no applicable suggestions",
          rejected_suggestions: [
            { suggestion_id: "s1", blocked_reason: "target_changed", message: "目標檔案變了。" },
          ],
        },
      };
    });
    await signedIn(page);
    const apply = page.getByRole("button", { name: APPLY_BUTTON, exact: true });
    await expect(apply).toHaveCount(1);
    await expect(apply).toBeEnabled();
    if (blockedSession) {
      checking = true;
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
      await expect(
        page.getByRole("button", { name: "重新確認登入狀態", exact: true }),
      ).toBeVisible();
    }
    await apply.click();
    const region = regionOf(page);
    if (blockedSession) {
      await expect(region.getByRole("alert")).toHaveText(
        "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。",
      );
      await expect(region.getByRole("alert")).toBeFocused();
    } else {
      await expect(resultAlert(page)).toContainText("沒有一項建議可以套用，所以沒有建立新版本。");
      await expect(resultAlert(page)).toContainText("目標檔案變了。");
    }
    await expect(
      region.getByRole("button", { name: "重新確認建議套用狀態", exact: true }),
    ).toHaveCount(0);
    await expect(region.getByRole("link", { name: "核對小工具版本", exact: true })).toHaveCount(0);
    expect(writes).toBe(blockedSession ? 0 : 1);
  });
}

for (const [transport, saved, associated, failedRead] of [
  ["network", true, true, false],
  ["network", true, false, false],
  ["server503", false, false, false],
  ["server500", true, true, false],
  ["malformed", true, true, false],
  ["incomplete", true, true, false],
  ["null", true, true, false],
  ["network", true, true, true],
] as const) {
  test(`${transport} apply saved=${saved} association=${associated} read failure=${failedRead} recovers by reading evidence`, async () => {
    const page = app();
    let committed = false;
    let reads = 0;
    const writes: unknown[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    stubApplyOutcome(page, transport, (body) => {
      writes.push(body);
      committed = saved;
    });
    try {
      page.route(`**/runs/${RUN}/suggestions`, async () => {
        reads += 1;
        if (reads === 2 && failedRead)
          return { status: 503, json: { error: "service unavailable" } };
        if (reads === 3) await gate;
        return {
          json: {
            ...SUGGESTIONS,
            suggestions: SUGGESTIONS.suggestions.map((s) => ({
              ...s,
              ...(committed && associated ? { applied_skill_version_id: NEW_VERSION } : {}),
            })),
          },
        };
      });
      page.route(`**/skills/${SKILL}/versions`, () => ({
        json: {
          versions: committed
            ? [
                {
                  version_id: NEW_VERSION,
                  version_number: 3,
                  content_hash: "hash-new",
                  created_at: "2026-10-15T12:00:00Z",
                  held_for_evaluation: false,
                  promoted: false,
                },
                ...SKILL_VERSIONS.versions,
              ]
            : SKILL_VERSIONS.versions,
        },
      }));
      await signedIn(page);
      await expect(page.getByRole("button", { name: APPLY_BUTTON, exact: true })).toHaveCount(1);
      await page.getByRole("button", { name: APPLY_BUTTON, exact: true }).click();
      const region = regionOf(page);
      const alert = resultAlert(page);
      await expect(alert).toContainText(UNKNOWN_OUTCOME);
      await expect(alert).toContainText("尚未回報套用版本，不代表沒有建立版本。");
      await expect(alert).toBeFocused();
      await page.eventually(() => expect(reads).toBe(2));
      expect(writes).toHaveLength(1);
      expect(writes[0]).toEqual({ evaluation_id: "eval-2", suggestion_ids: ["s1"] });
      if (failedRead) await expect(region.getByRole("alert")).toContainText("暫時無法讀取改善建議");
      else if (associated)
        await expect(region.getByText("已建成版本的建議不能撤回", { exact: true })).toBeVisible();
      const retry = region.getByRole("button", { name: "重新確認建議套用狀態", exact: true });
      await expect(alert).toContainText("無法確認是否已建立新版本。");
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() => expect(reads).toBe(3));
      await expect(page.locator("#suggestions-heading")).toBeFocused();
      await expect(retry).toBeDisabled();
      await expect(alert).toContainText("無法確認是否已建立新版本。");
      expect(writes).toHaveLength(1);
      const rebuild = region.getByRole("button", {
        name: `核對後仍以已接受的 ${associated ? 0 : 1} 項建議建立新版本`,
        exact: true,
      });
      if (!failedRead) await expect(rebuild).toBeDisabled();
      release();
      await expect(retry).toBeEnabled();
      if (associated) await expect(rebuild).toBeDisabled();
      else await expect(rebuild).toBeEnabled();
      await expect(region.getByText(/^已建立新版本/)).toHaveCount(0);
      expect(writes).toHaveLength(1);
      expect(await page.axeViolations("body")).toEqual([]);
      await verifyVersions(page, committed);
      expect(writes).toHaveLength(1);
    } finally {
      release();
    }
  });
}
