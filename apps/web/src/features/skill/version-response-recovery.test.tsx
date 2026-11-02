import { afterAll, beforeAll, expect, test } from "vitest";
import { SKILL, VERSION, TEST_CASE, RUN, SKILL_VERSIONS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});

afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

for (const [name, body] of [
  ["null list", { versions: null }],
  ["null item", { versions: [null] }],
  ["wrong order", { versions: [...SKILL_VERSIONS.versions].reverse() }],
] as const) {
  test(`a ${name} version response remains unreadable and recovers without claiming latest or no access`, async () => {
    const page = app();
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const cached = name === "wrong order";
    page.route(`**/skills/${SKILL}/versions`, async () => {
      reads += 1;
      if (cached && reads === 1) return { json: SKILL_VERSIONS };
      if (reads === (cached ? 2 : 1)) return { json: body };
      await gate;
      return { json: SKILL_VERSIONS };
    });
    try {
      await page.goto(`/skills/${SKILL}`);
      const history = page.getByRole("region", { name: "版本", exact: true });
      const heading = page.locator("#version-history-heading");
      if (cached) {
        await page.eventually(() => expect(history.text()).toContain("共 2 版，最新 v2"));
        await history.getByRole("button", { name: "重新整理", exact: true }).click();
      }
      await page.eventually(() => {
        expect(history.getByRole("alert").text()).toContain("暫時無法讀取版本歷史");
        expect(history.text()).not.toContain("最新");
        expect(history.text()).not.toContain("無權檢視");
      });
      await history.getByRole("button", { name: "再試一次", exact: true }).focus();
      await page.press("Enter");
      await page.eventually(() => {
        expect(reads).toBe(cached ? 3 : 2);
        expect(heading.isFocused()).toBe(true);
        expect(history.getByRole("status").text()).toContain("載入版本歷史中");
      });
      release();
      await page.eventually(() => {
        expect(history.text()).toContain("共 2 版，最新 v2");
        expect(history.getByRole("alert").count()).toBe(0);
        expect(heading.isFocused()).toBe(true);
      });
      expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
    }
  });
}

test("an invalid version number blocks preflight without losing the selected version or starting a Run", async () => {
  const page = app();
  let reads = 0;
  const summaries = () =>
    page.requests.filter((request) => request.url.pathname.endsWith("/runs/preflight")).length;
  page.route(`**/skills/${SKILL}/versions`, () => {
    reads += 1;
    return {
      json:
        reads === 1
          ? { versions: [{ ...SKILL_VERSIONS.versions[0], version_number: "2" }] }
          : SKILL_VERSIONS,
    };
  });
  await page.goto(
    `/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}&against=${RUN}`,
  );
  const picker = page.getByRole("group", { name: "小工具版本選擇", exact: true });
  await page.eventually(() => {
    expect(picker.getByRole("alert").text()).toContain("暫時無法讀取版本清單");
    expect(picker.getByRole("combobox").value()).toBe(VERSION);
    expect(picker.getByRole("combobox").isDisabled()).toBe(true);
  });
  expect(summaries()).toBe(0);
  await picker.getByRole("button", { name: "再試一次", exact: true }).click();
  await page.eventually(() => {
    expect(
      page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }).isDisabled(),
    ).toBe(false);
    expect(picker.getByRole("combobox").value()).toBe(VERSION);
    expect(picker.isFocused()).toBe(true);
  });
  expect(new URLSearchParams(page.search()).get("against")).toBe(RUN);
  expect(reads).toBe(2);
  expect(summaries()).toBe(1);
  expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
});

test("an incomplete embedded version list preserves the Bundle draft and recovers member choices without writing", async () => {
  const page = app();
  let reads = 0;
  page.route("**/me/bundles", () => ({ json: { bundles: [] } }));
  page.route(`**/skills/${SKILL}/versions`, () => {
    reads += 1;
    return { json: reads === 1 ? {} : SKILL_VERSIONS };
  });
  await page.goto(`/workspace/downloads?bundleVersion=${VERSION}`);
  const section = page.locator("#bundle-workspace");
  await page.eventually(() =>
    expect(section.getByRole("alert").text()).toContain("可加入 Bundle 的版本"),
  );
  const form = section.locator(".bundle-form");
  await form.getByLabel("名稱", { exact: true }).fill("draft-bundle");
  await form.getByLabel("說明", { exact: true }).fill("保留這段說明");
  await section.getByRole("button", { name: "再試一次", exact: true }).click();
  await page.eventually(() => {
    expect(section.getByRole("alert").count()).toBe(0);
    expect(form.getByLabel("名稱", { exact: true }).value()).toBe("draft-bundle");
    expect(form.getByLabel("說明", { exact: true }).value()).toBe("保留這段說明");
    expect(form.locator(`[data-bundle-version="${VERSION}"]`).value()).toBe(VERSION);
  });
  expect(reads).toBe(2);
  expect(page.requests.filter((request) => request.method !== "GET")).toEqual([]);
});
