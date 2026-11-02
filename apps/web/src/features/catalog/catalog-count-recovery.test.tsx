import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { CATALOG } from "../../testing/fixtures/platform";
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

const CASES = [
  { name: "cached zero", initialTotal: 0, recoveredTotal: 3, malformed: false },
  { name: "cached count", initialTotal: 3, recoveredTotal: 0, malformed: false },
  { name: "malformed count", initialTotal: 3, recoveredTotal: 3, malformed: true },
];
const CATEGORY_COUNTS = ["全部（3）", "文件（1）", "寫作（1）", "資料（1）"].join(" ");
const FAILED_COUNTS = [
  "全部（測量失敗）",
  "文件（測量失敗）",
  "寫作（測量失敗）",
  "資料（測量失敗）",
].join(" ");

function stubCounts(page: App, scenario: (typeof CASES)[number]) {
  let phase: "initial" | "failed" | "recovering" = "initial";
  let releaseFailure!: () => void;
  let releaseRecovery!: () => void;
  const failure = new Promise<void>((resolve) => {
    releaseFailure = resolve;
  });
  const recovery = new Promise<void>((resolve) => {
    releaseRecovery = resolve;
  });
  const reads: Record<string, string[]> = { initial: [], failed: [], recovering: [] };
  const primary: string[] = [];
  page.route("**/api/skills/catalog?*", async ({ url, method }) => {
    const category = url.searchParams.get("category");
    const count = url.searchParams.get("limit") === "1";
    const current = phase;
    expect(method).toBe("GET");
    if (count) {
      reads[current].push(category ?? "all");
      if (current === "failed") await failure;
      if (current === "recovering") await recovery;
      if (current === "failed" && !scenario.malformed) return { status: 503, body: "" };
    } else primary.push(url.href);
    const empty = count
      ? (current === "initial" ? scenario.initialTotal : scenario.recoveredTotal) === 0
      : current === "initial" && scenario.initialTotal === 0;
    const rows = empty
      ? []
      : CATALOG.results.filter((row) => !category || row.category.value === category);
    return {
      json: {
        ...CATALOG,
        results: count ? rows.slice(0, 1) : rows,
        limit: count ? 1 : 100,
        total: count && current === "failed" && scenario.malformed ? "0" : rows.length,
        truncated: count && rows.length > 1,
      },
    };
  });
  return {
    reads,
    primary,
    releaseFailure,
    releaseRecovery,
    advance: (next: "failed" | "recovering") => {
      phase = next;
    },
  };
}

for (const scenario of CASES) {
  test(`category counts ${scenario.name} recover in place`, async () => {
    const page = app();
    const { reads, primary, releaseFailure, releaseRecovery, advance } = stubCounts(page, scenario);
    const errors: string[] = [];
    const record = (event: ErrorEvent) => errors.push(event.message);
    window.addEventListener("error", record);
    const location = () => page.pathname() + page.search();
    try {
      await page.goto("/");
      const main = page.locator("main");
      const nav = main.getByRole("navigation", { name: "分類", exact: true });
      await page.eventually(() => expect(reads.initial.length).toBe(4));
      await expect(main.locator(".search-results > li")).toHaveCount(scenario.initialTotal);
      if (scenario.initialTotal === 0) await expect(nav).toHaveCount(0);
      else await expect(nav.getByRole("link")).toHaveText(CATEGORY_COUNTS);
      await page.locator('a[href="/policy"]').first().click();
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toBeVisible();
      advance("failed");
      await page.goBack();
      await page.eventually(() => expect(reads.failed.length).toBe(4));
      const rows = main.locator(".search-results > li");
      await expect(rows).toHaveCount(3);
      const input = main.getByLabel("任務描述", { exact: true });
      await input.fill("保留我的任務草稿");
      const checkbox = rows.last().getByRole("checkbox");
      await checkbox.focus();
      await page.press(" ");
      await expect(checkbox).toBeChecked();
      const selectedURL = location();
      releaseFailure();
      await expect(nav.getByRole("link")).toHaveText(FAILED_COUNTS);
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取分類數量");
      await expect(checkbox).toBeFocused();
      await expect(input).toHaveValue("保留我的任務草稿");
      expect(location()).toBe(selectedURL);
      expect(await page.axeViolations("main")).toEqual([]);
      advance("recovering");
      await page.setOffline(true);
      try {
        await main.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect(
          main.getByText("等待連線以讀取分類數量；恢復連線後會繼續讀取。", { exact: true }),
        ).toBeVisible();
        await expect(main.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
        expect(reads.recovering).toEqual([]);
        await expect(page.locator("#catalog-search-heading")).toBeFocused();
      } finally {
        await page.setOffline(false);
      }
      await page.eventually(() => expect(reads.recovering.length).toBe(4));
      await expect(main.getByRole("button", { name: "重新讀取中…", exact: true })).toBeDisabled();
      await page.press("Enter");
      expect(reads.recovering).toHaveLength(4);
      releaseRecovery();
      await expect(main.getByRole("alert")).toHaveCount(0);
      if (scenario.recoveredTotal === 0) await expect(nav).toHaveCount(0);
      else await expect(nav.getByRole("link")).toHaveText(CATEGORY_COUNTS);
      await expect(checkbox).toBeChecked();
      await expect(input).toHaveValue("保留我的任務草稿");
      await expect(page.locator("#catalog-search-heading")).toBeFocused();
      expect(location()).toBe(selectedURL);
      expect(primary).toHaveLength(2);
      for (const calls of Object.values(reads))
        expect(calls.slice().sort()).toEqual(["all", "data", "documents", "writing"]);
      expect(errors).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      window.removeEventListener("error", record);
      releaseFailure();
      releaseRecovery();
      await page.setOffline(false);
    }
  });
}
