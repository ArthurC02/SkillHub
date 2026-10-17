import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import {
  RUN,
  SKILL,
  SUGGESTIONS,
  SKILL_VERSIONS,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

const NEW_VERSION = "99999999-1111-4111-8111-111111111111";
async function verifyRecoveryFocus(page: Page) {
  const heading = page.locator("#suggestions-heading");
  await expect(heading).toBeFocused();
  const position = await heading.evaluate((element) => ({
    top: element.getBoundingClientRect().top,
    headerBottom: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
  }));
  expect(position.top).toBeGreaterThanOrEqual(position.headerBottom);
}
async function verifyVersions(page: Page, saved: boolean) {
  await page.getByRole("link", { name: "核對小工具版本", exact: true }).click();
  await expect(page.getByRole("heading", { name: "PDF Summariser", exact: true })).toBeVisible();
  await expect(
    page.getByText(saved ? "共 3 版，最新 v3" : "共 2 版，最新 v2", { exact: false }),
  ).toBeVisible();
}

test("a complete apply receipt exposes the created version and preserves the original Run for comparison", async ({
  page,
}) => {
  await stubPlatform(page);
  const writes: string[] = [];
  const receiver = receiverFor("valid", (body) => writes.push(body));
  await new Promise<void>((resolve, reject) => {
    receiver.once("error", reject);
    receiver.listen(0, "127.0.0.1", resolve);
  });
  try {
    await page.route(`**/skills/${SKILL}/versions/from-suggestions`, (route) =>
      route.continue({ url: `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/apply` }),
    );
    await page.goto(`/runs/${RUN}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await page.getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true }).click();
    const region = page.getByRole("region", { name: "改善建議", exact: true });
    await expect(region.getByRole("status")).toContainText("已建立新版本 #3");
    await expect(
      region.getByRole("link", { name: "開啟剛建立的版本", exact: true }),
    ).toHaveAttribute("href", `/skills/${SKILL}/versions/${NEW_VERSION}`);
    const rerun = region.getByRole("link", { name: "以新版本重跑這個測試題", exact: true });
    const url = new URL((await rerun.getAttribute("href")) as string, "http://127.0.0.1:4173");
    expect(url.searchParams.get("version")).toBe(NEW_VERSION);
    expect(url.searchParams.get("against")).toBe(RUN);
    await expect(region.getByRole("alert")).toHaveCount(0);
    await expect(
      region.getByRole("button", { name: "重新確認建議套用狀態", exact: true }),
    ).toHaveCount(0);
    expect(writes).toHaveLength(1);
    expect(JSON.parse(writes[0])).toEqual({ evaluation_id: "eval-2", suggestion_ids: ["s1"] });
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  } finally {
    receiver.closeAllConnections();
    await new Promise<void>((resolve) => receiver.close(() => resolve()));
  }
});

for (const blockedSession of [false, true]) {
  test(`${blockedSession ? "session-blocked" : "rejected"} apply keeps its known outcome`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let checking = false;
    let writes = 0;
    await page.route("**/me", (route) =>
      checking
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({ json: platformResponse("/me").body }),
    );
    await page.route(`**/skills/${SKILL}/versions/from-suggestions`, (route) => {
      writes += 1;
      return route.fulfill({
        status: 422,
        json: {
          error: "no applicable suggestions",
          rejected_suggestions: [
            { suggestion_id: "s1", blocked_reason: "target_changed", message: "目標檔案變了。" },
          ],
        },
      });
    });
    await page.goto(`/runs/${RUN}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    const apply = page.getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true });
    await expect(apply).toBeEnabled();
    if (blockedSession) {
      checking = true;
      await page.evaluate(() =>
        document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
      );
      await expect(
        page.getByRole("button", { name: "重新確認登入狀態", exact: true }),
      ).toBeVisible();
    }
    await apply.click();
    const region = page.getByRole("region", { name: "改善建議", exact: true });
    if (blockedSession) {
      await expect(region.getByRole("alert")).toHaveText(
        "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。",
      );
      await expect(region.getByRole("alert")).toBeFocused();
    } else {
      await expect(region.getByRole("alert", { name: "建立版本結果", exact: true })).toContainText(
        "沒有一項建議可以套用，所以沒有建立新版本。",
      );
      await expect(region.getByRole("alert", { name: "建立版本結果", exact: true })).toContainText(
        "目標檔案變了。",
      );
    }
    await expect(
      region.getByRole("button", { name: "重新確認建議套用狀態", exact: true }),
    ).toHaveCount(0);
    await expect(region.getByRole("link", { name: "核對小工具版本", exact: true })).toHaveCount(0);
    expect(writes).toBe(blockedSession ? 0 : 1);
  });
}
function receiverFor(transport: string, onBody: (body: string) => void) {
  return createServer(async (request, response) => {
    const headers = {
      "Access-Control-Allow-Origin": request.headers.origin ?? "http://127.0.0.1:4173",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Allow-Headers": "accept, content-type",
      "Access-Control-Allow-Methods": "POST",
      "Content-Type": "application/json",
    };
    if (request.method === "OPTIONS") {
      response.writeHead(204, headers).end();
      return;
    }
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    onBody(Buffer.concat(chunks).toString("utf8"));
    if (transport === "network") {
      response.writeHead(201, { ...headers, "Content-Length": "100" });
      response.write("{", () => response.destroy());
      return;
    }
    const bodies: Record<string, string> = {
      server500: JSON.stringify({ error: "internal error" }),
      server503: JSON.stringify({ error: "service unavailable" }),
      malformed: "{",
      incomplete: JSON.stringify({
        skill_id: SKILL,
        version_id: NEW_VERSION,
        version_number: 3,
        applied_suggestion_ids: ["s1"],
        rejected_suggestions: [],
      }),
      null: "null",
      valid: JSON.stringify({
        skill_id: SKILL,
        version_id: NEW_VERSION,
        version_number: 3,
        content_hash: "hash-new",
        duplicate: false,
        applied_suggestion_ids: ["s1"],
        rejected_suggestions: [],
      }),
    };
    response
      .writeHead(transport === "server500" ? 500 : transport === "server503" ? 503 : 201, headers)
      .end(bodies[transport]);
  });
}

