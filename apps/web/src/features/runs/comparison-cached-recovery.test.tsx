import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RUN, SKILL } from "../../testing/fixtures/platform";
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

function holdDependency(page: App, pattern: string) {
  const control = { phase: "ready", reads: [] as string[] };
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  const heldStarted = new Promise<void>((resolve) => (started = resolve));
  page.route(pattern, async () => {
    control.reads.push(control.phase);
    if (control.phase === "error") return { status: 503, json: { error: "service unavailable" } };
    if (control.phase === "held") {
      started();
      await held;
    }
    return undefined;
  });
  return { control, release, heldStarted };
}

for (const dependency of ["self", "candidates", "versions"] as const) {
  for (const paused of [false, true]) {
    for (const moved of [false, true]) {
      test(`cached comparison ${dependency} ${paused ? "paused" : "pending"} retry ${moved ? "preserves continued input" : "retains heading focus"}`, async () => {
        const page = app();
        const pattern =
          dependency === "self"
            ? `**/runs/${RUN}`
            : dependency === "candidates"
              ? "**/runs?*"
              : `**/skills/${SKILL}/versions`;
        const what =
          dependency === "self"
            ? "目前這次試跑"
            : dependency === "candidates"
              ? "可比較的試跑紀錄"
              : "候選試跑紀錄的 Version 編號";
        const { control, release, heldStarted } = holdDependency(page, pattern);
        const writes = () => page.requests.filter((request) => request.method !== "GET");
        const comparisons = () =>
          page.requests.filter((request) => request.url.pathname.endsWith("/comparison"));
        const nav = page.getByRole("navigation", { name: "這個小工具的工作台", exact: true });
        const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
        const retry = page.getByRole("button", { name: "再試一次", exact: true });
        const heading = page.locator("#run-comparison-heading");
        const status = page.getByRole("status").filter({ hasText: `載入${what}中…` });
        const compareButton = page.getByRole("button", {
          name: /^與這一次比較（建立於 .+）：v2，/,
        });
        const heldReads = () => control.reads.filter((phase) => phase === "held");
        try {
          await page.goto(`/runs/${RUN}/compare`);
          await expect(page.locator(".auth-name")).toHaveText("tester");
          await expect(compareButton).toBeVisible();
          await nav.getByRole("link", { name: "總覽", exact: true }).click();
          await expect(page.locator("h1")).not.toHaveText("試跑比較");
          await expect(
            page
              .getByRole("region", { name: "版本", exact: true })
              .getByText(/共\s*2\s*版，最新 v2/),
          ).toBeVisible();
          control.phase = "error";
          await page.goBack();
          await expect(page.getByRole("alert")).toContainText(`暫時無法讀取${what}`);
          if (dependency === "self") await expect(nav).toHaveCount(0);
          else await expect(nav).toBeVisible();
          await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
          await field.fill("尚未送出的比較 ID");
          control.phase = "held";
          if (paused) await page.setOffline(true);
          await retry.focus();
          await page.press("Enter");
          await expect(heading).toBeFocused();
          if (paused) {
            await expect(
              page
                .getByRole("status")
                .filter({ hasText: `等待連線以讀取${what}；恢復連線後會繼續讀取。` }),
            ).toBeVisible();
            expect(control.reads).not.toContain("held");
          } else {
            await heldStarted;
            await expect(status).toBeVisible();
          }
          await expect(page.getByRole("alert")).toHaveCount(0);
          await expect(retry).toHaveCount(0);
          await expect(field).toHaveValue("尚未送出的比較 ID");
          if (moved) await field.fill("繼續輸入的比較 ID");
          expect(await page.axeViolations("main")).toEqual([]);
          if (paused) await page.setOffline(false);
          await heldStarted;
          await expect(status).toBeVisible();
          await expect(retry).toHaveCount(0);
          expect(heldReads()).toHaveLength(1);
          release();
          await expect(compareButton).toBeVisible();
          await expect(page.locator("[data-loading]")).toHaveCount(0);
          await expect(page.getByRole("alert")).toHaveCount(0);
          await expect(field).toHaveValue(moved ? "繼續輸入的比較 ID" : "尚未送出的比較 ID");
          if (moved) await expect(field).toBeFocused();
          else await expect(heading).toBeFocused();
          expect(heldReads()).toHaveLength(1);
          expect(writes()).toEqual([]);
          expect(comparisons()).toEqual([]);
          expect(await page.axeViolations("main")).toEqual([]);
        } finally {
          release();
          await page.setOffline(false);
        }
      });
    }
  }
}
