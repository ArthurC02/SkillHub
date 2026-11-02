import { expect, test } from "vitest";
import { SKILL, VERSION, platformResponse } from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

const own = platformResponse("/skills").body as { skills: Array<Record<string, unknown>> };
const firstPage = Array.from({ length: 24 }, (_, index) => ({
  ...own.skills[0],
  skill_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
  name: `Earlier asset ${index + 1}`,
}));
const target = { ...own.skills[0], name: "Target asset", summary: "The requested asset" };
const LOAD_MORE = "載入更多小工具";

function pagedAssets(page: App) {
  const offsets: number[] = [];
  page.route("**/skills?*", ({ url }) => {
    const offset = Number(url.searchParams.get("offset"));
    offsets.push(offset);
    return {
      json: {
        skills: offset === 0 ? firstPage : [target],
        total: 25,
        limit: 24,
        truncated: offset === 0,
      },
    };
  });
  page.route("**/skills", () => {
    offsets.push(0);
    return { json: { skills: firstPage, total: 25, limit: 24, truncated: true } };
  });
  return offsets;
}

test("test-case asset paging avoids an ownership conclusion and retains the draft before finding the requested asset", async () => {
  const page = app();
  const offsets = pagedAssets(page);
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await page.eventually(() => expect(page.locator("#tc-skill option").count()).toBe(25));
  expect(page.getByText("這個小工具不在你的工作區。", { exact: false }).count()).toBe(0);
  const more = page.getByRole("button", { name: LOAD_MORE, exact: true });
  expect(more.count()).toBe(1);
  await page.getByLabel("名稱", { exact: true }).fill("尚未送出的測試題");
  await page.getByLabel("User Prompt", { exact: true }).fill("保留任務提示");
  await more.click();
  await page.eventually(() => expect(page.locator("#tc-skill").value()).toBe(SKILL));
  expect(page.getByLabel("名稱", { exact: true }).value()).toBe("尚未送出的測試題");
  expect(page.getByLabel("User Prompt", { exact: true }).value()).toBe("保留任務提示");
  expect(offsets).toEqual([0, 24]);
  await page.eventually(() => expect(more.count()).toBe(0));
  expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
  expect(await page.axeViolations("main")).toEqual([]);
});

test("Bundle asset paging reaches the requested version without replacing typed identity fields", async () => {
  const page = app();
  const offsets = pagedAssets(page);
  const details = platformResponse(`/api/skills/${SKILL}`).body as object;
  const versionList = platformResponse(`/skills/${SKILL}/versions`).body as {
    versions: Array<Record<string, unknown>>;
  };
  page.route("**/api/skills/*?view=embedded", ({ url }) => {
    const id = url.pathname.split("/").at(-1);
    return {
      json: { ...details, skill_id: id, name: id === SKILL ? target.name : "Earlier asset" },
    };
  });
  page.route("**/skills/*/versions", ({ url }) => {
    const id = url.pathname.split("/").at(-2);
    return {
      json: {
        versions:
          id === SKILL
            ? versionList.versions
            : [
                {
                  ...versionList.versions[0],
                  version_id: `20000000-0000-4000-8000-${id?.slice(-12)}`,
                },
              ],
      },
    };
  });
  await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
  const section = page.locator("#bundle-workspace");
  const form = section.locator(".bundle-form");
  await page.eventually(() => expect(form.locator("select").count()).toBe(24));
  expect(form.getByText("無法確認要接續的 Version", { exact: false }).count()).toBe(0);
  await form.getByRole("textbox", { name: "名稱", exact: true }).fill("draft-bundle");
  await form
    .getByRole("textbox", { name: "版本（semver，例如 1.0.0）", exact: true })
    .fill("1.2.0");
  await section.getByRole("button", { name: LOAD_MORE, exact: true }).click();
  await page.eventually(() =>
    expect(form.locator(`[data-bundle-version="${VERSION}"]`).value()).toBe(VERSION),
  );
  expect(form.getByRole("textbox", { name: "名稱", exact: true }).value()).toBe("draft-bundle");
  expect(
    form.getByRole("textbox", { name: "版本（semver，例如 1.0.0）", exact: true }).value(),
  ).toBe("1.2.0");
  expect(offsets).toEqual([0, 24]);
  await page.eventually(() =>
    expect(section.getByRole("button", { name: LOAD_MORE, exact: true }).count()).toBe(0),
  );
});

