import { expect, test } from "vitest";
import { SKILL, VERSION } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

const owners = [
  { name: "workspace", url: "/workspace/downloads", region: "發佈者名稱" },
  { name: "version", url: `/skills/${SKILL}/versions/${VERSION}`, region: "發佈" },
];
const outcomes = ["name_taken", "future_reason", "__proto__", "constructor", "uncertain"];

for (const owner of owners) {
  for (const width of [375, 1280]) {
    for (const outcome of outcomes) {
      test(`publisher feedback ${owner.name} ${outcome} protects the submitted draft at ${width}px`, async () => {
        const page = app();
        let releasePost!: () => void;
        const response = new Promise<void>((resolve) => {
          releasePost = resolve;
        });
        const posts: unknown[] = [];
        const errors: string[] = [];
        let reads = 0;
        const recordError = (event: ErrorEvent) => errors.push(event.message);
        window.addEventListener("error", recordError);
        page.route("**/me/publications", () => ({ json: { publications: [] } }));
        page.route(`**/skills/${SKILL}/publication`, () => ({
          status: 404,
          json: { error: "not published" },
        }));
        page.route("**/me/publisher", async ({ method, body }) => {
          if (method === "GET") {
            reads += 1;
            return { status: 404, json: { error: "no publisher" } };
          }
          posts.push(body);
          await response;
          return outcome === "uncertain"
            ? { status: 503, json: { error: "upstream unavailable" } }
            : { status: 409, json: { error: "publisher registration refused", reason: outcome } };
        });
        try {
          await page.goto(owner.url);
          const section = page.getByRole("region", { name: owner.region, exact: true });
          const input = section.locator("input");
          await expect(input).toHaveCount(1);
          await input.fill("submitted-name");
          await input.press("Enter");
          await page.eventually(() => {
            expect(section.locator("button").filter({ hasText: "註冊中…" }).isDisabled()).toBe(
              true,
            );
            expect(input.isDisabled()).toBe(true);
            expect(posts).toEqual([{ name: "submitted-name" }]);
          });
          releasePost();
          const alert = section.locator("[role=alert]");
          const sentence =
            outcome === "name_taken"
              ? "這個名稱已經被別人取走了，換一個名稱再試一次。"
              : outcome === "uncertain"
                ? "尚未確認註冊是否已完成；上面的狀態已重新讀取，請先看過再決定要不要重送。"
                : "註冊沒有成功，可以再試一次。";
          await page.eventually(() => {
            expect(input.isDisabled()).toBe(false);
            expect(input.value()).toBe("submitted-name");
            expect(alert.text()).toBe(sentence);
          });
          await input.fill("new-name");
          await page.eventually(() => {
            if (outcome === "uncertain") expect(alert.text()).toBe(sentence);
            else expect(alert.count()).toBe(0);
            expect(input.value()).toBe("new-name");
          });
          expect(posts).toEqual([{ name: "submitted-name" }]);
          await page.eventually(() => expect(reads).toBe(2));
          expect(errors).toEqual([]);
          await expect(page.getByText("這一頁暫時無法顯示", { exact: false })).toHaveCount(0);
          expect(await page.axeViolations("main")).toEqual([]);
        } finally {
          releasePost();
          window.removeEventListener("error", recordError);
        }
      });
    }
  }
}
