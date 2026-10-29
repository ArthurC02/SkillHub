import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE, TEST_CASE_DRAFT } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const forms = [
  {
    name: "prompt",
    panel: "任務提示設計",
    save: "儲存",
    selectors: ["#edit-name", "#edit-prompt"],
    fresh: ["伺服器新名稱", "伺服器新版任務"],
    owned: ["自己的名稱", "自己未送出的任務"],
    patch: { name: "自己的名稱", user_prompt: "自己未送出的任務" },
  },
  {
    name: "facts",
    panel: "模擬使用者設計",
    save: "儲存模擬使用者",
    selectors: ["#simulated-user-facts"],
    fresh: ["伺服器新背景"],
    owned: ["自己未送出的背景"],
    patch: { simulated_user_facts: ["自己未送出的背景"] },
  },
  {
    name: "rubric",
    panel: "驗收與評分設計",
    save: "儲存 Rubric",
    selectors: ["#rubric-version", "#rubric-c1"],
    fresh: ["server/v2", "伺服器新評分說明"],
    owned: ["own/v1", "自己未送出的評分說明"],
    patch: {
      rubric: {
        version: "own/v1",
        items: [{ ...TEST_CASE_DRAFT.rubric.items[0], text: "自己未送出的評分說明" }],
      },
    },
  },
] as const;

function updateServer(current: typeof TEST_CASE_DRAFT) {
  current.name = "伺服器新名稱";
  current.user_prompt = "伺服器新版任務";
  current.simulated_user_facts = ["伺服器新背景"];
  current.rubric.version = "server/v2";
  current.rubric.items[0].text = "伺服器新評分說明";
  current.acceptance_criteria[1].confirmed_at = "2026-08-18T01:00:00Z";
  current.updated_at = "2026-08-18T01:00:00Z";
}

async function inspectForm(page: Page, info: TestInfo, panel: string) {
  const axe = await new AxeBuilder({ page }).include(`section[aria-label="${panel}"]`).analyze();
  await writeFile(info.outputPath("form-draft-a11y.json"), JSON.stringify(axe));
  expect(axe.violations).toEqual([]);
  expect(axe.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("form-draft.png") });
}

function expectedFormValue(form: (typeof forms)[number], mode: string, index: number) {
  if (mode === "pristine" || mode === "saved" || (mode === "partial" && index > 0)) {
    return form.fresh[index];
  }
  return mode === "cleared" ? "" : form.owned[index];
}

for (const form of forms) {
  for (const mode of ["pristine", "partial", "cleared", "saved", "failed"]) {
    const expectedReads = mode === "failed" ? 3 : 2;
    for (const width of [375, 1280]) {
      test(`${form.name} draft ${mode} follows legitimate reads at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        const current = structuredClone(TEST_CASE_DRAFT);
        const writes: unknown[] = [];
        let reads = 0;
        let release!: () => void;
        const held = new Promise<void>((resolve) => {
          release = resolve;
        });
        await page.route(`**/test-cases/${TEST_CASE}`, async (route) => {
          if (route.request().isNavigationRequest()) return route.fallback();
          if (route.request().method() === "GET") {
            reads += 1;
            return route.fulfill({ json: current });
          }
          expect(route.request().method()).toBe("PATCH");
          writes.push(route.request().postDataJSON());
          await held;
          if (mode === "failed") return route.fulfill({ status: 500, json: { error: "更新失敗" } });
          updateServer(current);
          return route.fulfill({ json: current });
        });
        await page.route(`**/test-cases/${TEST_CASE}/criteria/c2`, (route) => {
          expect(route.request().method()).toBe("PATCH");
          writes.push(route.request().postDataJSON());
          updateServer(current);
          return route.fulfill({ json: current });
        });
        try {
          await page.goto(`/lab/test-cases/${TEST_CASE}`);
          const panel = page.getByRole("region", { name: form.panel, exact: true });
          for (let index = 0; index < form.selectors.length; index++) {
            const field = page.locator(form.selectors[index]);
            await expect(field).toBeEditable();
            if (mode === "pristine" || (mode === "partial" && index > 0)) continue;
            await field.fill(mode === "cleared" ? "" : form.owned[index]);
            await expect(field).toHaveValue(mode === "cleared" ? "" : form.owned[index]);
          }
          if (form.name === "rubric" && mode === "cleared") {
            await page.locator("#rubric-weight-c1").fill("");
            await page.locator("#rubric-evidence-c1").uncheck();
          }
          if (mode === "saved" || mode === "failed") {
            const save = panel.getByRole("button", {
              name: new RegExp(`^(?:${form.save}|儲存中…)$`),
            });
            await save.click();
            await expect(save).toHaveText("儲存中…");
            await expect(save).toBeDisabled();
            for (const selector of form.selectors) {
              await expect(page.locator(selector)).not.toBeEditable();
            }
            release();
            if (mode === "failed") {
              await expect(panel.getByRole("alert")).toHaveText(
                "操作結果尚未確認；請先確認最新資料，再決定是否重送。",
              );
            } else {
              await expect(panel.getByRole("status").filter({ hasText: "已儲存。" })).toHaveCount(
                1,
              );
            }
          }
          if (mode !== "saved") {
            const second = page
              .locator("li.criterion")
              .filter({ has: page.locator("#criterion-c2") });
            await second.getByRole("button", { name: "確認", exact: true }).click();
            await expect(second).toContainText("狀態：已確認");
          }
          for (let index = 0; index < form.selectors.length; index++) {
            await expect(page.locator(form.selectors[index])).toHaveValue(
              expectedFormValue(form, mode, index),
            );
          }
          if (form.name === "rubric" && mode === "cleared") {
            await expect(page.locator("#rubric-weight-c1")).toHaveValue("");
            await expect(page.locator("#rubric-evidence-c1")).not.toBeChecked();
          }
          await expect.poll(() => reads).toBe(expectedReads);
          expect(writes).toEqual(
            mode === "saved"
              ? [form.patch]
              : mode === "failed"
                ? [form.patch, { confirmed: true }]
                : [{ confirmed: true }],
          );
          const field = page.locator(form.selectors[0]);
          await field.click();
          await expect(field).toBeFocused();
          await expect(field).toBeInViewport({ ratio: 1 });
          await inspectForm(page, info, form.panel);
        } finally {
          release();
        }
      });
    }
  }
}
