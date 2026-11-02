import { expect, test } from "vitest";
import { ADMIN_ACCOUNT, ME } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

async function openLookup(page: App, draft = "Member+Ops@Example.com") {
  page.route("**/me", () => ({ json: { ...ME, operator: true } }));
  await page.goto("/admin/accounts");
  await expect(page.getByLabel("Email", { exact: true })).toHaveCount(1);
  if (draft) await page.getByLabel("Email", { exact: true }).fill(draft);
  const writes = () => page.requests.filter((request) => request.method === "POST");
  return { writes };
}

function browserFill(input: HTMLInputElement, value: string) {
  const submitted: string[] = [];
  input.form!.addEventListener("submit", () => submitted.push(input.value));
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
  return { submitted, valid: input.checkValidity() };
}

for (const previous of ["", "previous@example.com"]) {
  for (const refreshProfile of [false, true]) {
    test(`a browser-filled email replaces ${previous ? "a different" : "an empty"} draft as the submitted lookup intent${refreshProfile ? " after a profile refresh" : ""}`, async () => {
      const page = app();
      const state = await openLookup(page, previous);
      const input = page.getByLabel("Email", { exact: true });
      const email = "member@example.com";
      const reads: string[] = [];
      page.route("**/admin/accounts?email=*", ({ url }) => {
        const requested = url.searchParams.get("email")!;
        reads.push(requested);
        return {
          json:
            requested === email ? ADMIN_ACCOUNT : { ...ADMIN_ACCOUNT, display_name: "先前帳號" },
        };
      });
      const submissions = browserFill(input.element() as HTMLInputElement, email);
      expect(submissions.valid).toBe(true);
      if (refreshProfile) {
        page.route("**/me", () => ({
          json: { ...ME, operator: true, display_name: "updated tester" },
        }));
        document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
        await expect(page.locator(".auth-name")).toHaveText("updated tester");
      }
      await expect(input).toHaveValue(email);
      await page.getByRole("button", { name: "查詢", exact: true }).click();
      await page.eventually(() => expect(submissions.submitted).toEqual([email]));
      await page.eventually(() => expect(reads).toEqual([email]));
      await expect(page.getByRole("heading", { name: "封測者甲", exact: true })).toHaveCount(1);
      await expect(input).toHaveValue(email);
      expect(page.pathname()).toBe("/admin/accounts");
      expect(page.search()).toBe("");
      expect(state.writes()).toEqual([]);
    });
  }
}

for (const [name, body] of [
  ["an object display name", { ...ADMIN_ACCOUNT, display_name: { detail: "unreadable account" } }],
  ["an unknown beta verdict", { ...ADMIN_ACCOUNT, in_beta_allowlist: "false" }],
  ["an invalid workspace id", { ...ADMIN_ACCOUNT, workspace_id: "../unverified-workspace" }],
] as const) {
  for (const reread of [false, true]) {
    test(`${name} on ${reread ? "reread" : "first lookup"} stays unknown and recovers without losing the lookup draft`, async () => {
      const page = app();
      const state = await openLookup(page);
      let reads = 0;
      let recover = false;
      page.route("**/admin/accounts?email=*", () => {
        reads++;
        return { json: recover || (reread && reads === 1) ? ADMIN_ACCOUNT : body };
      });
      const lookupHeading = () => page.getByRole("heading", { name: "封測者甲", exact: true });
      const submitLookup = () => page.getByLabel("Email", { exact: true }).press("Enter");
      await submitLookup();
      if (reread) {
        await expect(lookupHeading()).toHaveCount(1);
        await expect(page.locator("#admin-grant-amount")).toHaveCount(1);
        await submitLookup();
      }
      await expect(page.locator("main [role=alert]")).toContainText("暫時無法讀取帳號。");
      await expect(page.getByRole("heading", { name: "帳號與點數", exact: true })).toHaveCount(1);
      await expect(page.getByLabel("Email", { exact: true })).toHaveValue("Member+Ops@Example.com");
      await expect(lookupHeading()).toHaveCount(0);
      await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
      await expect(
        page.getByText("目前不可通過；請確認封測名單或部署設定。", { exact: true }),
      ).toHaveCount(0);
      await expect(page.getByText("沒有申請", { exact: true })).toHaveCount(0);
      expect(reads).toBe(reread ? 2 : 1);
      expect(state.writes()).toEqual([]);
      expect(
        page.requests.filter((request) => request.url.href.includes("unverified-workspace")),
      ).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
      recover = true;
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(lookupHeading()).toHaveCount(1);
      await expect(page.locator("main [role=alert]")).toHaveCount(0);
      await expect(page.getByLabel("Email", { exact: true })).toHaveValue("Member+Ops@Example.com");
      expect(reads).toBe(reread ? 3 : 2);
      expect(state.writes()).toEqual([]);
    });
  }
}

test("an offline account reread hides the previous grant target until a fresh reply arrives", async () => {
  const page = app();
  const state = await openLookup(page);
  let reads = 0;
  let release!: () => void;
  const heldRead = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route("**/admin/accounts?email=*", async () => {
    reads++;
    if (reads === 2) await heldRead;
    return { json: ADMIN_ACCOUNT };
  });
  try {
    await page.getByLabel("Email", { exact: true }).press("Enter");
    await expect(page.locator("#admin-grant-amount")).toHaveCount(1);
    await page.locator("#admin-grant-amount").fill("25");
    await page.locator("#admin-grant-note").fill("draft for previous observation");
    await page.setOffline(true);
    await page.getByRole("button", { name: "查詢", exact: true }).click();
    await expect(page.locator("main [role=status]")).toContainText("等待連線以讀取帳號");
    await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
    await expect(page.getByLabel("Email", { exact: true })).toHaveValue("Member+Ops@Example.com");
    expect(reads).toBe(1);
    expect(state.writes()).toEqual([]);
    await page.setOffline(false);
    await page.eventually(() => expect(reads).toBe(2));
    await expect(page.locator("main [role=status]")).toContainText("載入帳號中");
    await expect(page.locator("#admin-grant-amount")).toHaveCount(0);
    release();
    await expect(page.locator("#admin-grant-amount")).toHaveCount(1);
    await expect(page.locator("#admin-grant-amount")).toHaveValue("");
    await expect(page.locator("#admin-grant-note")).toHaveValue("");
    expect(state.writes()).toEqual([]);
  } finally {
    release();
  }
});
