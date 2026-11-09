import { expect, test } from "vitest";
import { FILES, SKILL } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

test.each([
  [
    "a SKILL.md whose size the tree reports",
    [{ path: "SKILL.md", size: 2_000_000, is_script: false }],
    "共 2,000,000 位元組，這裡只顯示前 1,048,576 位元組（約 1 MB），那是這個端點的單次上限。",
  ],
  ["a SKILL.md the tree does not list", [], "內容過長，以下只顯示前 1,048,576 位元組（約 1 MB）。"],
] as const)("a truncated preview of %s states the exact bytes", async (_name, tree, want) => {
  const page = app();
  page.route("**/files", () => ({ json: { ...FILES, skill_md_truncated: true, tree } }));
  await page.goto(`/skills/${SKILL}/files`);
  await page.eventually(() =>
    expect(page.locator("main .skill-files-preview .notice").text()).toBe(want),
  );
});

test("each file in the tree states its size in exact bytes", async () => {
  const page = app();
  page.route("**/files", () => ({
    json: { ...FILES, tree: [{ path: "scripts/run.py", size: 12_345, is_script: true }] },
  }));
  await page.goto(`/skills/${SKILL}/files`);
  await page.eventually(() =>
    expect(page.locator("main .file-tree .file-size").text()).toBe("12,345 位元組"),
  );
});