for (const [transport, saved, associated, failedRead] of [
  ["network", true, true, false],
  ["network", true, false, false],
  ["server503", false, false, false],
  ["server500", true, true, false],
  ["malformed", true, true, false],
  ["incomplete", true, true, false],
  ["null", true, true, false],
  ["network", true, true, true],
] as const) {
  test(`${transport} apply saved=${saved} association=${associated} read failure=${failedRead} recovers by reading evidence`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let committed = false;
    let reads = 0;
    const writes: string[] = [];
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const receiver = receiverFor(transport, (body) => {
      writes.push(body);
      committed = saved;
    });
    await new Promise<void>((resolve, reject) => {
      receiver.once("error", reject);
      receiver.listen(0, "127.0.0.1", resolve);
    });
    try {
      await page.route(`**/skills/${SKILL}/versions/from-suggestions`, (route) =>
        route.continue({
          url: `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/apply`,
        }),
      );
      await page.route(`**/runs/${RUN}/suggestions`, async (route) => {
        reads += 1;
        if (reads === 2 && failedRead)
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        if (reads === 3) await gate;
        return route.fulfill({
          json: {
            ...SUGGESTIONS,
            suggestions: SUGGESTIONS.suggestions.map((s) => ({
              ...s,
              ...(committed && associated ? { applied_skill_version_id: NEW_VERSION } : {}),
            })),
          },
        });
      });
      await page.route(`**/skills/${SKILL}/versions`, (route) =>
        route.fulfill({
          json: {
            versions: committed
              ? [
                  {
                    version_id: NEW_VERSION,
                    version_number: 3,
                    content_hash: "hash-new",
                    created_at: "2026-10-15T12:00:00Z",
                  },
                  ...SKILL_VERSIONS.versions,
                ]
              : SKILL_VERSIONS.versions,
          },
        }),
      );
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await page
        .getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true })
        .click();
      const region = page.getByRole("region", { name: "改善建議", exact: true });
      const alert = region.getByRole("alert", { name: "建立版本結果", exact: true });
      await expect(alert).toContainText(
        "無法確認是否已建立新版本。請先核對版本清單與建議套用狀態，再決定是否重試。",
      );
      await expect(alert).toContainText("尚未回報套用版本，不代表沒有建立版本。");
      await expect(alert).toBeFocused();
      await expect.poll(() => reads).toBe(2);
      expect(writes).toHaveLength(1);
      expect(JSON.parse(writes[0])).toEqual({ evaluation_id: "eval-2", suggestion_ids: ["s1"] });
      if (failedRead)
        await expect(
          region
            .getByRole("alert")
            .filter({ hasNot: page.getByRole("link", { name: "核對小工具版本", exact: true }) })
            .last(),
        ).toContainText("暫時無法讀取改善建議");
      else if (associated)
        await expect(region.getByText("已建成版本的建議不能撤回", { exact: true })).toBeVisible();
      const retry = region.getByRole("button", { name: "重新確認建議套用狀態", exact: true });
      await expect(alert).toContainText("無法確認是否已建立新版本。");
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect.poll(() => reads).toBe(3);
      await verifyRecoveryFocus(page);
      await expect(retry).toBeDisabled();
      await expect(alert).toContainText("無法確認是否已建立新版本。");
      expect(writes).toHaveLength(1);
      if (!failedRead)
        await expect(
          region.getByRole("button", {
            name: "核對後仍以已接受的 1 項建議建立新版本",
            exact: true,
          }),
        ).toBeDisabled();
      release();
      await expect(retry).toBeEnabled();
      await expect(
        region.getByRole("button", { name: "核對後仍以已接受的 1 項建議建立新版本", exact: true }),
      ).toBeEnabled();
      await expect(region.getByText(/^已建立新版本/)).toHaveCount(0);
      expect(writes).toHaveLength(1);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      await alert.scrollIntoViewIfNeeded();
      await page.screenshot({ path: test.info().outputPath("apply-unknown.png") });
      await verifyVersions(page, committed);
      expect(writes).toHaveLength(1);
    } finally {
      release();
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}
