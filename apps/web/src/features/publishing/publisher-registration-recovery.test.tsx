import { expect, test } from "vitest";
import { OWN_PUBLISHER, SKILL, VERSION } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

const owners = [
  { name: "workspace", url: "/workspace/downloads", region: "發佈者名稱" },
  { name: "version", url: `/skills/${SKILL}/versions/${VERSION}`, region: "發佈" },
];

for (const owner of owners) {
  for (const width of [375, 1280]) {
    for (const outcome of ["refused", "uncertain", "read-error", "registered"] as const) {
      test(`publisher registration ${owner.name} preserves ${outcome} through confirmation at ${width}px`, async () => {
        const page = app();
        let reads = 0;
        let releaseRead!: () => void;
        const reread = new Promise<void>((resolve) => {
          releaseRead = resolve;
        });
        const posts: unknown[] = [];
        const errors: string[] = [];
        const recordError = (event: ErrorEvent) => errors.push(event.message);
        window.addEventListener("error", recordError);
        page.route("**/me/publications", () => ({ json: { publications: [] } }));
        page.route(`**/skills/${SKILL}/publication`, () => ({
          status: 404,
          json: { error: "not published" },
        }));
        page.route("**/me/publisher", async ({ method, body }) => {
          if (method === "POST") {
            posts.push(body);
            return outcome === "refused"
              ? { status: 409, json: { error: "publisher name is taken", reason: "name_taken" } }
              : { status: 503, json: { error: "upstream unavailable" } };
          }
          reads += 1;
          if (reads === 2) {
            await reread;
            if (outcome === "registered") return { json: OWN_PUBLISHER };
            if (outcome === "read-error") return { status: 503, json: { error: "unavailable" } };
          }
          return { status: 404, json: { error: "no publisher" } };
        });
        try {
          await page.goto(owner.url);
          const section = page.getByRole("region", { name: owner.region, exact: true });
          const heading = section.locator("h2").filter({ hasText: owner.region }).first();
          const input = section.locator("input");
          const form = section.locator("form.publisher-form");
          const alert = section.locator("[role=alert]");
          await expect(input).toHaveCount(1);
          await input.fill("kept-namespace");
          await input.press("Enter");
          await page.eventually(() => {
            expect(reads).toBe(2);
            expect(form.count()).toBe(0);
            expect(section.text()).not.toContain("還沒有註冊發佈者名稱");
            expect(heading.isFocused()).toBe(true);
          });
          releaseRead();
          if (outcome === "registered") {
            await page.eventually(() => {
              if (owner.name === "workspace") expect(section.text()).toContain(OWN_PUBLISHER.name);
              else expect(section.locator("label").filter({ hasText: "發佈名稱" }).count()).toBe(1);
              expect(form.count()).toBe(0);
              expect(section.text()).not.toContain("尚未確認註冊");
            });
          } else {
            if (outcome === "read-error") {
              await page.eventually(() => {
                expect(alert.text()).toContain("暫時無法讀取發佈者");
                expect(input.count()).toBe(0);
                expect(section.text()).not.toContain("還沒有註冊發佈者名稱");
              });
              await section.locator("button").filter({ hasText: "再試一次" }).click();
            }
            await page.eventually(() => {
              expect(input.value()).toBe("kept-namespace");
              expect(alert.text()).toBe(
                outcome === "refused"
                  ? "這個名稱已經被別人取走了，換一個名稱再試一次。"
                  : "尚未確認註冊是否已完成；上面的狀態已重新讀取，請先看過再決定要不要重送。",
              );
            });
          }
          await expect(heading).toBeFocused();
          expect(posts).toEqual([{ name: "kept-namespace" }]);
          expect(reads).toBe(outcome === "read-error" ? 3 : 2);
          expect(errors).toEqual([]);
          expect(await page.axeViolations("main")).toEqual([]);
        } finally {
          releaseRead();
          window.removeEventListener("error", recordError);
        }
      });
    }
  }
}
