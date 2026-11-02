import { expect, test } from "vitest";
import { TEST_CASE, TEST_CASE_DRAFT } from "../../../testing/fixtures/platform";
import { Locator, renderedApp } from "../../../testing/app";

const app = renderedApp();

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

function expectedFormValue(form: (typeof forms)[number], mode: string, index: number) {
  if (mode === "pristine" || mode === "saved" || (mode === "partial" && index > 0)) {
    return form.fresh[index];
  }
  return mode === "cleared" ? "" : form.owned[index];
}

const editable = (field: Locator) => !field.element().matches(":disabled, [readonly]");

for (const form of forms) {
  for (const mode of ["pristine", "partial", "cleared", "saved", "failed"]) {
    const expectedReads = mode === "failed" ? 3 : 2;
    test(`${form.name} draft ${mode} follows legitimate reads`, async () => {
      const page = app();
      const current = structuredClone(TEST_CASE_DRAFT);
      const writes: unknown[] = [];
      let reads = 0;
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route(`**/test-cases/${TEST_CASE}`, async ({ method, body }) => {
        if (method === "GET") {
          reads += 1;
          return { json: current };
        }
        expect(method).toBe("PATCH");
        writes.push(body);
        await held;
        if (mode === "failed") return { status: 500, json: { error: "更新失敗" } };
        updateServer(current);
        return { json: current };
      });
      page.route(`**/test-cases/${TEST_CASE}/criteria/c2`, ({ method, body }) => {
        expect(method).toBe("PATCH");
        writes.push(body);
        updateServer(current);
        return { json: current };
      });
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}`);
        const panel = page.getByRole("region", { name: form.panel, exact: true });
        for (let index = 0; index < form.selectors.length; index++) {
          const field = page.locator(form.selectors[index]);
          await page.eventually(() => expect(editable(field)).toBe(true));
          if (mode === "pristine" || (mode === "partial" && index > 0)) continue;
          const value = mode === "cleared" ? "" : form.owned[index];
          await field.fill(value);
          await page.eventually(() => expect(field.value()).toBe(value));
        }
        if (form.name === "rubric" && mode === "cleared") {
          await page.locator("#rubric-weight-c1").fill("");
          const evidence = page.locator("#rubric-evidence-c1");
          expect((evidence.element() as HTMLInputElement).checked).toBe(true);
          await evidence.click();
        }
        if (mode === "saved" || mode === "failed") {
          const save = panel.getByRole("button", {
            name: new RegExp(`^(?:${form.save}|儲存中…)$`),
          });
          await save.click();
          await page.eventually(() => {
            expect(save.text()).toBe("儲存中…");
            expect(save.isDisabled()).toBe(true);
            for (const selector of form.selectors) {
              expect(editable(page.locator(selector))).toBe(false);
            }
          });
          release();
          await page.eventually(() => {
            if (mode === "failed")
              expect(panel.getByRole("alert").text()).toBe(
                "操作結果尚未確認；請先確認最新資料，再決定是否重送。",
              );
            else expect(panel.getByRole("status").filter({ hasText: "已儲存。" }).count()).toBe(1);
          });
        }
        const second = new Locator(() => {
          const item = document.querySelector("#criterion-c2")?.closest("li.criterion");
          return item ? [item] : [];
        });
        if (mode !== "saved") {
          await second.getByRole("button", { name: "確認", exact: true }).click();
          await page.eventually(() => expect(second.text()).toContain("狀態：已確認"));
        }
        await page.eventually(() => {
          for (let index = 0; index < form.selectors.length; index++) {
            expect(page.locator(form.selectors[index]).value()).toBe(
              expectedFormValue(form, mode, index),
            );
          }
          if (form.name === "rubric" && mode === "cleared") {
            expect(page.locator("#rubric-weight-c1").value()).toBe("");
            expect(
              (page.locator("#rubric-evidence-c1").element() as HTMLInputElement).checked,
            ).toBe(false);
          }
          expect(reads).toBe(expectedReads);
        });
        expect(writes).toEqual(
          mode === "saved"
            ? [form.patch]
            : mode === "failed"
              ? [form.patch, { confirmed: true }]
              : [{ confirmed: true }],
        );
        const field = page.locator(form.selectors[0]);
        await field.click();
        expect(field.isFocused()).toBe(true);
        expect(await page.axeViolations(`section[aria-label="${form.panel}"]`)).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
