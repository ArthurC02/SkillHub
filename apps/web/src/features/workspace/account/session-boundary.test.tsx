import { expect, test } from "vitest";
import { platformResponse } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

for (const response of ["lost", "server-error"]) {
  test(`an uncertain logout with a ${response} response reconciles a unavailable session without replaying the write`, async () => {
    const page = app();
    let attempted = false;
    let writes = 0;
    let reads = 0;
    page.route("**/me", () => {
      if (!attempted) return { json: platformResponse("/me").body };
      reads += 1;
      return { status: 503, json: { error: "service unavailable" } };
    });
    page.route("**/auth/logout", () => {
      attempted = true;
      writes += 1;
      return response === "lost"
        ? "network-error"
        : { status: 500, json: { error: "internal error" } };
    });
    await page.goto("/workspace/account");
    const main = page.locator("main");
    await expect(main.getByText("tester@example.com", { exact: true })).toHaveCount(1);
    await page.getByRole("button", { name: "登出", exact: true }).click();
    await page.eventually(() => {
      expect(main.locator(".account-profile").count()).toBe(0);
      expect(main.locator(".account-danger-zone").count()).toBe(0);
      expect(page.locator("[data-auth-controls]").getByRole("alert").text()).toBe(
        "未能確認登出結果，請確認目前登入狀態後再試一次。",
      );
      expect(main.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(1);
    });
    expect(reads).toBeGreaterThan(0);
    expect(writes).toBe(1);
    expect(page.pathname()).toBe("/workspace/account");
  });
}

for (const action of ["logout", "login"]) {
  test(`a refused ${action} retains the current task and reports the failure without reloading`, async () => {
    window.__SKILLHUB_DEV_LOGIN__ = true;
    try {
      const page = app();
      if (action === "login")
        page.route("**/me", () => ({ status: 401, json: { error: "unauthorized" } }));
      page.route(action === "logout" ? "**/auth/logout" : "**/auth/dev/login", () => ({
        status: 500,
        json: { error: "internal error" },
      }));
      await page.goto("/workspace/account");
      const controls = page.locator("[data-auth-controls]");
      if (action === "logout") {
        await expect(controls.getByRole("button", { name: "登出", exact: true })).toHaveCount(1);
        await controls.getByRole("button", { name: "登出", exact: true }).click();
      } else {
        await expect(controls.getByRole("textbox")).toHaveCount(1);
        await controls.getByRole("textbox").fill("second");
        await controls.getByRole("button", { name: "登入", exact: true }).click();
      }
      await page.eventually(() => {
        expect(controls.getByRole("alert").text()).toContain(
          action === "logout" ? "未能確認登出結果" : "登入沒有成功",
        );
        if (action === "login") expect(controls.getByRole("textbox").value()).toBe("second");
        else
          expect(
            page.locator("main").getByText("tester@example.com", { exact: true }).count(),
          ).toBe(1);
      });
      expect(page.pathname()).toBe("/workspace/account");
    } finally {
      delete window.__SKILLHUB_DEV_LOGIN__;
    }
  });
}
