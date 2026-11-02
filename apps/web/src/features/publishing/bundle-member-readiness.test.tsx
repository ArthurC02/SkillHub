import { expect, test } from "vitest";
import { VERSION, platformResponse } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

function describedText(element: Element | null): string {
  return (element?.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ");
}

for (const state of ["paused", "fetching"]) {
  for (const focus of ["new member", "edited field"]) {
    test(`Bundle member pagination that is ${state} preserves the draft and focuses the ${focus}`, async () => {
      const page = app();
      const base = (platformResponse("/skills").body as { skills: Record<string, unknown>[] })
        .skills[0];
      const rows = Array.from({ length: 25 }, (_, index) => ({
        ...base,
        skill_id:
          index === 0
            ? base.skill_id
            : `a0000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
        name: `Bundle member ${index + 1}`,
      }));
      let finish!: () => void;
      const held = new Promise<void>((resolve) => {
        finish = resolve;
      });
      const offsets: number[] = [];
      const writes = () =>
        page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
      page.route("**/skills?*", async ({ url }) => {
        const offset = Number(url.searchParams.get("offset"));
        offsets.push(offset);
        if (offset > 0) await held;
        return {
          json: {
            skills: rows.slice(offset, offset + 24),
            limit: 24,
            total: 25,
            truncated: offset === 0,
          },
        };
      });
      await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
      const section = page.locator("#bundle-workspace");
      const form = section.locator(".bundle-form");
      const member = form.locator(`[data-bundle-version="${VERSION}"]`);
      const create = form.getByRole("button", { name: "建立", exact: true });
      const more = section.getByRole("button", { name: "載入更多小工具", exact: true });
      const nameField = form.getByRole("textbox", { name: "名稱", exact: true });
      try {
        await page.eventually(() => expect(member.value()).toBe(VERSION));
        await page.eventually(() => expect(create.isDisabled()).toBe(false));
        await nameField.fill("kept-bundle");
        await form
          .getByRole("textbox", { name: "版本（semver，例如 1.0.0）", exact: true })
          .fill("1.2.0");
        await form.getByRole("textbox", { name: "說明", exact: true }).fill("保留這段描述");
        if (state === "paused") await page.setOffline(true);
        await more.click();
        const status = section.locator("#bundle-members-read-status");
        await page.eventually(() => expect(create.isDisabled()).toBe(true));
        expect(member.isDisabled()).toBe(true);
        if (state === "paused") expect(more.isDisabled()).toBe(true);
        const expected = state === "paused" ? "等待連線" : "載入可加入 Bundle 的版本中";
        await page.eventually(() => expect(status.text()).toContain(expected));
        expect(describedText(member.element())).toContain(expected);
        expect(section.text()).not.toContain("還沒有可以選的小工具");
        expect(await page.axeViolations("#bundle-workspace")).toEqual([]);
        if (focus === "edited field") await nameField.fill("continued-bundle");
        await page.setOffline(false);
        finish();
        await page.eventually(() => expect(create.isDisabled()).toBe(false));
        await page.eventually(() => expect(member.isDisabled()).toBe(false));
        expect(member.value()).toBe(VERSION);
        expect(nameField.value()).toBe(
          focus === "edited field" ? "continued-bundle" : "kept-bundle",
        );
        expect(form.getByRole("textbox", { name: "說明", exact: true }).value()).toBe(
          "保留這段描述",
        );
        await page.eventually(() => expect(status.count()).toBe(0));
        expect(more.count()).toBe(0);
        const focused =
          focus === "edited field"
            ? nameField
            : form.locator("#bundle-member-a0000000-0000-4000-8000-000000000024");
        await page.eventually(() => expect(focused.isFocused()).toBe(true));
        expect(offsets).toEqual([0, 24]);
        expect(writes()).toEqual([]);
        expect(await page.axeViolations("#bundle-workspace")).toEqual([]);
      } finally {
        finish();
        await page.setOffline(false);
      }
    });
  }
}
