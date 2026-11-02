import { act } from "react";
import { expect, test } from "vitest";
import type { ImportResult } from "../../../core/api/types";
import { SKILL, SKILL_B, SKILL_C, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

const RECEIPT: ImportResult = {
  shape: "tree",
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
    {
      path: "skills/with-warning",
      skill_id: SKILL_B,
      version_id: "33333333-3333-3333-3333-333333333333",
      version_number: 2,
      content_hash: "b".repeat(64),
      duplicate: true,
      findings: {
        errors: [],
        warnings: [
          {
            severity: "warning",
            code: "frontmatter.long_description",
            message: "description is long",
            path: "SKILL.md",
          },
        ],
        infos: [
          {
            severity: "info",
            code: "external-url",
            message: "SKILL.md 內有外部網址",
            details: ["example.com"],
          },
        ],
      },
    },
    {
      path: "skills/with-info",
      skill_id: SKILL_C,
      version_id: "44444444-4444-4444-4444-444444444444",
      version_number: 3,
      content_hash: "c".repeat(64),
      duplicate: false,
      findings: {
        errors: [],
        warnings: [],
        infos: [{ severity: "info", code: "external-url", message: "SKILL.md 內有外部網址" }],
      },
    },
  ],
  refused: [],
  excluded_components: [],
};

async function chooseZipWithoutNativeValidation(page: App, name: string, content: string) {
  const input = page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement;
  const file = new File([content], name, { type: "application/zip" });
  Object.defineProperty(input, "files", { configurable: true, value: [file] });
  input.removeAttribute("required");
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

for (const source of ["url", "zip"] as const) {
  test(`${source} import keeps each skill's inspection summary visible before opening details`, async () => {
    const page = app();
    page.route(`**/skills/import/${source === "url" ? "url" : "upload"}`, () => ({
      status: 201,
      json: RECEIPT,
    }));
    await page.goto("/workspace/import");
    const submit = page.getByRole("button", { name: "開始匯入", exact: true });
    await page.eventually(() => expect(submit.isDisabled()).toBe(false));
    if (source === "url")
      await page.getByLabel("URL", { exact: true }).fill("https://github.com/example/skill");
    else {
      await page.getByLabel("上傳 zip", { exact: true }).check();
      await chooseZipWithoutNativeValidation(page, "skills.zip", "zip bytes");
    }
    await submit.click();
    await page.eventually(() =>
      expect(page.getByRole("status").filter({ hasText: "匯入完成" }).count()).toBe(1),
    );
    const outcomes = page.locator("main section > ul > li").filter({ hasText: "靜態檢查：" });
    await page.eventually(() => {
      expect(page.getByRole("heading", { name: "進來的小工具（3）", exact: true }).count()).toBe(1);
      expect(outcomes.count()).toBe(3);
    });
    for (const [index, name, summary] of [
      [0, "套件根目錄", "靜態檢查：阻擋錯誤 0／警告 0／資訊 0"],
      [1, "skills/with-warning", "靜態檢查：阻擋錯誤 0／警告 1／資訊 1"],
      [2, "skills/with-info", "靜態檢查：阻擋錯誤 0／警告 0／資訊 1"],
    ] as const) {
      const outcome = outcomes.nth(index);
      expect(outcome.locator("p.risk-counts").text()).toBe(summary);
      expect(outcome.locator("p.risk-counts").element().closest("details")).toBeNull();
      expect(outcome.locator("h3").text()).toBe(name);
      expect(outcome.locator("details").attribute("open")).toBeNull();
      await outcome.locator("summary").filter({ hasText: "靜態檢查結果" }).click();
    }
    const warning = outcomes.nth(1);
    await page.eventually(() => {
      expect(warning.locator("h4").filter({ hasText: "警告（1）" }).count()).toBe(1);
      expect(warning.text()).toContain("description is long");
      expect(warning.locator("a").filter({ hasText: "開啟 v2 並繼續驗證" }).attribute("href")).toBe(
        `/skills/${SKILL_B}/versions/33333333-3333-3333-3333-333333333333`,
      );
      expect(outcomes.nth(0).text()).toContain("靜態檢查跑完了，沒有任何發現");
    });
    expect(await page.axeViolations("main")).toEqual([]);
  });
}
