import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SKILL, SKILL_VERSIONS, platformResponse } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const COPY_ID = "00000000-0000-4000-8000-999999999999";
const COPY_NAME = "My existing PDF copy";

for (const mode of ["read failure", "incomplete listing"] as const) {
  test(`copy lookup recovers from ${mode} without creating another copy`, async () => {
    const page = app();
    const offsets: number[] = [];
    const errors: string[] = [];
    const record = (event: ErrorEvent) => errors.push(event.message);
    window.addEventListener("error", record);
    page.route(`**/skills/${SKILL}/versions`, () => ({
      json: { ...SKILL_VERSIONS, versions: [] },
    }));
    page.route(
      (url) => url.pathname === "/skills",
      ({ url }) => {
        offsets.push(Number(url.searchParams.get("offset") ?? 0));
        const offset = offsets[offsets.length - 1];
        const fixture = platformResponse(url.pathname + url.search).body as {
          skills: Record<string, unknown>[];
        };
        if (mode === "read failure" && offsets.length === 1)
          return { status: 503, json: { error: "workspace listing unavailable" } };
        if (mode === "incomplete listing" && offset === 0) {
          const skills = Array.from({ length: 100 }, (_, index) => ({
            ...fixture.skills[0],
            skill_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
            name: `Other owned skill ${index + 1}`,
          }));
          return { json: { skills, limit: 100, total: 101, truncated: true } };
        }
        return {
          json: {
            skills: [
              {
                ...fixture.skills[0],
                skill_id: COPY_ID,
                name: COPY_NAME,
                forked_from_skill_id: SKILL,
              },
            ],
            limit: 100,
            total: mode === "incomplete listing" ? 101 : 1,
            truncated: false,
          },
        };
      },
    );
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator("h1")).toHaveText("PDF Summariser");
      await page.eventually(() => expect(offsets).toHaveLength(1));
      const main = page.locator("main");
      if (mode === "read failure")
        await expect(page.getByRole("alert").filter({ hasText: "複本" })).toBeVisible();
      else await expect(page.getByRole("status").filter({ hasText: "尚未讀完" })).toBeVisible();
      await expect(main).not.toContainText("要先複製一份");
      expect(await page.axeViolations("main")).toEqual([]);
      await page
        .getByRole("button", {
          name: mode === "read failure" ? "再試一次" : "查找更多複本",
          exact: true,
        })
        .click();
      const copies = page.getByRole("link", { name: COPY_NAME, exact: true });
      await expect(copies).toHaveCount(3);
      for (const index of [0, 1, 2])
        await expect(copies.nth(index)).toHaveAttribute("href", `/skills/${COPY_ID}`);
      if (mode === "read failure")
        await expect(
          page.getByRole("heading", { name: "複製一份到你的工作區", exact: true }),
        ).toBeFocused();
      else
        await expect(
          page
            .locator("section")
            .filter({
              has: page.getByRole("heading", { name: "複製一份到你的工作區", exact: true }),
            })
            .getByRole("link", { name: COPY_NAME, exact: true }),
        ).toBeFocused();
      await expect(main).not.toContainText("要先複製一份");
      await expect(page.getByRole("alert")).toHaveCount(0);
      expect(offsets).toEqual(mode === "read failure" ? [0, 0] : [0, 100]);
      expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
      expect(errors).toEqual([]);
      expect(await page.axeViolations("main")).toEqual([]);
    } finally {
      window.removeEventListener("error", record);
    }
  });
}
