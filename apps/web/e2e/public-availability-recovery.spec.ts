import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  PUBLIC_PUBLICATION,
  PUBLIC_BUNDLE_PUBLICATION,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { serveStatic, STATIC_KINDS } from "./stub";

const DELISTED_NOTE = "作者撤回了這個發佈物，這一頁不再提供它的內容。";

async function publicationScenario(
  page: Page,
  kind: "skill" | "bundle",
  next: "unavailable" | "missing" | "failure" | "available",
  pause = false,
) {
  const initial = kind === "skill" ? PUBLIC_PUBLICATION : PUBLIC_BUNDLE_PUBLICATION;
  const address = `/publications/${initial.publisher}/${initial.name}`;
  let release!: () => void;
  const hold = new Promise<void>((resolve) => {
    release = resolve;
  });
  const state = { reads: 0, writes: 0, release, url: initial.address };
  const unavailable = {
    kind: initial.kind,
    publisher: initial.publisher,
    name: initial.name,
    address: initial.address,
    availability: { value: "delisted", label: "作者已撤回", note: DELISTED_NOTE },
    delisted_at: "2026-10-20T18:00:00Z",
    releases: [],
    exposure: { available: false, note: DELISTED_NOTE },
    acquisition: { available: false, note: DELISTED_NOTE },
  };
  await page.route("**/*", async (route, request) => {
    if (STATIC_KINDS.has(request.resourceType())) return serveStatic(route);
    const path = new URL(request.url()).pathname;
    if (path === address + "/acquisitions" && request.method() === "POST") {
      state.writes++;
      if (pause) await setOffline(page, true);
      return route.fulfill({
        status: next === "missing" ? 404 : 409,
        json:
          next === "missing"
            ? { error: "not found" }
            : { error: DELISTED_NOTE, reason: "delisted" },
      });
    }
    if (path === address) {
      state.reads++;
      if (state.reads === 1) return route.fulfill({ json: initial });
      await hold;
      if (next === "missing")
        return route.fulfill({ status: 404, json: { error: "no publication has this address" } });
      if (next === "failure" && state.reads === 2)
        return route.fulfill({ status: 500, json: { error: "publication lookup failed" } });
      return route.fulfill({ json: next === "available" ? initial : unavailable });
    }
    const response = platformResponse(request.url());
    return route.fulfill({ status: response.status, json: response.body });
  });
  await page.setViewportSize({ width: kind === "bundle" ? 1280 : 375, height: 900 });
  await page.goto(state.url);
  await page.getByRole("button", { name: "下載", exact: true }).click();
  return state;
}

async function withoutObsoleteContent(page: Page) {
  await expect(page.getByRole("button", { name: "下載", exact: true })).toHaveCount(0);
  await expect(page.locator(".publication-trust")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "取得這份套件", exact: true })).toHaveCount(0);
  await expect(page.getByRole("main")).toBeFocused();
}

async function capture(page: Page, state: string) {
  const report = await new AxeBuilder({ page }).include("main").analyze();
  await writeFile(
    test.info().outputPath(state + "-availability-a11y.json"),
    JSON.stringify(report),
  );
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(state + "-availability.png") });
}

for (const kind of ["skill", "bundle"] as const) {
  for (const status of [404, 409]) {
    test(`a ${kind} acquisition rejected with ${status} withholds obsolete content through its availability check`, async ({
      page,
    }) => {
      const state = await publicationScenario(
        page,
        kind,
        status === 404 ? "missing" : "unavailable",
      );
      try {
        await expect(page.getByRole("status")).toContainText("載入這個發佈物中");
        await expect(page.getByRole("heading", { name: "公開發佈物", exact: true })).toBeVisible();
        await expect.poll(() => state.reads).toBe(2);
        await withoutObsoleteContent(page);
        await capture(page, "checking");
        state.release();
        await expect(page.getByRole("alert")).toHaveText(
          status === 404 ? "沒有這個發佈物。" : "作者已撤回",
        );
        if (status === 409)
          await expect(page.getByText(DELISTED_NOTE, { exact: true })).toBeVisible();
        await withoutObsoleteContent(page);
        await expect(page).toHaveURL(new RegExp(state.url + "$"));
        expect(state.reads).toBe(2);
        expect(state.writes).toBe(1);
        await capture(page, "unavailable");
      } finally {
        state.release();
      }
    });
  }

  test(`a ${kind} availability read failure retries the read without resending its rejected acquisition`, async ({
    page,
  }) => {
    const state = await publicationScenario(page, kind, "failure");
    try {
      await expect(page.getByRole("status")).toContainText("載入這個發佈物中");
      state.release();
      await expect(page.getByRole("alert")).toContainText("暫時無法讀取這個發佈物");
      await withoutObsoleteContent(page);
      await capture(page, "failed");
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await page.keyboard.press("Enter");
      await expect(page.getByRole("alert")).toHaveText("作者已撤回");
      await withoutObsoleteContent(page);
      await expect(page).toHaveURL(new RegExp(state.url + "$"));
      expect(state.reads).toBe(3);
      expect(state.writes).toBe(1);
      await capture(page, "recovered");
    } finally {
      state.release();
    }
  });
}

