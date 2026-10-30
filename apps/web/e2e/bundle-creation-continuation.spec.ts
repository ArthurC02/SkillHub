import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { test } from "./socket-diagnostics";
import { SKILL, VERSION, platformResponse } from "../src/testing/fixtures/platform";
import type { BundleVersion } from "../src/features/publishing/publishing.service";
import { stubPlatform } from "./stub";

for (const width of [375, 1280]) {
  for (const leavesForm of [false, true]) {
    test(`Bundle creation ${leavesForm ? "preserves header focus" : "reveals each created version"} at ${width}px`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      const name = "continuation-bundle";
      const description = "這一份釘選測試過的 PDF 版本。";
      const created: BundleVersion[] = [];
      const requests: unknown[] = [];
      let reads = 0;
      let release = () => {};
      let heldRead = Promise.resolve();
      await page.route("**/skills?*", (route) => {
        const first = (platformResponse("/skills").body as { skills: object[] }).skills[0];
        return route.fulfill({
          json: {
            skills: [{ ...first, skill_id: SKILL, name: "PDF Summariser" }],
            total: 1,
            limit: 24,
            truncated: false,
          },
        });
      });
      await page.route("**/me/bundles", async (route, request) => {
        if (request.method() === "POST") {
          const body = request.postDataJSON();
          requests.push(body);
          const result: BundleVersion = {
            bundle: body.name,
            version: body.version,
            description: body.description,
            content_hash: `sha256:bundle-${body.version}`,
            created_at: "2026-10-20T00:00:00Z",
            members: [
              {
                skill_id: SKILL,
                version_id: VERSION,
                name: "PDF Summariser",
                version_number: 2,
                content_hash: "sha256:bbbb",
              },
            ],
          };
          created.unshift(result);
          return route.fulfill({ status: 201, json: result });
        }
        reads++;
        await heldRead;
        return route.fulfill({ json: { bundles: [...created] } });
      });
      await page.route("**/me/bundles/overview", (route) =>
        route.fulfill({
          json: { bundles: created.length ? [{ bundle: name, latest_version: created[0] }] : [] },
        }),
      );
      await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
      const form = page.getByRole("region", { name: "建立 Bundle", exact: true });
      const member = form.getByRole("combobox", { name: "PDF Summariser", exact: true });
      await expect(member).toHaveValue(VERSION);
      const search = page.getByRole("banner").getByRole("searchbox");
      try {
        for (const [attempt, version] of ["1.0.0", "1.1.0"].entries()) {
          heldRead = new Promise<void>((resolve) => {
            release = resolve;
          });
          await form.getByRole("textbox", { name: "名稱", exact: true }).fill(name);
          await form
            .getByRole("textbox", { name: "版本（semver，例如 1.0.0）", exact: true })
            .fill(version);
          await form.getByRole("textbox", { name: "說明", exact: true }).fill(description);
          await member.selectOption(VERSION);
          await form.getByRole("button", { name: "建立", exact: true }).click();
          await expect.poll(() => requests.length).toBe(attempt + 1);
          await expect.poll(() => reads).toBe(attempt + 2);
          await expect(form.getByRole("status").filter({ hasText: "已建立" })).toHaveText(
            `已建立 ${name} v${version}。`,
          );
          if (leavesForm) await search.fill("保留目前搜尋草稿");
          const row = page
            .getByRole("article", { name, exact: true })
            .locator(".bundle-version-item")
            .filter({ has: page.getByText(`v${version}`, { exact: true }) });
          await expect(row).toHaveCount(0);
          release();
          await expect(row).toHaveCount(1);
          if (leavesForm) {
            await expect(search).toBeFocused();
            await expect(search).toHaveValue("保留目前搜尋草稿");
          } else {
            await expect(row).toBeFocused();
            const bounds = await row.boundingBox();
            const header = await page.getByRole("banner").boundingBox();
            expect(bounds).not.toBeNull();
            expect(header).not.toBeNull();
            expect(bounds!.y).toBeGreaterThanOrEqual(header!.y + header!.height);
            expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(900);
          }
        }
        expect(requests).toEqual(
          ["1.0.0", "1.1.0"].map((version) => ({
            name,
            version,
            description,
            member_version_ids: [VERSION],
          })),
        );
        await captureBundleCreation(page);
      } finally {
        release();
      }
    });
  }
}

async function captureBundleCreation(page: Page) {
  const report = await new AxeBuilder({ page }).include("#bundle-workspace").analyze();
  await writeFile(test.info().outputPath("bundle-creation-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("bundle-creation.png") });
}
