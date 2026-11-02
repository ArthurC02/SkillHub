import { expect, test } from "vitest";
import {
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const valueOf = (selector: string) =>
  document.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector)?.value;

for (const scope of [
  {
    name: "workspace",
    url: "/lab/graders",
    path: "/evaluation/graders",
    grader: WORKSPACE_GRADER,
    heading: "評分器",
  },
  {
    name: "platform",
    url: "/admin/graders",
    path: "/admin/evaluation/graders",
    grader: PLATFORM_GRADER,
    heading: "平台評分器",
  },
]) {
  for (const malformed of ["collection", "instructions"] as const) {
    for (const reread of [false, true]) {
      test(`${scope.name} graders with missing ${malformed} on ${reread ? "reread" : "first read"} recover locally`, async () => {
        const page = app();
        let reads = 0;
        let recovered = false;
        page.route(`**${scope.path}`, () => {
          reads++;
          return {
            json:
              recovered || (reread && reads === 1)
                ? platformResponse(scope.path).body
                : malformed === "collection"
                  ? {}
                  : { graders: [{ ...scope.grader, instructions: null }] },
          };
        });
        const writes = () => page.requests.filter((request) => request.method !== "GET");
        await page.goto(scope.url);
        const instructions = page.locator(`#grader-${scope.grader.id}-instructions`);
        const reason = page.locator(`#grader-${scope.grader.id}-reason`);
        if (reread) {
          await page.eventually(() =>
            expect(page.locator("summary").filter({ hasText: "新增評分器" }).count()).toBe(1),
          );
          await page.locator("summary").filter({ hasText: "新增評分器" }).click();
          await page.locator("#grader-create-name").fill("未送出的評分器");
          await page.locator("#grader-create-instructions").fill("未送出的新指示");
          await page.locator("#grader-create-reason").fill("未送出的新增原因");
          await page.getByText(`修改「${scope.grader.name}」的評分指示`, { exact: true }).click();
          await instructions.fill("未送出的修訂指示");
          await reason.fill("未送出的修訂原因");
          await page.getByRole("button", { name: "重新整理", exact: true }).click();
        }
        await page.eventually(() => {
          expect(
            page.getByRole("alert").filter({ hasText: "暫時無法讀取評分器" }).count(),
          ).toBeGreaterThan(0);
          expect(
            page.getByRole("heading", { name: scope.heading, exact: true }).count(),
          ).toBeGreaterThan(0);
          expect(page.getByRole("button", { name: "重新載入頁面", exact: true }).count()).toBe(0);
          expect(
            page.locator("h3").count() + page.getByText("還沒有評分器。", { exact: true }).count(),
          ).toBe(0);
        });
        if (reread) {
          expect(valueOf(`#grader-${scope.grader.id}-instructions`)).toBe("未送出的修訂指示");
          const list = document.querySelector(".grader-list");
          expect(list?.hasAttribute("hidden")).toBe(true);
          expect(list?.getAttribute("inert")).toBe("");
          expect(valueOf("#grader-create-name")).toBe("未送出的評分器");
        }
        expect(reads).toBe(reread ? 2 : 1);
        expect(writes()).toEqual([]);
        expect(await page.axeViolations("main")).toEqual([]);
        recovered = true;
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
        await page.eventually(() => {
          expect(page.getByRole("alert").count()).toBe(0);
          expect(
            page
              .getByRole("heading", {
                name: `${scope.grader.name} v${scope.grader.version}`,
                exact: true,
              })
              .count(),
          ).toBeGreaterThan(0);
          expect(page.locator("#grader-list-heading").isFocused()).toBe(true);
        });
        if (reread) {
          expect(instructions.value()).toBe("未送出的修訂指示");
          expect(reason.value()).toBe("未送出的修訂原因");
          expect(page.locator("#grader-create-name").value()).toBe("未送出的評分器");
          expect(page.locator("#grader-create-instructions").value()).toBe("未送出的新指示");
          expect(page.locator("#grader-create-reason").value()).toBe("未送出的新增原因");
        }
        expect(reads).toBe(reread ? 3 : 2);
        expect(writes()).toEqual([]);
        expect(await page.axeViolations("main")).toEqual([]);
      });
    }
  }
}