test("a paused public availability check explains the wait and resumes only the read", async ({
  page,
}) => {
  const state = await publicationScenario(page, "skill", "unavailable", true);
  try {
    await expect(page.getByRole("status")).toContainText("等待連線以讀取這個發佈物");
    await withoutObsoleteContent(page);
    expect(state.reads).toBe(1);
    expect(state.writes).toBe(1);
    await capture(page, "paused");
    state.release();
    await setOffline(page, false);
    await expect(page.getByRole("alert")).toHaveText("作者已撤回");
    await withoutObsoleteContent(page);
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
    await capture(page, "resumed");
  } finally {
    state.release();
    await setOffline(page, false);
  }
});

test("a newly available public address retains its rejected acquisition outcome", async ({
  page,
}) => {
  const state = await publicationScenario(page, "skill", "available");
  try {
    await expect(page.getByRole("status")).toContainText("載入這個發佈物中");
    await withoutObsoleteContent(page);
    await capture(page, "checking");
    state.release();
    const refusal = page.getByRole("alert").filter({ hasText: DELISTED_NOTE });
    await expect(page.getByText("上次下載的回饋", { exact: true })).toBeVisible();
    await expect(refusal).toBeFocused();
    await expect(page.getByRole("button", { name: "下載", exact: true })).toBeEnabled();
    await expect(page.getByRole("heading", { name: "PDF Summariser", exact: true })).toBeVisible();
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
    await capture(page, "reopened");
  } finally {
    state.release();
  }
});

test("an availability check preserves focus moved to the page header", async ({ page }) => {
  const state = await publicationScenario(page, "skill", "unavailable");
  try {
    await expect(page.getByRole("status")).toContainText("載入這個發佈物中");
    const search = page.getByPlaceholder("搜尋小工具或描述任務");
    await search.fill("PDF");
    await capture(page, "header-focused");
    state.release();
    await expect(page.getByRole("alert")).toHaveText("作者已撤回");
    await expect(search).toBeFocused();
    await page.keyboard.type(" tools");
    await expect(search).toHaveValue("PDF tools");
    await expect(page.getByRole("button", { name: "下載", exact: true })).toHaveCount(0);
    await expect(page.locator(".publication-trust")).toHaveCount(0);
    expect(state.reads).toBe(2);
    expect(state.writes).toBe(1);
    await capture(page, "header-preserved");
  } finally {
    state.release();
  }
});

for (const kind of ["skill", "bundle"] as const) {
  test(`a reopened ${kind} publication preserves ongoing header search input`, async ({ page }) => {
    const state = await publicationScenario(page, kind, "available");
    try {
      await expect(page.getByRole("status")).toContainText("載入這個發佈物中");
      const search = page.getByPlaceholder("搜尋小工具或描述任務");
      await search.fill("PDF");
      await capture(page, "reopening-header-focused");
      state.release();
      await expect(
        page.getByRole("heading", {
          name: kind === "skill" ? "PDF Summariser" : "pdf-toolkit",
          exact: true,
        }),
      ).toBeVisible();
      await expect(page.getByRole("alert")).toHaveText(DELISTED_NOTE);
      await expect(search).toBeFocused();
      await page.keyboard.type(" tools");
      await expect(search).toHaveValue("PDF tools");
      await expect(page.getByRole("button", { name: "下載", exact: true })).toBeEnabled();
      expect(state.reads).toBe(2);
      expect(state.writes).toBe(1);
      await capture(page, "reopened-header-preserved");
    } finally {
      state.release();
    }
  });
}
