import { expect, test } from "vitest";
import { SKILL, VERSION, platformResponse } from "../../testing/fixtures/platform";
import type { BundleVersion } from "./publishing.service";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

for (const leavesForm of [false, true]) {
  test(`Bundle creation ${leavesForm ? "preserves header focus" : "reveals each created version"}`, async () => {
    const page = app();
    const name = "continuation-bundle";
    const description = "這一份釘選測試過的 PDF 版本。";
    const created: BundleVersion[] = [];
    const requests: unknown[] = [];
    let reads = 0;
    let release = () => {};
    let heldRead = Promise.resolve();
    page.route("**/skills?*", () => {
      const first = (platformResponse("/skills").body as { skills: object[] }).skills[0];
      return {
        json: {
          skills: [{ ...first, skill_id: SKILL, name: "PDF Summariser" }],
          total: 1,
          limit: 24,
          truncated: false,
        },
      };
    });
    page.route("**/me/bundles", async ({ method, body }) => {
      if (method === "POST") {
        const input = body as { name: string; version: string; description: string };
        requests.push(input);
        const result: BundleVersion = {
          bundle: input.name,
          version: input.version,
          description: input.description,
          content_hash: `sha256:bundle-${input.version}`,
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
        return { status: 201, json: result };
      }
      reads++;
      await heldRead;
      return { json: { bundles: [...created] } };
    });
    page.route("**/me/bundles/overview", () => ({
      json: { bundles: created.length ? [{ bundle: name, latest_version: created[0] }] : [] },
    }));
    await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
    const form = page.getByRole("region", { name: "建立 Bundle", exact: true });
    const member = form.getByRole("combobox", { name: "PDF Summariser", exact: true });
    await page.eventually(() => expect(member.value()).toBe(VERSION));
    const search = page.locator("header input[type=search]");
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
        await member.select(VERSION);
        await form.getByRole("button", { name: "建立", exact: true }).click();
        await page.eventually(() => expect(requests.length).toBe(attempt + 1));
        await page.eventually(() => expect(reads).toBe(attempt + 2));
        await page.eventually(() =>
          expect(form.getByRole("status").filter({ hasText: "已建立" }).text()).toBe(
            `已建立 ${name} v${version}。`,
          ),
        );
        if (leavesForm) await search.fill("保留目前搜尋草稿");
        const row = page
          .locator("article")
          .filter({ hasText: name })
          .locator(".bundle-version-item")
          .filter({ hasText: `v${version}` });
        expect(row.count()).toBe(0);
        release();
        await page.eventually(() => expect(row.count()).toBe(1));
        if (leavesForm) {
          await page.eventually(() => expect(search.isFocused()).toBe(true));
          expect(search.value()).toBe("保留目前搜尋草稿");
        } else {
          await page.eventually(() => expect(row.isFocused()).toBe(true));
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
      expect(await page.axeViolations("#bundle-workspace")).toEqual([]);
    } finally {
      release();
    }
  });
}
