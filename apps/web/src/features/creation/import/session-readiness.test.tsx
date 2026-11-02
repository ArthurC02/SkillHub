import { expect, test } from "vitest";
import { platformResponse, SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";
import type { ImportResult } from "../../../core/api/types";

const app = renderedApp();

const draft = "https://github.com/example/private-draft";
const limits = {
  allowed_hosts: ["github.com"],
  max_zip_bytes: 10485760,
  max_unpacked_bytes: 104857600,
  max_files: 100,
  max_file_bytes: 1048576,
  max_path_depth: 8,
  max_skills_per_import: 20,
  note: "",
};
const imported = {
  shape: "skill",
  skills: [
    {
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 2,
      content_hash: "a".repeat(64),
      path: "skill",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
} satisfies ImportResult;

const SESSION_UNCONFIRMED = "登入狀態尚未確認；請先在頁首確認，再送出匯入。";

for (const failure of ["pending", "server", "network", "offline"]) {
  test(`${failure} identity verification blocks import and recovers without replaying the draft`, async () => {
    const page = app();
    page.route("**/skills/import/limits", () => ({ json: limits }));
    let checking = false;
    let release!: () => void;
    let started!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const readStarted = new Promise<void>((resolve) => {
      started = resolve;
    });
    page.route("**/me", async () => {
      if (checking && failure === "pending") {
        started();
        await gate;
      }
      if (checking && failure === "server")
        return { status: 503, json: { error: "service unavailable" } };
      if (checking && failure === "network") return "network-error";
      return { json: platformResponse("/me").body };
    });
    page.route("**/skills/import/url", () => ({ status: 201, json: imported }));
    const writes = () => page.requests.filter((request) => request.method === "POST");
    const start = page.getByRole("button", { name: "開始匯入", exact: true });
    const input = page.getByRole("textbox", { name: "URL", exact: true });
    const reconfirm = page.getByRole("button", { name: "重新確認登入狀態", exact: true });
    try {
      await page.goto("/workspace/import");
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await input.fill(draft);
      await expect(start).toBeEnabled();
      checking = true;
      if (failure === "offline") await page.setOffline(true);
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
      if (failure === "pending") await readStarted;
      await page.eventually(() => {
        if (failure === "offline")
          expect(page.getByText("等待連線以確認登入狀態…", { exact: true }).count()).toBe(1);
        else if (failure !== "pending") expect(reconfirm.count()).toBe(1);
        expect(page.locator(".auth-name").count()).toBe(0);
        expect(start.isDisabled()).toBe(true);
      });
      await input.focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(start.isDisabled()).toBe(true);
        expect(start.attribute("aria-describedby")).toBe("import-session-unavailable");
        expect(page.locator("#import-session-unavailable").text()).toBe(SESSION_UNCONFIRMED);
        expect(input.isFocused()).toBe(true);
      });
      expect(writes()).toHaveLength(0);
      await expect(input).toHaveValue(draft);
      expect(await page.axeViolations("body")).toEqual([]);
      checking = false;
      if (failure === "pending") release();
      else if (failure === "offline") await page.setOffline(false);
      else {
        await reconfirm.click();
        await expect(page.locator("main")).toBeFocused();
      }
      await page.eventually(() => {
        expect(page.getByText("正在確認登入狀態…", { exact: true }).count()).toBe(0);
        expect(page.locator(".auth-name").text()).toBe("tester");
        expect(reconfirm.count()).toBe(0);
      });
      expect(writes()).toHaveLength(0);
      await expect(input).toHaveValue(draft);
      await start.click();
      await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toHaveCount(1);
      expect(writes()).toHaveLength(1);
    } finally {
      release();
      await page.setOffline(false);
    }
  });
}
