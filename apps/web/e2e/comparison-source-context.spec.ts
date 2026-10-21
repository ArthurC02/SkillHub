import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type Route } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import {
  OTHER_RUN,
  RUN,
  RUNS,
  SKILL,
  SKILL_VERSIONS,
  platformResponse,
} from "../src/testing/fixtures/platform";
import { setOffline } from "./offline";
import { stubPlatform } from "./stub";

const SOURCE_VERSION = "abcdefab-1111-4111-8111-111111111111";
const MISSING_VERSION = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const SOURCE_LABEL = "比較基準：v1";

async function setup(
  page: Page,
  versionId = SOURCE_VERSION,
  empty = false,
  dependency: "self" | "versions" = "versions",
) {
  await stubPlatform(page);
  const control = { phase: "ready", reads: [] as string[] };
  const requests = {
    self: 0,
    versions: 0,
    candidates: 0,
    comparisons: [] as string[],
    writes: [] as string[],
  };
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  const heldStarted = new Promise<void>((resolve) => (started = resolve));
  const versions = SKILL_VERSIONS.versions.map((version) =>
    version.version_number === 1 ? { ...version, version_id: SOURCE_VERSION } : version,
  );
  async function respond(route: Route, kind: "self" | "versions", body: unknown) {
    if (route.request().resourceType() === "document") return route.fallback();
    requests[kind] += 1;
    const phase = kind === dependency ? control.phase : "ready";
    if (kind === dependency) control.reads.push(phase);
    if (phase === "error")
      return route.fulfill({ status: 503, json: { error: "service unavailable" } });
    if (phase === "held") {
      started();
      await held;
    }
    return route.fulfill({ json: body });
  }
  await page.route(`**/runs/${RUN}`, (route) =>
    respond(route, "self", {
      ...(platformResponse(`/runs/${RUN}`).body as object),
      skill_version_id: versionId,
    }),
  );
  await page.route(`**/skills/${SKILL}/versions`, (route) =>
    respond(route, "versions", { versions }),
  );
  await page.route("**/runs?*", (route) => {
    requests.candidates += 1;
    return route.fulfill({
      json: { runs: empty ? [] : RUNS.runs.filter((run) => run.run_id === OTHER_RUN) },
    });
  });
  page.on("request", (request) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) requests.writes.push(request.url());
    if (new URL(request.url()).pathname.endsWith("/comparison"))
      requests.comparisons.push(request.url());
  });
  return { control, requests, release, heldStarted };
}

async function capture(page: Page, path: string) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(path, JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
}

for (const { mode, width, expected } of [
  { mode: "canonical", width: 375, expected: SOURCE_LABEL },
  { mode: "uppercase", width: 1280, expected: SOURCE_LABEL },
  { mode: "missing", width: 375, expected: "比較基準：版本編號未知" },
  { mode: "error", width: 1280, expected: "比較基準：版本編號未知" },
  { mode: "empty", width: 1280, expected: SOURCE_LABEL },
] as const) {
  test(`source Version context is truthful for ${mode} metadata at ${width}px`, async ({
    page,
  }, info) => {
    const id =
      mode === "missing"
        ? MISSING_VERSION
        : mode === "uppercase"
          ? SOURCE_VERSION.toUpperCase()
          : SOURCE_VERSION;
    const state = await setup(page, id, mode === "empty");
    if (mode === "error") state.control.phase = "error";
    await page.setViewportSize({ width, height: 900 });
    await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
    await page.goto(`/runs/${RUN}/compare`);
    const context = page.getByText(/^比較基準：/);
    await expect(context).toHaveText(expected);
    await expect(context).toBeInViewport({ ratio: 1 });
    await expect(page.getByRole("link", { name: "版本與發佈", exact: true })).toHaveAttribute(
      "href",
      `/skills/${SKILL}/versions/${id}`,
    );
    const rows = page.locator("main .download-list > .download-item");
    await expect(rows).toHaveCount(mode === "empty" ? 0 : 1);
    if (mode !== "error" && mode !== "empty") {
      await expect(rows.getByRole("link", { name: "v2", exact: true })).toBeVisible();
      await expect(rows.getByRole("button")).toBeInViewport({ ratio: 1 });
    }
    expect(state.requests.versions).toBe(1);
    expect(state.requests.self).toBe(1);
    expect(state.requests.candidates).toBe(1);
    if (mode === "empty")
      await page.evaluate(() => (document.documentElement.style.fontSize = "200%"));
    await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
    const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
    await field.fill("辨識基準時保留的比較 ID");
    await expect(field).toBeFocused();
    await expect(field).toBeInViewport({ ratio: 1 });
    await capture(page, info.outputPath("source-context-a11y.json"));
    await page.screenshot({ path: info.outputPath("source-context.png"), fullPage: true });
    if (mode === "error") {
      state.control.phase = "ready";
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      await expect(context).toHaveText(SOURCE_LABEL);
      await expect(field).toHaveValue("辨識基準時保留的比較 ID");
      expect(state.control.reads).toEqual(["error", "ready"]);
      await capture(page, info.outputPath("recovered-source-context-a11y.json"));
    }
    expect(state.requests.comparisons).toEqual([]);
    expect(state.requests.writes).toEqual([]);
  });
}

