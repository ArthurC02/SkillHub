import { setOffline } from "./offline";
import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import { SKILL, SKILL_VERSIONS, skillDetail } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

async function verifyLayout(page: Page, state: string) {
  const results = await new AxeBuilder({ page }).analyze();
  const report = test.info().outputPath(`${state}-accessibility.json`);
  await writeFile(report, JSON.stringify(results));
  await test.info().attach(`${state}-accessibility`, {
    path: report,
    contentType: "application/json",
  });
  expect(results.violations).toEqual([]);
  expect(results.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
  await page.screenshot({ path: test.info().outputPath(`${state}.png`), fullPage: true });
}

for (const outcome of [
  "accepted",
  "unavailable",
  "lost response",
  "malformed receipt",
  "rejected",
  "unauthenticated",
] as const) {
  test(`a ${outcome} category write describes its submitted choice and preserves the next choice`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    const writes: unknown[] = [];
    let current = { value: "documents", label: "文件", note: "由擁有者設定。" };
    let reads = 0;
    await page.route(`**/api/skills/${SKILL}`, (route) => {
      reads += 1;
      return route.fulfill({
        json: { ...skillDetail(SKILL, "PDF Summariser"), category: current },
      });
    });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/skills/${SKILL}/category`, async (route) => {
      writes.push(route.request().postDataJSON());
      await gate;
      if (outcome === "rejected")
        return route.fulfill({ status: 400, json: { error: "invalid category" } });
      if (outcome === "unauthenticated")
        return route.fulfill({ status: 401, json: { error: "not authenticated" } });
      current = { value: "writing", label: "寫作", note: "由擁有者設定。" };
      if (outcome === "unavailable")
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      if (outcome === "lost response") return route.abort("failed");
      if (outcome === "malformed receipt") return route.fulfill({ json: {} });
      return route.fulfill({
        json: { skill_id: SKILL, name: "PDF Summariser", summary: "", redistribution: "allowed" },
      });
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const choice = page.getByLabel("這個小工具是做什麼用的", { exact: true });
      await choice.selectOption("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(
        page.getByRole("status").filter({ hasText: "正在儲存「寫作」類別。" }),
      ).toBeVisible();
      await choice.selectOption("data");
      await choice.focus();
      await choice.evaluate((element) =>
        element
          .closest("form")!
          .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
      );
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
        expect(reads).toBe(3);
        expect(writes).toEqual([{ category: "writing" }]);
        await expect(category).toContainText(message);
        await expect(choice).toHaveValue("data");
      }
      await verifyLayout(page, outcome.replaceAll(" ", "-"));
    } finally {
      release();
    }
  });
}

for (const status of [401, 403, 404, 410, 200]) {
  test(`HTTP ${status} in the owned version reread discards category editing when ownership is absent`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let revoked = false;
    await page.route(`**/skills/${SKILL}/versions`, (route) =>
      route.fulfill(
        revoked
          ? status === 200
            ? { json: { versions: [] } }
            : {
                status,
                json: {
                  error:
                    status === 401
                      ? "not authenticated"
                      : status === 403
                        ? "forbidden"
                        : "not found",
                },
              }
          : { json: SKILL_VERSIONS },
      ),
    );
    await page.goto(`/skills/${SKILL}`);
    const choice = page.getByLabel("這個小工具是做什麼用的", { exact: true });
    await choice.selectOption("data");
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
  test(`HTTP ${status} in the public skill reread clears the private category choice and receipt`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let saved = false;
    let recovered = false;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/api/skills/${SKILL}`, async (route) => {
      if (saved && !recovered) {
        await gate;
        return route.fulfill({
          status,
          json: {
            error:
              status === 401 ? "not authenticated" : status === 403 ? "forbidden" : "not found",
          },
        });
      }
      return route.fulfill({ json: skillDetail(SKILL, "PDF Summariser") });
    });
    await page.route(`**/skills/${SKILL}/category`, (route) => {
      saved = true;
      return route.fulfill({
        json: { skill_id: SKILL, name: "PDF Summariser", summary: "", redistribution: "allowed" },
      });
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const choice = page.getByLabel("這個小工具是做什麼用的", { exact: true });
      await choice.selectOption("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(page.getByRole("status").filter({ hasText: "類別已更新" })).toBeVisible();
      await choice.selectOption("data");
      release();
      await expect(choice).toHaveCount(0);
      await expect(page.getByRole("status").filter({ hasText: "類別已更新" })).toHaveCount(0);
      await expect(page.locator(".detail-answer, .detail-main")).toHaveCount(0);
      await expect(page.locator(".detail-rail")).toBeHidden();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("小工具總覽");
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
  test(`a category receipt and next choice survive a failed ${source} reread and offline recovery`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let saved = false;
    let recovered = false;
    const writes: unknown[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/api/skills/${SKILL}`, async (route) => {
      if (saved && !recovered && source === "skill") {
        await gate;
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      }
      return route.fulfill({
        json: {
          ...skillDetail(SKILL, "PDF Summariser"),
          category: saved
            ? { value: "writing", label: "寫作", note: "由擁有者設定。" }
            : { value: "documents", label: "文件", note: "由擁有者設定。" },
        },
      });
    });
    await page.route(`**/skills/${SKILL}/versions`, async (route) => {
      if (saved && !recovered && source === "versions") {
        await gate;
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      }
      return route.fulfill({ json: SKILL_VERSIONS });
    });
    await page.route(`**/skills/${SKILL}/category`, (route) => {
      writes.push(route.request().postDataJSON());
      saved = true;
      return route.fulfill({
        json: { skill_id: SKILL, name: "PDF Summariser", summary: "", redistribution: "allowed" },
      });
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const choice = page.getByLabel("這個小工具是做什麼用的", { exact: true });
      await choice.selectOption("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(page.getByRole("status").filter({ hasText: "類別已更新" })).toBeVisible();
      await choice.selectOption("data");
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
      await choice.evaluate((element) =>
        element
          .closest("form")!
          .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
      );
      expect(writes).toEqual([{ category: "writing" }]);
      await verifyLayout(page, "read-failed");
      await setOffline(page, true);
      await page.getByRole("button", { name: "重新確認編輯條件", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(page.getByRole("button", { name: "重新確認中…", exact: true })).toBeDisabled();
      await expect(
        page.getByRole("status").filter({ hasText: "等待連線以確認編輯條件" }),
      ).toBeVisible();
      await expect(choice).toHaveValue("data");
      await expect(page.locator("#main-content")).toBeFocused();
      recovered = true;
      await setOffline(page, false);
      await expect(page.getByRole("button", { name: "儲存", exact: true })).toBeEnabled();
      await expect(choice).toHaveValue("data");
      await expect(page.locator("header .badge-row")).toContainText("寫作");
      expect(writes).toEqual([{ category: "writing" }]);
      await verifyLayout(page, "recovered");
    } finally {
      release();
      await setOffline(page, false);
    }
  });
}
