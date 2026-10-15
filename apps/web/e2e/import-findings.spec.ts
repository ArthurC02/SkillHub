import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import type { ImportResult } from "../src/core/api/types";
import { SKILL, SKILL_B, SKILL_C, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

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

for (const source of ["url", "zip"] as const) {
  test(`${source} import keeps each skill's inspection summary visible before opening details`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 900 });
    await stubPlatform(page);
    await page.route(`**/skills/import/${source === "url" ? "url" : "upload"}`, (route) =>
      route.fulfill({ status: 201, json: RECEIPT }),
    );
    await page.goto("/workspace/import");
    const submit = page.getByRole("button", { name: "開始匯入", exact: true });
    await expect(submit).toBeEnabled();
    if (source === "url")
      await page.getByLabel("URL", { exact: true }).fill("https://github.com/example/skill");
    else {
      await page.getByLabel("上傳 zip", { exact: true }).check();
      await page.getByLabel("小工具 zip", { exact: true }).setInputFiles({
        name: "skills.zip",
        mimeType: "application/zip",
        buffer: Buffer.from("zip bytes"),
      });
    }
    await submit.click();
    await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
    await page.screenshot({
      path: test.info().outputPath("import-findings-phone.png"),
      fullPage: true,
    });
    const outcomes = page
      .getByRole("heading", { level: 2, name: "進來的小工具（3）", exact: true })
      .locator("..")
      .locator(":scope > ul > li");
    await expect(outcomes).toHaveCount(3);
    for (const [index, name, summary] of [
      [0, "套件根目錄", "靜態檢查：阻擋錯誤 0／警告 0／資訊 0"],
      [1, "skills/with-warning", "靜態檢查：阻擋錯誤 0／警告 1／資訊 1"],
      [2, "skills/with-info", "靜態檢查：阻擋錯誤 0／警告 0／資訊 1"],
    ] as const) {
      const outcome = outcomes.nth(index);
      await expect(outcome.getByText(summary, { exact: true })).toBeVisible();
      expect(
        await outcome
          .getByText(summary, { exact: true })
          .evaluate((node) => node.closest("details") === null),
      ).toBe(true);
      await expect(outcome.getByRole("heading", { level: 3, name, exact: true })).toBeVisible();
      await expect(outcome.locator(":scope > details")).not.toHaveAttribute("open");
      await outcome.getByText("靜態檢查結果", { exact: true }).click();
    }
    const warning = outcomes.nth(1);
    await expect(
      warning.getByRole("heading", { level: 4, name: "警告（1）", exact: true }),
    ).toBeVisible();
    await expect(warning.getByText("description is long", { exact: false })).toBeVisible();
    await expect(
      warning.getByRole("link", { name: "開啟 v2 並繼續驗證", exact: true }),
    ).toHaveAttribute("href", `/skills/${SKILL_B}/versions/33333333-3333-3333-3333-333333333333`);
    await expect(
      outcomes.nth(0).getByText("靜態檢查跑完了，沒有任何發現", { exact: false }),
    ).toBeVisible();
    await page.evaluate(() => scrollTo(0, 0));
    const report = await new AxeBuilder({ page }).include("main").analyze();
    await writeFile(test.info().outputPath("import-findings-a11y.json"), JSON.stringify(report));
    expect(report.violations).toEqual([]);
    expect(report.incomplete).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: test.info().outputPath("import-findings-expanded-phone.png"),
      fullPage: true,
    });
  });
}