test("a failed asset page preserves loaded choices and the draft until that page is recovered", async () => {
  const page = app();
  const offsets = pagedAssets(page);
  let failed = false;
  page.route("**/skills?*", ({ url }) => {
    const offset = Number(url.searchParams.get("offset"));
    if (offset === 24 && !failed) {
      failed = true;
      offsets.push(offset);
      return { status: 503, json: { code: "unavailable", message: "Unavailable" } };
    }
    return undefined;
  });
  await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
  await page.eventually(() => expect(page.locator("#tc-skill option").count()).toBe(25));
  await page.getByLabel("名稱", { exact: true }).fill("保留草稿");
  await page.getByLabel("User Prompt", { exact: true }).fill("保留提示");
  await page.getByRole("button", { name: LOAD_MORE, exact: true }).click();
  await page.eventually(() =>
    expect(
      page.getByText("目前無法確認可用的小工具，請先重新讀取清單。", { exact: true }).count(),
    ).toBeGreaterThan(0),
  );
  expect(page.locator("#tc-skill option").count()).toBe(25);
  expect(page.getByText("這個小工具不在你的工作區。", { exact: false }).count()).toBe(0);
  await page.getByRole("button", { name: "再試一次", exact: true }).click();
  await page.eventually(() => expect(page.locator("#tc-skill").value()).toBe(SKILL));
  expect(page.getByLabel("名稱", { exact: true }).value()).toBe("保留草稿");
  expect(page.getByLabel("User Prompt", { exact: true }).value()).toBe("保留提示");
  expect(offsets).toEqual([0, 24, 24]);
  await page.eventually(() =>
    expect(page.locator("#create-test-case-heading").isFocused()).toBe(true),
  );
});

test("the reference picker distinguishes a partial asset search from absence and reaches an older match", async () => {
  const page = app();
  page.route("**/me", ({ url }) => ({
    json: {
      ...(platformResponse(url.pathname).body as object),
      features: { generate_skill: true, creation_skill: false },
    },
  }));
  const offsets = pagedAssets(page);
  page.route("**/api/skills/search?*", () => ({
    json: {
      query: "Target",
      results: [],
      total: 0,
      no_results: true,
      limit: 20,
      truncated: false,
      degraded: false,
      partial_index: false,
      filtered_out: false,
    },
  }));
  await page.goto("/workspace/creations");
  const reveal = page.getByText("附一張流程圖，或指定要參考的小工具（都是選填）", { exact: true });
  await page.eventually(() => expect(reveal.count()).toBe(1));
  await reveal.click();
  await page.getByLabel("搜尋要參考的小工具", { exact: true }).fill("Target");
  await page.eventually(() =>
    expect(
      page.getByRole("heading", { name: "資產庫中的小工具", exact: true }).count(),
    ).toBeGreaterThan(0),
  );
  expect(page.getByText("資產庫裡沒有符合項目。", { exact: true }).count()).toBe(0);
  const more = page.getByRole("button", { name: LOAD_MORE, exact: true });
  await page.eventually(() => expect(more.count()).toBeGreaterThan(0));
  await more.click();
  const targetBox = page.getByRole("checkbox", { name: /Target asset/ });
  await page.eventually(() => expect(targetBox.count()).toBeGreaterThan(0));
  await targetBox.check();
  await page.eventually(() =>
    expect(page.getByRole("list", { name: "已選的參考小工具", exact: true }).text()).toContain(
      "Target asset",
    ),
  );
  expect(offsets).toEqual([0, 24]);
});