for (const mode of ["pending", "paused", "source-error"] as const) {
  test(`cached source Version context handles ${mode} without losing the comparison draft`, async ({
    page,
  }, info) => {
    const self = mode === "source-error";
    const paused = mode === "paused";
    const state = await setup(page, SOURCE_VERSION, false, self ? "self" : "versions");
    await page.setViewportSize({ width: paused ? 1280 : 375, height: 900 });
    await page.emulateMedia({ colorScheme: paused ? "dark" : "light" });
    const context = page.getByText(/^比較基準：/);
    try {
      await page.goto(`/runs/${RUN}/compare`);
      await expect(context).toHaveText(SOURCE_LABEL);
      await page
        .getByRole("navigation", { name: "這個小工具的工作台", exact: true })
        .getByRole("link", { name: "總覽", exact: true })
        .click();
      await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("試跑比較");
      await expect(
        page.getByRole("region", { name: "版本", exact: true }).getByText(/共\s*2\s*版，最新 v2/),
      ).toBeVisible();
      state.control.phase = "error";
      await page.goBack();
      await expect(page.getByRole("alert")).toContainText(
        self ? "暫時無法讀取目前這次試跑" : "暫時無法讀取候選試跑紀錄的 Version 編號",
      );
      if (self) await expect(context).toHaveCount(0);
      else await expect(context).toHaveText("比較基準：版本編號未知");
      await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
      const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
      await field.fill("恢復基準資訊前保留的比較 ID");
      await capture(page, info.outputPath("unreadable-source-context-a11y.json"));
      state.control.phase = "held";
      if (paused) await setOffline(page, true);
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.keyboard.press("Enter");
      if (self) await expect(context).toHaveCount(0);
      else
        await expect(context).toHaveText(
          paused ? "比較基準：版本編號待連線" : "比較基準：版本編號載入中",
        );
      await expect(page.getByRole("alert")).toHaveCount(0);
      await field.fill("恢復基準資訊時繼續輸入的比較 ID");
      await capture(page, info.outputPath("waiting-source-context-a11y.json"));
      if (paused) {
        expect(state.control.reads).not.toContain("held");
        await setOffline(page, false);
      }
      await state.heldStarted;
      if (!self) await expect(context).toHaveText("比較基準：版本編號載入中");
      expect(state.control.reads.filter((phase) => phase === "held")).toHaveLength(1);
      state.release();
      await expect(context).toHaveText(SOURCE_LABEL);
      await expect(field).toHaveValue("恢復基準資訊時繼續輸入的比較 ID");
      await expect(field).toBeFocused();
      await expect(field).toBeInViewport({ ratio: 1 });
      expect(state.requests.comparisons).toEqual([]);
      expect(state.requests.writes).toEqual([]);
      await capture(page, info.outputPath("restored-source-context-a11y.json"));
    } finally {
      state.release();
      await setOffline(page, false);
    }
  });
}
