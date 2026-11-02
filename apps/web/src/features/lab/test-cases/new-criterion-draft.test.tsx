import { expect, test, vi } from "vitest";
import { TEST_CASE, TEST_CASE_DRAFT } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

vi.mock("../../evaluation/components/CheckFlow", () => ({ CheckFlow: () => null }));

const app = renderedApp();

const scenarios = [
  { name: "unchanged check", initialCheck: true, edit: "none", text: "", check: null },
  { name: "newer text", initialCheck: true, edit: "text", text: "下一條草稿", check: " 42 " },
  {
    name: "newer check",
    initialCheck: false,
    edit: "check",
    text: "已送出的這一條",
    check: "下一條的值",
  },
  {
    name: "removed check",
    initialCheck: true,
    edit: "remove",
    text: "已送出的這一條",
    check: null,
  },
  {
    name: "failed newer text",
    initialCheck: true,
    edit: "text",
    text: "下一條草稿",
    check: " 42 ",
  },
];

for (const scenario of scenarios) {
  for (const width of [375, 1280]) {
    test(`new criterion preserves ${scenario.name} during submission at ${width}px`, async () => {
      const page = app();
      const current = structuredClone(TEST_CASE_DRAFT);
      const writes: unknown[] = [];
      let reads = 0;
      let release!: () => void;
      let started!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      const received = new Promise<void>((resolve) => {
        started = resolve;
      });
      const failed = scenario.name.startsWith("failed");
      page.route(
        (url) => url.pathname.endsWith(`/test-cases/${TEST_CASE}`),
        ({ method }) => {
          if (method !== "GET") return undefined;
          reads += 1;
          return { json: current };
        },
      );
      page.route(
        (url) => url.pathname.endsWith(`/test-cases/${TEST_CASE}/criteria`),
        async ({ method, body }) => {
          writes.push({ method, body });
          started();
          await held;
          if (failed) return { status: 500, json: { error: "新增驗收條件失敗" } };
          current.acceptance_criteria.push({
            ...current.acceptance_criteria[0],
            id: "c-added",
            text: "已送出的這一條",
            source: "user",
            confirmed_at: null,
          });
          return { status: 201, json: current };
        },
      );
      try {
        await page.goto(`/lab/test-cases/${TEST_CASE}`);
        const field = page.locator("#new-criterion");
        const checkField = page.getByRole("textbox", { name: "第 1 個", exact: true });
        const add = page.getByRole("button", { name: /^新增(?:中…)?$/ });
        const designCheck = async (value: string) => {
          await page.getByRole("button", { name: "開始設計", exact: true }).click();
          await expect(checkField).toHaveCount(1);
          await checkField.fill(value);
        };
        await expect(field).toHaveValue("");
        await field.fill("已送出的這一條");
        if (scenario.initialCheck) {
          await designCheck(" 42 ");
        }
        await expect(add).toBeEnabled();
        await add.click();
        await received;
        await page.eventually(() => {
          expect(add.text()).toBe("新增中…");
          expect(add.isDisabled()).toBe(true);
          expect(field.isDisabled()).toBe(false);
          expect(field.attribute("readonly")).toBeNull();
        });
        if (scenario.edit === "text") await field.fill("下一條草稿");
        else if (scenario.edit === "check") {
          await designCheck("下一條的值");
        } else if (scenario.edit === "remove") {
          await page.getByRole("button", { name: "拿掉整個檢查", exact: true }).click();
        }
        release();
        if (failed)
          await expect(page.getByRole("alert")).toHaveText(
            "尚未確認這一條驗收條件是否已新增；條件清單已重新讀取，請先確認，再決定要不要重送，以免重複。",
          );
        else await expect(page.locator("#criterion-c-added")).toHaveValue("已送出的這一條");
        const receipt = page.getByRole("status").filter({ hasText: "已新增驗收條件。" });
        await page.eventually(() => {
          expect(add.text()).toBe("新增");
          expect(field.value()).toBe(scenario.text);
          if (scenario.check === null) expect(checkField.count()).toBe(0);
          else expect(checkField.value()).toBe(scenario.check);
          expect(add.isDisabled()).toBe(!scenario.text);
          if (failed) expect(receipt.count()).toBe(0);
          else
            expect(receipt.text()).toBe(
              scenario.edit === "none"
                ? "已新增驗收條件。"
                : "已新增驗收條件。目前欄位裡的草稿尚未新增。",
            );
          expect(reads).toBe(2);
        });
        expect(writes).toEqual([
          {
            method: "POST",
            body: {
              source: "user",
              text: "已送出的這一條",
              ...(scenario.initialCheck
                ? { check: { kind: "contains_values", values: ["42"] } }
                : {}),
            },
          },
        ]);
        await field.click();
        await expect(field).toBeFocused();
        expect(await page.axeViolations('section[aria-label="驗收與評分設計"]')).toEqual([]);
      } finally {
        release();
      }
    });
  }
}
