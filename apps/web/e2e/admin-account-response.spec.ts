import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_ACCOUNT, ME } from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

async function openLookup(page: Page) {
  await page.setViewportSize({ width: 375, height: 812 });
  await stubPlatform(page);
  await page.route("**/me", (route) => route.fulfill({ json: { ...ME, operator: true } }));
  let documents = 0;
  const writes: string[] = [];
  page.on("domcontentloaded", () => documents++);
  page.on("request", (request) => {
    if (request.method() === "POST") writes.push(request.url());
  });
  await page.goto("/admin/accounts");
  await page.getByLabel("Email", { exact: true }).fill("Member+Ops@Example.com");
  return {
    writes,
    get documents() {
      return documents;
    },
  };
}

for (const [name, body] of [
  ["an object display name", { ...ADMIN_ACCOUNT, display_name: { detail: "unreadable account" } }],
  ["an unknown beta verdict", { ...ADMIN_ACCOUNT, in_beta_allowlist: "false" }],
  ["an invalid workspace id", { ...ADMIN_ACCOUNT, workspace_id: "../unverified-workspace" }],
] as const) {
  for (const reread of [false, true]) {
    test(`${name} on ${reread ? "reread" : "first lookup"} stays unknown and recovers without losing the lookup draft`, async ({
      page,
    }) => {
      const state = await openLookup(page);
      let reads = 0;
      let recover = false;
      const requests: string[] = [];
      page.on("request", (request) => {
        requests.push(request.url());
      });
      await page.route("**/admin/accounts?email=*", (route) => {
        reads++;
        return route.fulfill({ json: recover || (reread && reads === 1) ? ADMIN_ACCOUNT : body });
      });
      const main = page.getByRole("main");
      const lookup = page.getByRole("button", { name: "查詢", exact: true });
      await lookup.click();
      if (reread) {
        await expect(main.getByRole("heading", { name: "封測者甲", exact: true })).toBeVisible();
        await expect(page.locator("#admin-grant-amount")).toBeVisible();
        await lookup.click();
      }
      await expect(main.getByRole("alert")).toContainText("暫時無法讀取帳號。");
      await expect(
        main.getByRole("heading", { level: 1, name: "帳號與點數", exact: true }),
      ).toBeVisible();
      await expect(page.getByLabel("Email", { exact: true })).toHaveValue("Member+Ops@Example.com");
      await expect(main.getByRole("heading", { name: "封測者甲", exact: true })).toHaveCount(0);
      await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
      await expect(
        main.getByText("目前不可通過；請確認封測名單或部署設定。", { exact: true }),
      ).toHaveCount(0);
      await expect(main.getByText("沒有申請", { exact: true })).toHaveCount(0);
      expect(reads).toBe(reread ? 2 : 1);
      expect(state.documents).toBe(1);
      expect(state.writes).toEqual([]);
      expect(requests.filter((url) => url.includes("unverified-workspace"))).toEqual([]);
      const report = await new AxeBuilder({ page }).analyze();
      await writeFile(
        test.info().outputPath("admin-account-response-a11y.json"),
        JSON.stringify(report),
      );
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      await main.getByRole("alert").scrollIntoViewIfNeeded();
      await page.screenshot({
        path: test.info().outputPath("admin-account-unknown-phone.png"),
        fullPage: true,
      });
      recover = true;
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(main.getByRole("heading", { name: "封測者甲", exact: true })).toBeVisible();
      await expect(main.getByRole("alert")).toHaveCount(0);
      await expect(page.getByLabel("Email", { exact: true })).toHaveValue("Member+Ops@Example.com");
      expect(reads).toBe(reread ? 3 : 2);
      expect(state.documents).toBe(1);
      expect(state.writes).toEqual([]);
    });
  }
}

test("an offline account reread hides the previous grant target until a fresh reply arrives", async ({
  page,
}) => {
  const state = await openLookup(page);
  let reads = 0;
  let release!: () => void;
  const heldRead = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/admin/accounts?email=*", async (route) => {
    reads++;
    if (reads === 2) await heldRead;
    return route.fulfill({ json: ADMIN_ACCOUNT });
  });
  try {
    await page.getByRole("button", { name: "查詢", exact: true }).click();
    await expect(page.locator("#admin-grant-amount")).toBeVisible();
    await page.locator("#admin-grant-amount").fill("25");
    await page.locator("#admin-grant-note").fill("draft for previous observation");
    await setOffline(page, true);
    await page.getByRole("button", { name: "查詢", exact: true }).click();
    await expect(page.getByRole("main").getByRole("status")).toContainText("等待連線以讀取帳號");
    await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
    await expect(page.getByLabel("Email", { exact: true })).toHaveValue("Member+Ops@Example.com");
    expect(reads).toBe(1);
    expect(state.writes).toEqual([]);
    await setOffline(page, false);
    await expect.poll(() => reads).toBe(2);
    await expect(page.getByRole("main").getByRole("status")).toContainText("載入帳號中");
    await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
    release();
    await expect(page.locator("#admin-grant-amount")).toBeVisible();
    await expect(page.locator("#admin-grant-amount")).toHaveValue("");
    await expect(page.locator("#admin-grant-note")).toHaveValue("");
    expect(state.documents).toBe(1);
    expect(state.writes).toEqual([]);
  } finally {
    release();
  }
});
