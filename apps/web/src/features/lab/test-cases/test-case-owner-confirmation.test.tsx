import { expect, test } from "vitest";
import { platformResponse, SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const EMPTY_SKILLS = { skills: [], total: 0, limit: 24, truncated: false };
const EMPTY_TEST_CASES = { test_cases: [], total: 0, limit: 50, truncated: false };

async function leaveAndReturn(page: ReturnType<typeof app>, beforeReturn: () => Promise<void>) {
  await page.locator('a[href="/policy"]').first().click();
  await expect(page.getByRole("heading", { name: "資料保存政策", exact: true })).toHaveCount(1);
  await beforeReturn();
  await page.goBack();
}

for (const offline of [false, true]) {
  for (const absent of [false, true]) {
    test(`test-case ownership waits for a ${offline ? "paused" : "fetching"} empty snapshot read that confirms ${absent ? "absence" : "ownership"}`, async () => {
      const page = app();
      let reads = 0;
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      page.route("**/skills?*", async () => {
        reads += 1;
        if (reads > 1) await gate;
        return { json: reads === 1 || absent ? EMPTY_SKILLS : platformResponse("/skills").body };
      });
      page.route("**/test-cases?*", () => ({ json: EMPTY_TEST_CASES }));
      const missing = page.getByRole("status").filter({ hasText: "這個小工具不在你的工作區" });
      const editor = page.locator("#new-test-case");
      const name = editor.getByLabel("名稱", { exact: true });
      const prompt = editor.getByLabel("User Prompt", { exact: true });
      const create = editor.getByRole("button", { name: "建立", exact: true });
      try {
        await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
        await expect(missing).toHaveCount(1);
        await leaveAndReturn(page, () => page.setOffline(offline));
        await page.eventually(() => {
          if (!offline) expect(reads).toBe(2);
          expect(missing.count()).toBe(0);
          expect(
            editor
              .getByRole("status")
              .filter({
                hasText: offline ? "等待連線以讀取你的小工具清單" : "載入你的小工具清單中",
              })
              .count(),
          ).toBe(1);
        });
        await name.fill("ownership draft");
        await prompt.fill("keep this task");
        await expect(create).toBeDisabled();
        expect(await page.axeViolations("body")).toEqual([]);
        await page.setOffline(false);
        await page.eventually(() => expect(reads).toBe(2));
        release();
        await page.eventually(() => {
          expect(editor.locator("[data-loading]").count()).toBe(0);
          if (absent) {
            expect(missing.count()).toBe(1);
            expect(editor.getByLabel("小工具", { exact: true }).value()).toBe("");
            expect(create.isDisabled()).toBe(true);
          } else {
            expect(missing.count()).toBe(0);
            expect(editor.getByLabel("小工具", { exact: true }).value()).toBe(SKILL);
            expect(create.isDisabled()).toBe(false);
          }
          expect(name.value()).toBe("ownership draft");
          expect(prompt.value()).toBe("keep this task");
        });
        const search = new URLSearchParams(page.search());
        expect(search.get("skill")).toBe(SKILL);
        expect(search.get("version")).toBe(VERSION);
        expect(reads).toBe(2);
        expect(await page.axeViolations("body")).toEqual([]);
      } finally {
        release();
        await page.setOffline(false);
      }
    });
  }
}

test("a paused test-case cached owner retry keeps its draft and waits before confirming ownership", async () => {
  const page = app();
  let reads = 0;
  page.route("**/skills?*", () => {
    reads += 1;
    if (reads === 1) return { json: EMPTY_SKILLS };
    return reads === 2
      ? { status: 503, json: { error: "service unavailable" } }
      : { json: platformResponse("/skills").body };
  });
  const editor = page.locator("#new-test-case");
  const missing = page.getByText("這個小工具不在你的工作區", { exact: false });
  const name = editor.getByLabel("名稱", { exact: true });
  const prompt = editor.getByLabel("User Prompt", { exact: true });
  try {
    await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
    await expect(missing).toHaveCount(1);
    await leaveAndReturn(page, async () => {});
    await expect(editor.getByRole("alert")).toHaveCount(1);
    await name.fill("retry draft");
    await prompt.fill("keep retry task");
    await page.setOffline(true);
    await editor.getByRole("button", { name: "再試一次", exact: true }).focus();
    await page.press("Enter");
    await page.eventually(() => {
      expect(editor.getByRole("button", { name: "重新讀取中…", exact: true }).isDisabled()).toBe(
        true,
      );
      expect(
        editor.getByRole("status").filter({ hasText: "等待連線以讀取你的小工具清單" }).count(),
      ).toBe(1);
      expect(page.locator("#create-test-case-heading").isFocused()).toBe(true);
      expect(missing.count()).toBe(0);
    });
    expect(reads).toBe(2);
    expect(await page.axeViolations("body")).toEqual([]);
    await page.setOffline(false);
    await page.eventually(() => {
      expect(editor.getByLabel("小工具", { exact: true }).value()).toBe(SKILL);
      expect(editor.getByRole("alert").count()).toBe(0);
      expect(name.value()).toBe("retry draft");
      expect(prompt.value()).toBe("keep retry task");
    });
    expect(reads).toBe(3);
    expect(await page.axeViolations("body")).toEqual([]);
  } finally {
    await page.setOffline(false);
  }
});
