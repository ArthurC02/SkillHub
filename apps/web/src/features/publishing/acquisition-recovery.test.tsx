import { expect, test } from "vitest";
import {
  ARTIFACT,
  OWN_BUNDLE,
  PUBLICATION,
  PUBLISHER,
  PUBLIC_BUNDLE_PUBLICATION,
} from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();

const RECEIPT = {
  artifact_id: ARTIFACT,
  file_name: "x".repeat(128) + ".zip",
  size_bytes: 0,
  content_hash: "sha256:another-release",
  expires_at: "2099-01-01T00:00:00Z",
  duplicate: true,
  content_url: `/downloads/${ARTIFACT}/content`,
};
type Kind = "skill" | "public bundle" | "private bundle";

function stubAcquisition(page: App, kind: Kind, reply: unknown, status = 201) {
  const writes: string[] = [];
  page.route(
    () => true,
    ({ url, method }) => {
      const path = url.pathname;
      if (method === "POST" && (path.endsWith("/acquisitions") || path.endsWith("/export"))) {
        writes.push(url.href);
        return { status, json: reply };
      }
      if (path === "/me/bundles") return { json: { bundles: [OWN_BUNDLE] } };
      if (path === "/me/bundles/overview")
        return { json: { bundles: [{ bundle: "pdf-toolkit", latest_version: OWN_BUNDLE }] } };
      if (path === `/publications/${PUBLISHER}/pdf-toolkit`)
        return { json: PUBLIC_BUNDLE_PUBLICATION };
      return undefined;
    },
  );
  return {
    writes,
    url:
      kind === "private bundle"
        ? "/workspace/downloads"
        : `/p/${PUBLISHER}/${kind === "skill" ? PUBLICATION : "pdf-toolkit"}`,
    button: kind === "private bundle" ? "匯出 v1.1.0 Plugin" : "下載",
  };
}

async function openAndClick(page: App, url: string, button: string) {
  await page.goto(url);
  await expect(page.getByRole("button", { name: button, exact: true })).toHaveCount(1);
  await page.getByRole("button", { name: button, exact: true }).click();
}

async function expectAccessible(page: App) {
  expect(await page.axeViolations("main")).toEqual([]);
}

for (const kind of ["skill", "public bundle", "private bundle"] as const) {
  for (const damage of ["null", "missing URL"] as const) {
    test(`an uncertain ${kind} acquisition with ${damage} retains context during keyboard verification`, async () => {
      const page = app();
      const { content_url: _contentURL, ...incomplete } = RECEIPT;
      const state = stubAcquisition(page, kind, damage === "null" ? null : incomplete);
      await openAndClick(page, state.url, state.button);
      const alert = () => page.getByRole("alert").filter({ hasText: "下載結果尚未確認" });
      await expect(alert()).toBeFocused();
      expect(page.pathname() + page.search()).toBe(state.url);
      await expect(
        page.getByRole("heading", {
          name:
            kind === "private bundle"
              ? "發佈與交付"
              : kind === "skill"
                ? "PDF Summariser"
                : "pdf-toolkit",
          exact: true,
        }),
      ).toBeVisible();
      expect(state.writes).toHaveLength(1);
      await expect(page.locator('a[href="undefined"]')).toHaveCount(0);
      await expectAccessible(page);
      const recovery = page.getByRole("link", { name: "另開交付紀錄核對", exact: true });
      expect(recovery.attribute("href")).toBe("/workspace/downloads");
      expect(recovery.attribute("rel")).toBe("noopener noreferrer");
      expect(recovery.attribute("target")).toBe("_blank");
      await recovery.focus();
      await page.press("Enter");
      expect(page.pathname() + page.search()).toBe(state.url);
      await expect(alert()).toContainText("下載結果尚未確認");
      expect(state.writes).toHaveLength(1);
    });
  }

  test(`a valid ${kind} acquisition keeps a long filename and exact delivery link readable`, async () => {
    const page = app();
    const state = stubAcquisition(page, kind, RECEIPT);
    await openAndClick(page, state.url, state.button);
    const download = () =>
      page.getByRole("link", { name: "下載 " + RECEIPT.file_name, exact: true });
    await expect(download()).toHaveCount(1);
    expect(download().attribute("href")).toBe(`/downloads/${ARTIFACT}/content`);
    await expect(page.getByRole("status").filter({ hasText: RECEIPT.file_name })).toBeVisible();
    const history = page.getByRole("link", {
      name: kind === "private bundle" ? "查看這次交付紀錄" : "在交付紀錄查看這一份",
      exact: true,
    });
    expect(history.attribute("href")).toBe(`/workspace/downloads?artifact=${ARTIFACT}`);
    expect(state.writes).toHaveLength(1);
    await expectAccessible(page);
  });
}

test("a foreign acquisition URL offers record verification without an unverified download", async () => {
  const page = app();
  const state = stubAcquisition(page, "public bundle", {
    ...RECEIPT,
    content_url: "/downloads/55555555-5555-4555-8555-555555555555/content",
  });
  await openAndClick(page, state.url, "下載");
  await expect(page.getByRole("alert").filter({ hasText: "下載結果尚未確認" })).toBeFocused();
  await expect(page.locator('a[href*="55555555-5555"]')).toHaveCount(0);
  await expect(page.getByRole("link", { name: "另開交付紀錄核對", exact: true })).toHaveCount(1);
  expect(state.writes).toHaveLength(1);
  await expectAccessible(page);
});

for (const [status, message, expected] of [
  [401, "not authenticated", "這個下載需要登入"],
  [
    403,
    "Skill Hub 還在封測：瀏覽與小工具詳情對所有人開放，但複製一份、試跑與下載只開放給受邀的測試者。",
    "還在封測",
  ],
  [409, "作者撤回了這個發佈物，這一頁不再提供它的內容。", "作者撤回了這個發佈物"],
] as const) {
  test(`an acquisition rejected with ${status} retains its definite outcome`, async () => {
    const page = app();
    const state = stubAcquisition(page, "skill", { error: message }, status);
    await openAndClick(page, state.url, "下載");
    await expect(
      page.getByRole(status === 401 ? "status" : "alert").filter({ hasText: expected }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "另開交付紀錄核對", exact: true })).toHaveCount(0);
    expect(state.writes).toHaveLength(1);
    await expectAccessible(page);
  });
}

test("an acquisition server error retains publication context and offers verification", async () => {
  const page = app();
  const state = stubAcquisition(page, "skill", { error: "internal error" }, 500);
  await openAndClick(page, state.url, "下載");
  await expect(page.getByRole("alert").filter({ hasText: "下載結果尚未確認" })).toBeFocused();
  await expect(page.getByRole("link", { name: "另開交付紀錄核對", exact: true })).toHaveCount(1);
  expect(state.writes).toHaveLength(1);
  await expectAccessible(page);
});
