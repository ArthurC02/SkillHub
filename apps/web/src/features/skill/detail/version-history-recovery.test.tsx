import { afterAll, beforeAll, expect, test } from "vitest";
import { SKILL, SKILL_VERSIONS } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});

afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const NEW_VERSION_ID = "99999999-1111-4111-8111-111111111111";
const CURRENT = {
  versions: [
    {
      version_id: NEW_VERSION_ID,
      version_number: 3,
      content_hash: "hash-new",
      created_at: "2026-10-15T12:00:00Z",
    },
    ...SKILL_VERSIONS.versions,
  ],
};

for (const [cached, empty] of [
  [false, false],
  [true, false],
  [true, true],
] as const) {
  test(`version history failure with cached=${cached} empty=${empty} does not claim latest or no access and recovers in place`, async () => {
    const page = app();
    let checking = !cached;
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(`**/skills/${SKILL}/versions`, async () => {
      reads += 1;
      if (!checking) return { json: empty ? { versions: [] } : SKILL_VERSIONS };
      if (reads === (cached ? 2 : 1))
        return { status: 503, json: { error: "service unavailable" } };
      await gate;
      return { json: CURRENT };
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      const history = page.getByRole("region", { name: "版本", exact: true });
      const heading = page.getByRole("heading", { name: "版本", exact: true });
      if (cached) {
        await page.eventually(() => {
          expect(history.getByRole("button", { name: "重新整理", exact: true }).count()).toBe(1);
          expect(history.text()).toContain(empty ? "無權檢視" : "共 2 版，最新 v2");
        });
        checking = true;
        await history.getByRole("button", { name: "重新整理", exact: true }).click();
      }
      await page.eventually(() => {
        expect(history.getByRole("alert").text()).toContain("暫時無法讀取版本歷史");
        expect(history.text()).not.toContain("最新 v2");
        expect(history.text()).not.toContain("無權檢視");
        expect(history.getByRole("group").count()).toBe(0);
      });
      const retry = history.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(reads).toBe(cached ? 3 : 2);
        expect(history.getByRole("status").text()).toContain("載入版本歷史中…");
        expect(heading.isFocused()).toBe(true);
        expect(history.text()).not.toContain("最新 v2");
      });
      release();
      await page.eventually(() => {
        expect(history.text()).toContain("共 3 版，最新 v3");
        expect(history.getByRole("alert").count()).toBe(0);
        expect(heading.isFocused()).toBe(true);
      });
      await history.locator("summary").click();
      await page.eventually(() =>
        expect(history.getByRole("link", { name: "v3", exact: true }).attribute("href")).toBe(
          `/skills/${SKILL}/versions/${NEW_VERSION_ID}`,
        ),
      );
      expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
    }
  });
}

for (const mode of ["pending", "offline"] as const) {
  test(`a cached version history in ${mode} waits for a current snapshot without declaring latest`, async () => {
    const page = app();
    let checking = false;
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(`**/skills/${SKILL}/versions`, async () => {
      reads += 1;
      if (checking) await gate;
      return { json: checking ? CURRENT : SKILL_VERSIONS };
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      const history = page.getByRole("region", { name: "版本", exact: true });
      const heading = page.getByRole("heading", { name: "版本", exact: true });
      await expect(history).toContainText("共 2 版，最新 v2");
      checking = true;
      if (mode === "offline") await page.setOffline(true);
      await history.getByRole("button", { name: "重新整理", exact: true }).click();
      await page.eventually(() => {
        expect(history.getByRole("status").text()).toBe(
          mode === "offline" ? "等待連線以確認版本歷史；目前無法確認最新版本。" : "載入版本歷史中…",
        );
        expect(history.text()).not.toContain("共 2 版，最新 v2");
        expect(history.locator("details").count()).toBe(0);
        expect(history.text()).not.toContain("無權檢視");
        expect(heading.isFocused()).toBe(true);
        expect(reads).toBe(mode === "offline" ? 1 : 2);
      });
      if (mode === "offline") await page.setOffline(false);
      await page.eventually(() => expect(reads).toBe(2));
      release();
      await page.eventually(() => {
        expect(history.text()).toContain("共 3 版，最新 v3");
        expect(history.text()).toContain("上次取得於");
        expect(heading.isFocused()).toBe(true);
      });
      expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
    } finally {
      release();
      await page.setOffline(false);
    }
  });
}
