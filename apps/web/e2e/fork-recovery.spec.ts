import AxeBuilder from "@axe-core/playwright";
import { writeFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import {
  SKILL,
  SKILL_VERSIONS,
  skillDetail,
  platformResponse,
} from "../src/testing/fixtures/platform";
import type { OwnSkills } from "../src/core/api/types";
import { STATIC_KINDS, serveStatic, stubPlatform } from "./stub";

const COPIED = {
  skill_id: "abcdef01-2345-4789-abcd-0123456789ab",
  version_id: "abcdef02-2345-4789-abcd-0123456789ab",
  version_number: 1,
  name: "PDF Summariser-fork",
  summary: "",
  redistribution: "self_supplied",
};

const SECOND_COPY = {
  ...COPIED,
  skill_id: "abcdef03-2345-4789-abcd-0123456789ab",
  version_id: "abcdef04-2345-4789-abcd-0123456789ab",
  name: "PDF Summariser-fork-2",
};

async function verifyLayout(page: Page, state: string) {
  const results = await new AxeBuilder({ page }).analyze();
  const report = test.info().outputPath(`${state}-accessibility.json`);
  await writeFile(report, JSON.stringify(results));
  await test
    .info()
    .attach(`${state}-accessibility`, { path: report, contentType: "application/json" });
  expect(results.violations).toEqual([]);
  expect(results.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`${state}.png`) });
}

