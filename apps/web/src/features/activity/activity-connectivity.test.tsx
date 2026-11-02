import { expect, test } from "vitest";
import { ACTIVITY, RUN } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

type Mode = "retry" | "populated" | "empty" | "next-page" | "next-page-retry";

function stubActivityReads(page: App, mode: Mode) {
  const pagination = mode.startsWith("next-page");
  const cursors: Array<string | null> = [];
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route("**/me/activity?*", async ({ url }) => {
    cursors.push(url.searchParams.get("cursor"));
    if (cursors.length === 1) {
      if (mode === "retry") return { status: 503, json: { error: "service unavailable" } };
      return {
        json: {
          ...ACTIVITY,
          items: mode === "empty" ? [] : ACTIVITY.items,
          ...(pagination ? { next_cursor: "older-page" } : {}),
        },
      };
    }
    if (mode === "next-page-retry" && cursors.length === 2)
      return { status: 503, json: { error: "service unavailable" } };
    await gate;
    return {
      json: pagination
        ? {
            ...ACTIVITY,
            items: [
              {
                ...ACTIVITY.items[0],
                source_id: RUN,
                summary: "較早的試跑",
                continuation: { kind: "run", run_id: RUN },
              },
            ],
          }
        : ACTIVITY,
    };
  });
  return { cursors, release };
}

for (const mode of ["retry", "populated", "empty", "next-page", "next-page-retry"] as const) {
  test(`activity ${mode} explains an offline wait and resumes once`, async () => {
    const page = app();
    const pagination = mode.startsWith("next-page");
    const retry = mode.endsWith("retry");
    const { cursors, release } = stubActivityReads(page, mode);
    const rows = () => page.locator("main .activity-list > li").count();
    const empty = () => page.getByRole("heading", { name: "目前沒有活動", exact: true });
    const waiting = () =>
      page.getByRole("status").filter({ hasText: "等待連線以讀取Workspace 活動" });
    const alertText = () => page.locator("main [role=alert]").text();
    const writes = () => page.requests.filter((request) => request.method !== "GET");
    try {
      await page.goto("/activity");
      if (mode === "retry")
        await page.eventually(() => expect(page.locator("main [role=alert]").count()).toBe(1));
      else if (mode === "empty") await page.eventually(() => expect(empty().count()).toBe(1));
      else await page.eventually(() => expect(rows()).toBe(3));
      if (mode === "next-page-retry") {
        await page.getByRole("button", { name: "載入更多活動", exact: true }).click();
        await page.eventually(() => expect(alertText()).toContain("更早的活動讀取失敗"));
      }
      await page.setOffline(true);
      const action = page.getByRole("button", {
        name: retry ? "再試一次" : pagination ? "載入更多活動" : "重新整理",
        exact: true,
      });
      await action.focus();
      await page.press("Enter");
      await page.eventually(() => expect(waiting().text()).toContain("恢復連線後會繼續讀取。"));
      expect(navigator.onLine).toBe(false);
      expect(page.locator("main .loading-skeleton").count()).toBe(0);
      if (mode === "next-page-retry") {
        await page.eventually(() => expect(alertText()).toContain("僅保留已載入的紀錄，尚未讀完"));
        expect(page.getByRole("button", { name: "重新讀取中…", exact: true }).isDisabled()).toBe(
          true,
        );
      } else expect(page.locator("main [role=alert]").count()).toBe(0);
      expect(empty().count()).toBe(0);
      expect(rows()).toBe(mode === "retry" || mode === "empty" ? 0 : 3);
      const buttons = page.getByRole("button", { name: "等待連線…", exact: true });
      expect(buttons.count()).toBe(retry ? 0 : pagination ? 2 : 1);
      for (let index = 0; index < buttons.count(); index += 1)
        expect(buttons.nth(index).isDisabled()).toBe(true);
      if (mode === "retry" || mode === "empty")
        expect(page.locator("main .activity-summary").count()).toBe(0);
      const before = mode === "next-page-retry" ? [null, "older-page"] : [null];
      expect(cursors).toEqual(before);
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
      await page.setOffline(false);
      await page.eventually(() => expect(cursors.length).toBe(before.length + 1));
      expect(empty().count()).toBe(0);
      if (mode === "empty")
        await page.eventually(() =>
          expect(page.locator("main [data-loading]").text()).toContain("載入Workspace 活動中…"),
        );
      if (pagination) expect(rows()).toBe(3);
      release();
      await page.eventually(() => expect(rows()).toBe(pagination ? 4 : 3));
      await page.eventually(() => expect(waiting().count()).toBe(0));
      await page.eventually(() =>
        expect(page.getByRole("button", { name: "重新整理", exact: true }).isDisabled()).toBe(
          false,
        ),
      );
      expect(page.getByRole("button", { name: "載入更多活動", exact: true }).count()).toBe(0);
      expect(page.getByText("已載入全部活動", { exact: true }).count()).toBe(1);
      expect(cursors).toEqual([...before, pagination ? "older-page" : null]);
      expect(writes()).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
      await page.setOffline(false);
    }
  });
}
