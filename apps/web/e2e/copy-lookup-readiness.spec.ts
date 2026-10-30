import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { SKILL, SKILL_VERSIONS, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const COPY_ID = "00000000-0000-4000-8000-999999999999";
const COPY_NAME = "My existing PDF copy";

async function inspectAccessibility(page: Page, info: TestInfo, filename: string) {
  const result = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(info.outputPath(filename), JSON.stringify(result, null, 2));
  expect(result.violations).toEqual([]);
}

for (const width of [375, 1280]) {
  for (const mode of ["read failure", "incomplete listing"] as const) {
    test(`copy lookup recovers from ${mode} without creating another copy at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      const writes: string[] = [];
      const errors: string[] = [];
      const offsets: number[] = [];
      page.on("request", (request) => {
        if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
      });
      page.on("pageerror", (error) => errors.push(error.message));
      await page.route(`**/skills/${SKILL}/versions`, (route) =>
        route.fulfill({ json: { ...SKILL_VERSIONS, versions: [] } }),
      );
      await page.route(
        (url) => url.pathname === "/skills",
        (route) => {
          const offset = Number(new URL(route.request().url()).searchParams.get("offset") ?? 0);
          offsets.push(offset);
          const fixture = platformResponse(route.request().url()).body as {
            skills: Record<string, unknown>[];
          };
          if (mode === "read failure" && offsets.length === 1)
            return route.fulfill({ status: 503, json: { error: "workspace listing unavailable" } });
          if (mode === "incomplete listing" && offset === 0) {
            const skills = Array.from({ length: 100 }, (_, index) => ({
              ...fixture.skills[0],
              skill_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
              name: `Other owned skill ${index + 1}`,
            }));
            return route.fulfill({ json: { skills, limit: 100, total: 101, truncated: true } });
          }
          return route.fulfill({
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
          });
        },
      );
      await page.goto(`/skills/${SKILL}`);
      await expect(
        page.getByRole("heading", { level: 1, name: "PDF Summariser", exact: true }),
      ).toBeVisible();
      await expect.poll(() => offsets.length).toBe(1);
      try {
        if (mode === "read failure")
          await expect(page.getByRole("alert").filter({ hasText: "複本" })).toBeVisible();
        else await expect(page.getByRole("status").filter({ hasText: "尚未讀完" })).toBeVisible();
        await expect(page.getByRole("main")).not.toContainText("要先複製一份");
        await inspectAccessibility(page, info, "copy-initial-accessibility.json");
        await page.screenshot({ path: info.outputPath("copy-initial.png"), fullPage: true });
        await page
          .getByRole("button", {
            name: mode === "read failure" ? "再試一次" : "查找更多複本",
            exact: true,
          })
          .click();
        const copies = page.getByRole("link", { name: COPY_NAME, exact: true });
        await expect(copies).toHaveCount(3);
        for (const copy of await copies.all())
          await expect(copy).toHaveAttribute("href", `/skills/${COPY_ID}`);
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
        await expect(page.getByRole("main")).not.toContainText("要先複製一份");
        await expect(page.getByRole("alert")).toHaveCount(0);
        expect(offsets).toEqual(mode === "read failure" ? [0, 0] : [0, 100]);
        expect(writes).toEqual([]);
        expect(errors).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
          false,
        );
        await inspectAccessibility(page, info, "copy-accessibility.json");
      } finally {
        await writeFile(
          info.outputPath("copy-decision.json"),
          JSON.stringify(
            {
              mode,
              width,
              offsets,
              writes,
              errors,
              text: await page.getByRole("main").textContent(),
            },
            null,
            2,
          ),
        );
        await page.screenshot({ path: info.outputPath("copy-decision.png"), fullPage: true });
      }
    });
  }
}
