import { expect, test } from "vitest";
import { PUBLIC_BUNDLE_PUBLICATION, PUBLIC_PUBLICATION } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

for (const kind of ["skill", "bundle"] as const) {
  for (const mode of [
    "null",
    "incomplete evidence",
    "availability reread404",
    "availability reread409",
  ] as const) {
    test(`a ${kind} public read with ${mode} withholds unknown content and retries only its read`, async () => {
      const page = app();
      const initial = kind === "skill" ? PUBLIC_PUBLICATION : PUBLIC_BUNDLE_PUBLICATION;
      const incomplete =
        kind === "skill"
          ? {
              ...PUBLIC_PUBLICATION,
              release: { ...PUBLIC_PUBLICATION.release, findings: null },
            }
          : {
              ...PUBLIC_BUNDLE_PUBLICATION,
              bundle_release: { ...PUBLIC_BUNDLE_PUBLICATION.bundle_release, findings: null },
            };
      const address = `/publications/${initial.publisher}/${initial.name}`;
      const rechecking = mode.startsWith("availability reread");
      const malformedRead = rechecking ? 2 : 1;
      let reads = 0;
      page.route(
        (url) => url.pathname === address + "/acquisitions",
        ({ method }) => {
          if (method !== "POST") return undefined;
          return mode.endsWith("404")
            ? { status: 404, json: { error: "not found" } }
            : {
                status: 409,
                json: {
                  error: "作者撤回了這個發佈物，這一頁不再提供它的內容。",
                  reason: "delisted",
                },
              };
        },
      );
      page.route(
        (url) => url.pathname === address,
        ({ method }) => {
          if (method !== "GET") return undefined;
          reads++;
          if (reads === malformedRead)
            return { json: mode === "incomplete evidence" ? incomplete : null };
          return { json: initial };
        },
      );
      const writes = () =>
        page.requests.filter((request) => request.method !== "GET").map((r) => r.url.pathname);
      await page.goto(initial.address);
      const download = page.getByRole("button", { name: "下載", exact: true });
      if (rechecking) {
        await page.eventually(() => expect(download.count()).toBe(1));
        await download.click();
      }
      const main = page.locator("main");
      const alerts = main.locator("[role=alert]");
      await page.eventually(() => {
        expect(main.locator("h1").filter({ hasText: "公開發佈物" }).count()).toBe(1);
        expect(alerts.text()).toContain("暫時無法讀取這個發佈物");
        expect(main.isFocused()).toBe(true);
        expect(page.locator(".publication-trust").count()).toBe(0);
        expect(download.count()).toBe(0);
        expect(main.text()).not.toContain("作者已撤回");
      });
      expect(reads).toBe(malformedRead);
      expect(await page.axeViolations("main")).toEqual([]);
      await main.locator("button").filter({ hasText: "再試一次" }).focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(page.locator(".publication-trust").count()).toBe(1);
        expect(download.count()).toBe(1);
        expect(alerts.filter({ hasText: "暫時無法讀取" }).count()).toBe(0);
        expect(main.isFocused()).toBe(true);
      });
      expect(page.pathname()).toBe(initial.address);
      expect(reads).toBe(malformedRead + 1);
      expect(writes()).toEqual(rechecking ? [address + "/acquisitions"] : []);
      expect(await page.axeViolations("main")).toEqual([]);
      if (rechecking) {
        await download.focus();
        await page.press("Enter");
        await page.eventually(() => {
          const refusal = alerts.filter({
            hasText: mode.endsWith("404") ? "下載沒有成功" : "作者撤回",
          });
          expect(refusal.isFocused()).toBe(true);
          expect(reads).toBe(malformedRead + 2);
        });
        expect(writes()).toEqual([address + "/acquisitions", address + "/acquisitions"]);
      }
    });
  }
}
