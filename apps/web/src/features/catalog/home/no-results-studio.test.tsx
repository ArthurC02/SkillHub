import { expect, test } from "vitest";
import { renderedApp, type App } from "../../../testing/app";
import { CreationActionMessageMaxLength } from "@skillhub/api-client-ts/limits";
import { ME } from "../../../testing/fixtures/platform";

const app = renderedApp();

const QUERY = "沒有人做過的事";
const STUDIO_LINK = "到 Studio 用這段描述開始創作";

function noResultsWith(page: App, features: Record<string, boolean>) {
  page.route("**/me", () => ({ json: { ...ME, features } }));
  page.route("**/api/skills/search?*", () => ({
    json: {
      query: QUERY,
      results: [],
      total: 0,
      limit: 20,
      truncated: false,
      degraded: false,
      partial_index: false,
      filtered_out: false,
      no_results: true,
    },
  }));
}

test("with Studio on, an empty search hands its words to Studio instead of the one-shot form", async () => {
  const page = app();
  noResultsWith(page, { generate_skill: true, creation_skill: true });
  await page.goto(`/?q=${encodeURIComponent(QUERY)}`);
  const link = page.getByRole("link", { name: STUDIO_LINK });
  await page.eventually(() => expect(link.count()).toBe(1));
  const target = new URL(link.attribute("href") ?? "", "http://x");
  expect(target.pathname).toBe("/workspace/creations");
  expect(target.searchParams.get("task")).toBe(QUERY);
  expect(page.locator("#generate-task").count()).toBe(0);
});

test("⛔ with generation off, an empty search offers neither Studio nor the one-shot form, even with Studio on", async () => {
  const page = app();
  noResultsWith(page, { creation_skill: true });
  await page.goto(`/?q=${encodeURIComponent(QUERY)}`);
  await page.eventually(() => expect(page.getByText("沒有夠接近的小工具。").count()).toBe(1));
  expect(page.getByRole("link", { name: STUDIO_LINK }).count()).toBe(0);
  expect(page.locator("#generate-task").count()).toBe(0);
});

test("Studio opened from an empty search starts with that description in its message box, unsent", async () => {
  const page = app();
  noResultsWith(page, { generate_skill: true, creation_skill: true });
  await page.goto(`/workspace/creations?task=${encodeURIComponent(QUERY)}`);
  const box = page.getByLabel("想完成的任務");
  await page.eventually(() => expect(box.value()).toBe(QUERY));
  expect(page.requests.filter((r) => r.method === "POST")).toHaveLength(0);
});

test.each([
  ["blank", " ", ""],
  [
    "at the message limit",
    "字".repeat(CreationActionMessageMaxLength),
    "字".repeat(CreationActionMessageMaxLength),
  ],
  ["one past the message limit", "字".repeat(CreationActionMessageMaxLength + 1), ""],
])(
  "a %s task in the address prefills Studio only when one message could carry it",
  async (_name, task, want) => {
    const page = app();
    noResultsWith(page, { generate_skill: true, creation_skill: true });
    await page.goto(`/workspace/creations?task=${encodeURIComponent(task)}`);
    const box = page.getByLabel("想完成的任務");
    await page.eventually(() => expect(box.count()).toBe(1));
    expect(box.value()).toBe(want);
  },
);
