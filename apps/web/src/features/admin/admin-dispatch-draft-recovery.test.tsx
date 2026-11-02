import { expect, test } from "vitest";
import { ADMIN_DISPATCH } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

const DISPATCH = {
  ...ADMIN_DISPATCH,
  halts: [
    ...ADMIN_DISPATCH.halts,
    {
      ...ADMIN_DISPATCH.halts[0],
      target: "node-2",
      source: "orphan_threshold",
      reason: "orphan capacity reached",
      automatic_recovery: true,
      clear_rounds: 1,
    },
  ],
};

const NOTE = "證據已保存，事故處置紀錄已核對";

for (const target of ["pool", "node-2"]) {
  for (const outcome of ["503", "malformed"]) {
    test(`dispatch release draft survives ${outcome} and requires a deliberate confirmation for ${target}`, async () => {
      const page = app();
      let readMode = "valid";
      let released = false;
      let held: Promise<void> | undefined;
      let finishRead!: () => void;
      const bodies: unknown[] = [];
      const writes = () =>
        page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
      page.route("**/admin/dispatch", async () => {
        if (held) await held;
        if (readMode === "503") return { status: 503, json: { error: "service unavailable" } };
        if (readMode === "malformed") return { json: { dispatching: "false", halts: [] } };
        return {
          json: released
            ? {
                dispatching: target === "pool",
                halts: DISPATCH.halts.filter((halt) => halt.target !== target),
              }
            : DISPATCH,
        };
      });
      page.route("**/admin/dispatch/halt", ({ method, body }) => {
        if (method !== "DELETE") return { status: 500, json: { error: "unexpected write" } };
        bodies.push(body);
        released = true;
        return { status: 204 };
      });
      await page.goto("/admin/dispatch");
      const select = page.locator("#admin-halt-lift-target");
      const note = page.locator("#admin-halt-lift-note");
      const section = () =>
        document.querySelectorAll('section[aria-labelledby="admin-dispatch-lift"]');
      const refresh = page.getByRole("button", { name: "重新整理派送狀態", exact: true });
      const confirm = page.getByRole("button", { name: "確認恢復派送", exact: true });
      await page.eventually(() => expect(select.count()).toBe(1));
      await select.select(target);
      await note.fill(NOTE);
      await page.getByRole("button", { name: "恢復派送", exact: true }).click();
      await page.eventually(() => expect(confirm.count()).toBe(1));
      expect(confirm.isDisabled()).toBe(false);
      readMode = outcome;
      held = new Promise<void>((resolve) => {
        finishRead = resolve;
      });
      await refresh.click();
      try {
        await page.eventually(() => expect(select.isDisabled()).toBe(true));
        await page.eventually(() => expect(confirm.isDisabled()).toBe(true));
        const describedBy = select.attribute("aria-describedby") ?? "";
        expect(document.getElementById(describedBy)?.textContent).toBe(
          "正在重新讀取派送狀態，完成後才能恢復派送。",
        );
        expect(await page.axeViolations("main")).toEqual([]);
        expect(writes()).toEqual([]);
      } finally {
        finishRead();
        held = undefined;
      }
      await page.eventually(() =>
        expect(page.getByRole("alert").text()).toContain("暫時無法讀取派送狀態"),
      );
      expect(section()).toHaveLength(1);
      expect(page.locator('section[aria-labelledby="admin-dispatch-lift"]').count()).toBe(0);
      expect(
        page.getByText("原本的欄位先保留，狀態恢復後再核對。", { exact: false }).count(),
      ).toBeGreaterThan(0);
      expect(select.count()).toBe(0);
      expect((document.getElementById("admin-halt-lift-target") as HTMLSelectElement).value).toBe(
        target,
      );
      expect((document.getElementById("admin-halt-lift-note") as HTMLTextAreaElement).value).toBe(
        NOTE,
      );
      expect(confirm.count()).toBe(0);
      readMode = "valid";
      await refresh.click();
      await page.eventually(() => expect(select.count()).toBe(1));
      expect(select.value()).toBe(target);
      expect(note.value()).toBe(NOTE);
      await page.eventually(() => expect(confirm.isDisabled()).toBe(false));
      expect(writes()).toEqual([]);
      await confirm.click();
      const result = page.locator("#admin-dispatch-lift-result");
      await page.eventually(() => expect(result.text()).toContain("已不在生效中的煞車清單"));
      expect(result.isFocused()).toBe(true);
      expect(select.value()).toBe("");
      expect(select.locator(`option[value="${target}"]`).count()).toBe(0);
      expect(
        select.locator(`option[value="${target === "pool" ? "node-2" : "pool"}"]`).count(),
      ).toBe(1);
      expect(note.value()).toBe("");
      expect(page.getByRole("button", { name: "恢復派送", exact: true }).isDisabled()).toBe(true);
      expect(bodies).toEqual([
        target === "pool" ? { note: NOTE } : { note: NOTE, provider: "node-2" },
      ]);
      expect(writes()).toHaveLength(1);
    });
  }
}
