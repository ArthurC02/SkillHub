import { expect, test } from "vitest";
import { CATALOG, SEARCH, SKILL, SKILL_B } from "../../../testing/fixtures/platform";
import { emptySearchIntent } from "../../../core/api/searchIntent";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

for (const mode of ["catalog", "search", "corrected"] as const) {
  test(`${mode} rejects malformed success and recovers without losing context`, async () => {
    const page = app();
    const errors: string[] = [];
    const recordError = (event: ErrorEvent) => errors.push(event.message);
    window.addEventListener("error", recordError);
    let phase: "initial" | "failed" | "recovering" = "initial";
    let releaseFailure!: () => void;
    let releaseRecovery!: () => void;
    const failure = new Promise<void>((resolve) => {
      releaseFailure = resolve;
    });
    const recovery = new Promise<void>((resolve) => {
      releaseRecovery = resolve;
    });
    const calls: { method: string; url: string; body: unknown }[] = [];
    const valid = mode === "catalog" ? CATALOG : SEARCH;
    page.route(
      mode === "catalog" ? /\/api\/skills\/catalog\?/ : /\/api\/skills\/search(\?|$)/,
      async ({ url, method, body }) => {
        if (mode === "catalog" && url.searchParams.get("limit") === "1") return undefined;
        const current = phase;
        calls.push({ method, url: url.href, body: method === "POST" ? body : null });
        if (current === "failed") await failure;
        if (current === "recovering") await recovery;
        const json =
          current !== "failed"
            ? valid
            : mode === "catalog"
              ? null
              : mode === "search"
                ? { ...SEARCH, results: null }
                : { ...SEARCH, results: SEARCH.results.map((hit) => ({ ...hit, risk: null })) };
        return { status: 200, json };
      },
    );
    const params = new URLSearchParams({ compare: `${SKILL},${SKILL_B}` });
    if (mode !== "catalog") params.set("q", "pdf 摘要");
    if (mode === "corrected")
      params.set(
        "correction",
        JSON.stringify({ intent: { ...emptySearchIntent(), input: "PDF" }, keywords: ["PDF"] }),
      );
    try {
      await page.goto(`/?${params}`);
      const main = page.locator("main");
      const rows = main.locator(".search-results > li");
      const isChecked = (index: number) =>
        (rows.nth(index).getByRole("checkbox").element() as HTMLInputElement).checked;
      await page.eventually(() => {
        expect(rows.count()).toBe(valid.results.length);
        expect(isChecked(0)).toBe(true);
      });
      const originalURL = page.pathname() + page.search();
      const originalRequest = calls[0];
      expect(originalRequest.method).toBe(mode === "corrected" ? "POST" : "GET");
      await page.locator('a[href="/policy"]').first().click();
      await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
      phase = "failed";
      await page.goBack();
      await page.eventually(() => expect(calls).toHaveLength(2));
      const input = page.getByLabel("任務描述", { exact: true });
      await input.fill("保留尚未送出的任務草稿");
      releaseFailure();
      await page.eventually(() => {
        expect(main.getByRole("alert").text()).toContain(
          mode === "catalog" ? "暫時無法讀取目錄" : "暫時無法讀取搜尋結果",
        );
        expect(rows.count()).toBe(0);
        expect(input.value()).toBe("保留尚未送出的任務草稿");
        expect(main.getByText("沒有夠接近的小工具。", { exact: true }).count()).toBe(0);
      });
      expect(page.pathname() + page.search()).toBe(originalURL);
      expect(await page.axeViolations("main")).toEqual([]);
      phase = "recovering";
      await page.setOffline(true);
      try {
        await main.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect(
          main.getByText(
            `等待連線以讀取${mode === "catalog" ? "目錄" : "搜尋結果"}；恢復連線後會繼續讀取。`,
            { exact: true },
          ),
        ).toHaveCount(1);
        expect(calls).toHaveLength(2);
      } finally {
        await page.setOffline(false);
      }
      await page.eventually(() => {
        expect(calls).toHaveLength(3);
        expect(main.getByRole("button", { name: "重新讀取中…", exact: true }).isDisabled()).toBe(
          true,
        );
      });
      releaseRecovery();
      await page.eventually(() => {
        expect(main.getByRole("alert").count()).toBe(0);
        expect(rows.count()).toBe(valid.results.length);
        expect(isChecked(0)).toBe(true);
        expect(isChecked(1)).toBe(true);
        expect(input.value()).toBe("保留尚未送出的任務草稿");
        expect(page.locator("#catalog-search-heading").isFocused()).toBe(true);
      });
      expect(page.pathname() + page.search()).toBe(originalURL);
      expect(calls).toEqual([originalRequest, originalRequest, originalRequest]);
      expect(errors).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      releaseFailure();
      releaseRecovery();
      window.removeEventListener("error", recordError);
    }
  });
}
