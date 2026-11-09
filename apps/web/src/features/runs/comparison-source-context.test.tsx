import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  OTHER_RUN,
  RUN,
  RUNS,
  SKILL,
  SKILL_VERSIONS,
  platformResponse,
} from "../../testing/fixtures/platform";
import { renderedApp, type App } from "../../testing/app";

const app = renderedApp();
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const SOURCE_VERSION = "abcdefab-1111-4111-8111-111111111111";
const MISSING_VERSION = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const SOURCE_LABEL = "比較基準：v1";

function setup(
  page: App,
  versionId = SOURCE_VERSION,
  empty = false,
  dependency: "self" | "versions" = "versions",
) {
  const control = { phase: "ready", reads: [] as string[] };
  const requests = { self: 0, versions: 0, candidates: 0 };
  let release!: () => void;
  let started!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  const heldStarted = new Promise<void>((resolve) => (started = resolve));
  const versions = SKILL_VERSIONS.versions.map((version) =>
    version.version_number === 1 ? { ...version, version_id: SOURCE_VERSION } : version,
  );
  async function respond(kind: "self" | "versions", body: unknown) {
    requests[kind] += 1;
    const phase = kind === dependency ? control.phase : "ready";
    if (kind === dependency) control.reads.push(phase);
    if (phase === "error") return { status: 503, json: { error: "service unavailable" } };
    if (phase === "held") {
      started();
      await held;
    }
    return { json: body };
  }
  page.route(`**/runs/${RUN}`, () =>
    respond("self", {
      ...(platformResponse(`/runs/${RUN}`).body as object),
      skill_version_id: versionId,
    }),
  );
  page.route(`**/skills/${SKILL}/versions`, () => respond("versions", { versions }));
  page.route("**/runs?*", () => {
    requests.candidates += 1;
    return { json: { runs: empty ? [] : RUNS.runs.filter((run) => run.run_id === OTHER_RUN) } };
  });
  return { control, requests, release, heldStarted };
}

const comparisons = (page: App) =>
  page.requests.filter((request) => request.url.pathname.endsWith("/comparison"));
const writes = (page: App) => page.requests.filter((request) => request.method !== "GET");

for (const { mode, expected } of [
  { mode: "canonical", expected: SOURCE_LABEL },
  { mode: "uppercase", expected: SOURCE_LABEL },
  { mode: "missing", expected: "比較基準：版本編號未測量" },
  { mode: "error", expected: "比較基準：版本編號測量失敗" },
  { mode: "empty", expected: SOURCE_LABEL },
] as const) {
  test(`source Version context is truthful for ${mode} metadata`, async () => {
    const page = app();
    const id =
      mode === "missing"
        ? MISSING_VERSION
        : mode === "uppercase"
          ? SOURCE_VERSION.toUpperCase()
          : SOURCE_VERSION;
    const state = setup(page, id, mode === "empty");
    if (mode === "error") state.control.phase = "error";
    await page.goto(`/runs/${RUN}/compare`);
    const context = page.getByText(/^比較基準：/);
    await expect(context).toHaveText(expected);
    await expect(page.getByRole("link", { name: "版本與發佈", exact: true })).toHaveAttribute(
      "href",
      `/skills/${SKILL}/versions/${id}`,
    );
    const rows = page.locator("main .download-list > .download-item");
    await expect(rows).toHaveCount(mode === "empty" ? 0 : 1);
    if (mode !== "error" && mode !== "empty") {
      await expect(rows.getByRole("link", { name: "v2", exact: true })).toBeVisible();
      await expect(rows.getByRole("button")).toBeVisible();
    }
    expect(state.requests.versions).toBe(1);
    expect(state.requests.self).toBe(1);
    expect(state.requests.candidates).toBe(1);
    await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
    const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
    await field.fill("辨識基準時保留的比較 ID");
    await expect(field).toBeFocused();
    expect(await page.axeViolations("body")).toEqual([]);
    if (mode === "error") {
      state.control.phase = "ready";
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      await expect(context).toHaveText(SOURCE_LABEL);
      await expect(field).toHaveValue("辨識基準時保留的比較 ID");
      expect(state.control.reads).toEqual(["error", "ready"]);
      expect(await page.axeViolations("body")).toEqual([]);
    }
    expect(comparisons(page)).toEqual([]);
    expect(writes(page)).toEqual([]);
  });
}

for (const mode of ["pending", "paused", "source-error"] as const) {
  test(`cached source Version context handles ${mode} without losing the comparison draft`, async () => {
    const page = app();
    const self = mode === "source-error";
    const paused = mode === "paused";
    const state = setup(page, SOURCE_VERSION, false, self ? "self" : "versions");
    const context = page.getByText(/^比較基準：/);
    try {
      await page.goto(`/runs/${RUN}/compare`);
      await expect(context).toHaveText(SOURCE_LABEL);
      await page
        .getByRole("navigation", { name: "這個小工具的工作台", exact: true })
        .getByRole("link", { name: "總覽", exact: true })
        .click();
      await expect(page.locator("h1")).not.toHaveText("試跑比較");
      await expect(
        page.getByRole("region", { name: "版本", exact: true }).getByText(/共\s*2\s*版，最新 v2/),
      ).toBeVisible();
      state.control.phase = "error";
      await page.goBack();
      await expect(page.getByRole("alert")).toContainText(
        self ? "暫時無法讀取目前這次試跑" : "暫時無法讀取候選試跑紀錄的 Version 編號",
      );
      if (self) await expect(context).toHaveCount(0);
      else await expect(context).toHaveText("比較基準：版本編號測量失敗");
      await page.getByText("直接輸入試跑紀錄 ID", { exact: true }).click();
      const field = page.getByLabel("要比較的另一個試跑紀錄 ID");
      await field.fill("恢復基準資訊前保留的比較 ID");
      expect(await page.axeViolations("body")).toEqual([]);
      state.control.phase = "held";
      if (paused) await page.setOffline(true);
      await page.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      if (self) await expect(context).toHaveCount(0);
      else
        await expect(context).toHaveText(
          paused ? "比較基準：版本編號待連線" : "比較基準：版本編號載入中",
        );
      await expect(page.getByRole("alert")).toHaveCount(0);
      await field.fill("恢復基準資訊時繼續輸入的比較 ID");
      expect(await page.axeViolations("body")).toEqual([]);
      if (paused) {
        expect(state.control.reads).not.toContain("held");
        await page.setOffline(false);
      }
      await state.heldStarted;
      if (!self) await expect(context).toHaveText("比較基準：版本編號載入中");
      expect(state.control.reads.filter((phase) => phase === "held")).toHaveLength(1);
      state.release();
      await expect(context).toHaveText(SOURCE_LABEL);
      await expect(field).toHaveValue("恢復基準資訊時繼續輸入的比較 ID");
      await expect(field).toBeFocused();
      expect(comparisons(page)).toEqual([]);
      expect(writes(page)).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      state.release();
      await page.setOffline(false);
    }
  });
}