for (const width of [375, 1280]) {
  test(`a long source name and fork receipt wrap without clipping at ${width}px`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    const name = "a".repeat(63);
    await page.route(`**/api/skills/${SKILL}`, (route) =>
      route.fulfill({ json: skillDetail(SKILL, name) }),
    );
    await page.route(`**/skills/${SKILL}/fork`, (route) =>
      route.fulfill({ status: 201, json: { ...COPIED, name: `${name}-fork` } }),
    );
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await expect(page.getByRole("heading", { level: 1, name, exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      width,
    );
    await page.screenshot({ path: test.info().outputPath(`long-source-${width}.png`) });
    await page.getByRole("button", { name: "以這個小工具為起點建立我自己的", exact: true }).click();
    const receipt = page.getByRole("link", {
      name: `開啟 ${name}-fork v1 並繼續驗證`,
      exact: true,
    });
    await expect(receipt).toHaveAttribute(
      "href",
      `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
    );
    await expect(receipt).toBeFocused();
    await verifyLayout(page, `long-name-${width}`);
  });
}

for (const [status, message] of [
  [400, "這次複製請求未獲接受"],
  [401, "複製小工具需要登入。"],
  [403, "這個帳號還沒有封測邀請"],
  [404, "這個小工具已經不在了"],
  [409, "你的工作區已經有同名的小工具，"],
] as const) {
  test(`HTTP ${status} refuses a fork without suggesting that its outcome is unknown`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let writes = 0;
    await page.route(`**/skills/${SKILL}/fork`, (route) => {
      writes += 1;
      return route.fulfill({
        status,
        json: {
          error:
            status === 403
              ? "Skill Hub is in closed beta: browsing and skill details are open to everyone, but forking, running and downloading are limited to invited participants."
              : status === 401
                ? "not authenticated"
                : status === 409
                  ? "你的工作區已經有同名的 Skill"
                  : "not found",
        },
      });
    });
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await page.getByRole("button", { name: "以這個小工具為起點建立我自己的", exact: true }).click();
    await expect(
      page.getByRole(status === 401 ? "status" : "alert").filter({ hasText: message }),
    ).toBeVisible();
    await expect(page.getByRole("main")).not.toContainText("尚未確認是否已複製");
    await expect(
      page.getByRole("link", { name: "另開資產庫確認複製結果", exact: true }),
    ).toHaveCount(0);
    if (status === 409)
      await expect(
        page.getByRole("link", { name: "前往資產庫找出同名小工具", exact: true }),
      ).toHaveAttribute("href", "/library");
    expect(writes).toBe(1);
    await verifyLayout(page, `refused-${status}`);
  });
}

test("forking another workspace's skill hands the keyboard off to the exact new owned version", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let writes = 0;
  await page.route(`**/skills/${SKILL}/versions`, (route) =>
    route.fulfill({ json: { versions: [] } }),
  );
  await page.route(`**/skills/${SKILL}/fork`, (route) => {
    writes += 1;
    return route.fulfill({ status: 201, json: COPIED });
  });
  await page.route(`**/api/skills/${COPIED.skill_id}*`, (route) =>
    route.fulfill({
      json: {
        ...skillDetail(COPIED.skill_id, COPIED.name),
        version: {
          ...SKILL_VERSIONS.versions[0],
          version_id: COPIED.version_id,
          version_number: 1,
        },
      },
    }),
  );
  await page.route(`**/skills/${COPIED.skill_id}/versions`, (route) =>
    route.fulfill({
      json: {
        versions: [
          { ...SKILL_VERSIONS.versions[0], version_id: COPIED.version_id, version_number: 1 },
        ],
      },
    }),
  );
  await page.goto(`/skills/${SKILL}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await expect(page.getByLabel("這個小工具是做什麼用的", { exact: true })).toHaveCount(0);
  const create = page.getByRole("button", { name: "以這個小工具為起點建立我自己的", exact: true });
  await expect(create).toHaveClass("action");
  await create.focus();
  await page.keyboard.press("Enter");
  const receipt = page.getByRole("link", {
    name: "開啟 PDF Summariser-fork v1 並繼續驗證",
    exact: true,
  });
  await expect(receipt).toBeFocused();
  await expect(receipt).toHaveAttribute(
    "href",
    `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
  );
  await expect(page.getByRole("button", { name: "已建立自己的版本", exact: true })).toBeDisabled();
  expect(writes).toBe(1);
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("heading", { level: 1, name: "PDF Summariser-fork v1", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".version-facts")).toBeVisible();
  expect(writes).toBe(1);
  await verifyLayout(page, "owned-version");
});

for (const outcome of ["lost response", "unavailable", "malformed receipt"]) {
  test(`a ${outcome} fork stays uncertain through a read-only Library check and an explicit resend`, async ({
    page,
    context,
  }) => {
    let writes = 0;
    let recovered = false;
    let sourceFailed = false;
    await context.route("**/*", (route, request) => {
      if (STATIC_KINDS.has(request.resourceType())) return serveStatic(route);
      const path = new URL(request.url()).pathname;
      if (path === `/api/skills/${SKILL}` && sourceFailed)
        return route.fulfill({ status: 503, json: { error: "service unavailable" } });
      if (path === `/skills/${SKILL}/category`) {
        expect(request.method()).toBe("PUT");
        sourceFailed = true;
        return route.fulfill({
          json: { skill_id: SKILL, name: "PDF Summariser", summary: "", redistribution: "allowed" },
        });
      }
      if (path === `/skills/${SKILL}/fork`) {
        writes += 1;
        expect(request.method()).toBe("POST");
        if (writes > 1) return route.fulfill({ status: 201, json: SECOND_COPY });
        if (outcome === "lost response") return route.abort("failed");
        if (outcome === "unavailable")
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        return route.fulfill({ status: 201, json: { ...COPIED, version_id: "not-a-version" } });
      }
      if (path === "/skills") {
        expect(request.method()).toBe("GET");
        if (!recovered)
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        const original = platformResponse(request.url()).body as OwnSkills;
        return route.fulfill({
          json: {
            ...original,
            skills: [{ ...original.skills[0], ...COPIED }],
            limit: 24,
            total: 1,
            truncated: false,
          },
        });
      }
      const response = platformResponse(request.url());
      return route.fulfill({ status: response.status, json: response.body });
    });
    await page.setViewportSize({ width: 375, height: 900 });
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await page.getByRole("button", { name: "以這個小工具為起點建立我自己的", exact: true }).focus();
    await page.keyboard.press("Enter");
    const failure = page.getByRole("alert", { name: "複製小工具結果", exact: true });
    await expect(failure).toContainText("尚未確認是否已複製到你的工作區");
    await expect(failure).toContainText("重新送出可能再建立一份副本");
    await expect(failure).toBeFocused();
    const position = await failure.evaluate((element) => ({
      top: element.getBoundingClientRect().top,
      header: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
    }));
    expect(position.top).toBeGreaterThanOrEqual(position.header);
    const lookup = page.getByRole("link", { name: "另開資產庫確認複製結果", exact: true });
    await expect(lookup).toHaveAttribute("href", "/library");
    await expect(lookup).toHaveAttribute("target", "_blank");
    await verifyLayout(page, "uncertain");
    await page.getByLabel("這個小工具是做什麼用的", { exact: true }).selectOption("writing");
    await page.getByRole("button", { name: "儲存", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "小工具總覽", level: 1, exact: true }),
    ).toBeVisible();
    await expect(failure).toContainText("尚未確認");
    await expect(
      page.getByRole("button", { name: "重新送出複製請求", exact: true }),
    ).toBeDisabled();
    await expect(
      page.getByRole("status").filter({ hasText: "小工具資訊尚未重新確認，暫時無法開始複製。" }),
    ).toBeVisible();
    sourceFailed = false;
    await page
      .locator("section")
      .filter({ hasText: "暫時無法讀取這個小工具" })
      .getByRole("button", { name: "再試一次", exact: true })
      .click();
    await expect(page.getByRole("button", { name: "重新送出複製請求", exact: true })).toBeEnabled();
    await expect(failure).toContainText("尚未確認");
    const [popup] = await Promise.all([context.waitForEvent("page"), lookup.click()]);
    try {
      await expect(
        popup.getByRole("alert").filter({ hasText: "暫時無法讀取資產庫內容" }),
      ).toBeVisible();
      await expect(popup.getByRole("main")).not.toContainText("資產庫還沒有任何小工具");
      expect(writes).toBe(1);
      recovered = true;
      await popup.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(
        popup.getByRole("link", { name: "PDF Summariser-fork", exact: true }),
      ).toBeVisible();
      expect(writes).toBe(1);
    } finally {
      await popup.close();
    }
    await expect(failure).toContainText("尚未確認");
    await page.getByRole("button", { name: "重新送出複製請求", exact: true }).click();
    const secondReceipt = page.getByRole("link", {
      name: "開啟 PDF Summariser-fork-2 v1 並繼續驗證",
      exact: true,
    });
    await expect(secondReceipt).toHaveAttribute(
      "href",
      `/skills/${SECOND_COPY.skill_id}/versions/${SECOND_COPY.version_id}`,
    );
    await expect(secondReceipt).toBeFocused();
    await expect(failure).toHaveCount(0);
    expect(writes).toBe(2);
    await expect(lookup).toHaveCount(0);
    await verifyLayout(page, "second-copy");
  });
}

for (const phase of ["confirmed", "pending"]) {
  test(`a ${phase} fork survives a source read failure without replaying or stealing draft focus`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let copied = false;
    let changed = false;
    let writes = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/skills/${SKILL}/fork`, async (route) => {
      writes += 1;
      copied = true;
      if (phase === "pending") await gate;
      return route.fulfill({ status: 201, json: COPIED });
    });
    await page.route(`**/api/skills/${SKILL}`, (route) =>
      route.fulfill(
        changed
          ? { status: 503, json: { error: "service unavailable" } }
          : { json: skillDetail(SKILL, "PDF Summariser") },
      ),
    );
    await page.route(`**/skills/${SKILL}/category`, (route) => {
      expect(copied).toBe(true);
      changed = true;
      return route.fulfill({
        json: { skill_id: SKILL, name: "PDF Summariser", summary: "", redistribution: "allowed" },
      });
    });
    await page.setViewportSize({ width: 375, height: 900 });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await page
        .getByRole("button", { name: "以這個小工具為起點建立我自己的", exact: true })
        .click();
      const receipt = page.getByRole("link", {
        name: "開啟 PDF Summariser-fork v1 並繼續驗證",
        exact: true,
      });
      if (phase === "confirmed") {
        await expect(receipt).toHaveAttribute(
          "href",
          `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
        );
        await expect(receipt).toBeFocused();
      } else {
        await expect(page.getByRole("button", { name: "建立中…", exact: true })).toBeDisabled();
        await expect(
          page.getByRole("status").filter({ hasText: "正在等待建立結果" }),
        ).toBeVisible();
      }
      await expect.poll(() => copied).toBe(true);
      await page.getByLabel("這個小工具是做什麼用的", { exact: true }).selectOption("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(
        page.getByRole("heading", { name: "小工具總覽", level: 1, exact: true }),
      ).toBeVisible();
      const choice = page.getByLabel("這個小工具是做什麼用的", { exact: true });
      await choice.selectOption("data");
      await choice.focus();
      release();
      await expect(receipt).toBeVisible();
      await expect(choice).toBeFocused();
      await expect(choice).toHaveValue("data");
      await expect(
        page.getByRole("button", { name: "已建立自己的版本", exact: true }),
      ).toBeDisabled();
      expect(writes).toBe(1);
      await verifyLayout(page, `${phase}-read-failed`);
      changed = false;
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(page.locator(".detail-answer")).toBeVisible();
      await expect(receipt).toHaveAttribute(
        "href",
        `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
      );
      await expect(choice).toHaveValue("data");
      expect(writes).toBe(1);
      await verifyLayout(page, `${phase}-recovered`);
    } finally {
      release();
    }
  });
}
