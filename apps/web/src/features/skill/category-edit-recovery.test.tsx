import { act } from "react";
import { expect, test } from "vitest";
import { SKILL, SKILL_VERSIONS, skillDetail } from "../../testing/fixtures/platform";
import { renderedApp, type Locator, type Reply } from "../../testing/app";
import { stubScrollIntoView } from "../../testing/dom";

const app = renderedApp();
stubScrollIntoView();

const RECEIPT = { skill_id: SKILL, name: "PDF Summariser", summary: "", redistribution: "allowed" };
const WRITING = { value: "writing", label: "寫作", note: "由擁有者設定。" };
const DOCUMENTS = { value: "documents", label: "文件", note: "由擁有者設定。" };
const CHOICE_LABEL = "這個小工具是做什麼用的";

function gate() {
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { release, held };
}

async function submitForm(choice: Locator) {
  const form = choice.element().closest("form")!;
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

function errorBody(status: number) {
  return status === 401 ? "not authenticated" : status === 403 ? "forbidden" : "not found";
}

for (const outcome of [
  "accepted",
  "unavailable",
  "lost response",
  "malformed receipt",
  "rejected",
  "unauthenticated",
] as const) {
  test(`a ${outcome} category write describes its submitted choice and preserves the next choice`, async () => {
    const page = app();
    const writes: unknown[] = [];
    let current = DOCUMENTS;
    let reads = 0;
    page.route(`**/api/skills/${SKILL}`, () => {
      reads += 1;
      return { json: { ...skillDetail(SKILL, "PDF Summariser"), category: current } };
    });
    const { release, held } = gate();
    page.route(`**/skills/${SKILL}/category`, async ({ body }): Promise<Reply> => {
      writes.push(body);
      await held;
      if (outcome === "rejected") return { status: 400, json: { error: "invalid category" } };
      if (outcome === "unauthenticated")
        return { status: 401, json: { error: "not authenticated" } };
      current = WRITING;
      if (outcome === "unavailable") return { status: 503, json: { error: "service unavailable" } };
      if (outcome === "lost response") return "network-error";
      if (outcome === "malformed receipt") return { json: {} };
      return { json: RECEIPT };
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const choice = page.getByLabel(CHOICE_LABEL, { exact: true });
      await choice.select("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(
        page.getByRole("status").filter({ hasText: "正在儲存「寫作」類別。" }),
      ).toBeVisible();
      await choice.select("data");
      await choice.focus();
      await submitForm(choice);
      release();
      const category = page.getByRole("region", { name: "類別", exact: true });
      const message =
        outcome === "accepted"
          ? "類別已更新為「寫作」。"
          : outcome === "unauthenticated"
            ? "設定類別需要登入。"
            : outcome === "rejected"
              ? "「寫作」類別的儲存請求未獲接受。"
              : "尚未確認「寫作」類別是否已儲存。";
      await expect(category).toContainText(message);
      await expect(choice).toHaveValue("data");
      await expect(choice).toBeFocused();
      await expect(page.getByRole("button", { name: "儲存", exact: true })).toBeEnabled();
      expect(reads).toBe(2);
      expect(writes).toEqual([{ category: "writing" }]);
      if (outcome !== "accepted") {
        await expect(category).not.toContainText("類別已更新");
        await category.getByRole("button", { name: "重新確認編輯條件", exact: true }).click();
        await expect(page.getByRole("button", { name: "儲存", exact: true })).toBeEnabled();
        await page.eventually(() => expect(reads).toBe(3));
        expect(writes).toEqual([{ category: "writing" }]);
        await expect(category).toContainText(message);
        await expect(choice).toHaveValue("data");
      }
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
    }
  });
}

for (const status of [401, 403, 404, 410, 200]) {
  test(`HTTP ${status} in the owned version reread discards category editing when ownership is absent`, async () => {
    const page = app();
    let revoked = false;
    page.route(`**/skills/${SKILL}/versions`, () => {
      if (!revoked) return { json: SKILL_VERSIONS };
      return status === 200
        ? { json: { versions: [] } }
        : { status, json: { error: errorBody(status) } };
    });
    await page.goto(`/skills/${SKILL}`);
    const choice = page.getByLabel(CHOICE_LABEL, { exact: true });
    await choice.select("data");
    revoked = true;
    const history = page.getByRole("region", { name: "版本", exact: true });
    await history.getByRole("button", { name: "重新整理", exact: true }).click();
    await expect(choice).toHaveCount(0);
    await expect(page.getByRole("region", { name: "類別", exact: true })).toHaveCount(0);
    revoked = false;
    if (status !== 401) {
      await history
        .getByRole("button", { name: status === 200 ? "重新整理" : "再試一次", exact: true })
        .click();
      await expect(choice).toHaveValue("documents");
    }
  });
}

for (const status of [401, 403, 404, 410]) {
  test(`HTTP ${status} in the public skill reread clears the private category choice and receipt`, async () => {
    const page = app();
    let saved = false;
    let recovered = false;
    const { release, held } = gate();
    page.route(`**/api/skills/${SKILL}`, async (): Promise<Reply | undefined> => {
      if (saved && !recovered) {
        await held;
        return { status, json: { error: errorBody(status) } };
      }
      return { json: skillDetail(SKILL, "PDF Summariser") };
    });
    page.route(`**/skills/${SKILL}/category`, () => {
      saved = true;
      return { json: RECEIPT };
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const choice = page.getByLabel(CHOICE_LABEL, { exact: true });
      await choice.select("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(page.getByRole("status").filter({ hasText: "類別已更新" })).toBeVisible();
      await choice.select("data");
      release();
      await expect(choice).toHaveCount(0);
      await expect(page.getByRole("status").filter({ hasText: "類別已更新" })).toHaveCount(0);
      await expect(page.locator(".detail-answer, .detail-main")).toHaveCount(0);
      await expect(page.locator(".detail-rail > *")).toHaveCount(0);
      await expect(page.locator("h1")).toHaveText("小工具總覽");
      if (status === 404) {
        await expect(page.getByRole("alert")).toHaveText(
          "找不到這個小工具；它可能已被刪除，或你沒有權限看到它。",
        );
        await expect(page.getByRole("button", { name: "再試一次", exact: true })).toHaveCount(0);
      }
      if (status === 403) {
        recovered = true;
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
        await expect(choice).toHaveValue("documents");
        await expect(page.getByRole("status").filter({ hasText: "類別已更新" })).toHaveCount(0);
      }
    } finally {
      release();
    }
  });
}

for (const source of ["skill", "versions"]) {
  test(`a category receipt and next choice survive a failed ${source} reread and offline recovery`, async () => {
    const page = app();
    let saved = false;
    let recovered = false;
    const writes: unknown[] = [];
    const { release, held } = gate();
    page.route(`**/api/skills/${SKILL}`, async (): Promise<Reply> => {
      if (saved && !recovered && source === "skill") {
        await held;
        return { status: 503, json: { error: "service unavailable" } };
      }
      return {
        json: { ...skillDetail(SKILL, "PDF Summariser"), category: saved ? WRITING : DOCUMENTS },
      };
    });
    page.route(`**/skills/${SKILL}/versions`, async (): Promise<Reply> => {
      if (saved && !recovered && source === "versions") {
        await held;
        return { status: 503, json: { error: "service unavailable" } };
      }
      return { json: SKILL_VERSIONS };
    });
    page.route(`**/skills/${SKILL}/category`, ({ body }) => {
      writes.push(body);
      saved = true;
      return { json: RECEIPT };
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const choice = page.getByLabel(CHOICE_LABEL, { exact: true });
      await choice.select("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(page.getByRole("status").filter({ hasText: "類別已更新" })).toBeVisible();
      await choice.select("data");
      await choice.focus();
      release();
      await expect(
        page.getByRole("alert").filter({
          hasText: source === "skill" ? "暫時無法讀取這個小工具。" : "暫時無法讀取版本歷史。",
        }),
      ).toBeVisible();
      await expect(choice).toHaveValue("data");
      await expect(choice).toBeFocused();
      await expect(
        page.getByRole("status").filter({ hasText: "類別已更新為「寫作」" }),
      ).toBeVisible();
      await expect(page.getByRole("button", { name: "儲存", exact: true })).toBeDisabled();
      await submitForm(choice);
      expect(writes).toEqual([{ category: "writing" }]);
      expect(await page.axeViolations("body")).toEqual([]);
      await page.setOffline(true);
      await page.getByRole("button", { name: "重新確認編輯條件", exact: true }).focus();
      await page.press("Enter");
      await expect(page.getByRole("button", { name: "重新確認中…", exact: true })).toBeDisabled();
      await expect(
        page.getByRole("status").filter({ hasText: "等待連線以確認編輯條件" }),
      ).toBeVisible();
      await expect(choice).toHaveValue("data");
      await expect(page.locator("#main-content")).toBeFocused();
      recovered = true;
      await page.setOffline(false);
      await expect(page.getByRole("button", { name: "儲存", exact: true })).toBeEnabled();
      await expect(choice).toHaveValue("data");
      await expect(page.locator("header .badge-row")).toContainText("寫作");
      expect(writes).toEqual([{ category: "writing" }]);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
      await page.setOffline(false);
    }
  });
}
