import AxeBuilder from "./axe";
import { writeFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";
import { SKILL, platformResponse } from "../src/testing/fixtures/platform";
import type { OwnSkills } from "../src/core/api/types";
import { STATIC_KINDS, serveStatic } from "./stub";

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
