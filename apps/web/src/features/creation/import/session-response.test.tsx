import { act } from "react";
import { expect, test } from "vitest";
import { ME, SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const RECEIPT = {
  shape: "skill",
  skills: [
    {
      path: "",
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 1,
      content_hash: "a".repeat(64),
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
};

function accessibleDescription(element: HTMLElement): string {
  return (element.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();
}

for (const [name, reply] of [
  ["null", null],
  ["empty object", {}],
  ["string operator", { ...ME, operator: "false" }],
  ["object display name", { ...ME, display_name: { label: "tester" } }],
] as const) {
  for (const phase of ["initial", "foreground", "failed logout"] as const) {
    test(`a ${phase} session with ${name} stays unconfirmed until a valid reread`, async () => {
      const page = app();
      let invalid = phase === "initial";
      const imports: unknown[] = [];
      page.route("**/me", () => ({ json: invalid ? reply : ME }));
      page.route("**/auth/logout", () => ({ status: 503, json: { error: "service unavailable" } }));
      page.route("**/skills/import/url", (request) => {
        imports.push(request.body);
        return { status: 201, json: RECEIPT };
      });
      await page.goto("/workspace/import");
      const input = page.getByLabel("URL", { exact: true });
      const submit = page.getByRole("button", { name: "開始匯入", exact: true });
      const retry = page.getByRole("button", { name: "重新確認登入狀態", exact: true });
      if (phase !== "initial") {
        await page.eventually(() => {
          expect(page.locator(".auth-name").text()).toBe("tester");
          expect(submit.isDisabled()).toBe(false);
        });
      } else {
        await page.eventually(() => expect(retry.count()).toBe(1));
      }
      await input.fill("https://github.com/example/unsent");
      if (phase !== "initial") {
        invalid = true;
        if (phase === "failed logout")
          await page.getByRole("button", { name: "登出", exact: true }).click();
        else
          await act(async () => {
            document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
          });
      }
      await page.eventually(() => {
        expect(retry.count()).toBe(1);
        expect(page.locator("header").getByRole("alert").text()).toContain(
          phase === "failed logout" ? "未能確認登出結果" : "登入狀態暫時無法確認",
        );
        expect(
          page.getByRole("heading", { name: "匯入小工具", exact: true }).count(),
        ).toBeGreaterThan(0);
        expect(submit.isDisabled()).toBe(true);
        expect(accessibleDescription(submit.element())).toBe(
          "登入狀態尚未確認；請先在頁首確認，再送出匯入。",
        );
        expect(
          page.locator("header").getByRole("link", { name: "後台", exact: true }).count(),
        ).toBe(0);
      });
      await act(async () => {
        (page.locator(".import-source form").element() as HTMLFormElement).requestSubmit();
      });
      expect(imports).toEqual([]);
      expect(input.value()).toBe("https://github.com/example/unsent");
      expect(page.requests.filter((r) => r.url.pathname === "/auth/logout")).toHaveLength(
        phase === "failed logout" ? 1 : 0,
      );
      expect(await page.axeViolations("main")).toEqual([]);
      expect(await page.axeViolations(".app-header")).toEqual([]);
      invalid = false;
      await retry.click();
      await page.eventually(() => {
        expect(page.locator(".auth-name").text()).toBe("tester");
        expect(retry.count()).toBe(0);
        expect(submit.isDisabled()).toBe(false);
        expect(input.value()).toBe("https://github.com/example/unsent");
      });
      expect(imports).toEqual([]);
      await submit.click();
      await page.eventually(() =>
        expect(page.getByRole("status").filter({ hasText: "匯入完成" }).count()).toBe(1),
      );
      expect(imports).toEqual([{ url: "https://github.com/example/unsent" }]);
      expect(
        page.getByRole("link", { name: "開啟 v1 並繼續驗證", exact: true }).attribute("href"),
      ).toBe(`/skills/${SKILL}/versions/${VERSION}`);
    });
  }
}
