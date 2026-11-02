import { expect, test } from "vitest";
import { platformResponse, SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const fixture = platformResponse("/test-cases").body as {
  test_cases: Record<string, unknown>[];
};
const row = fixture.test_cases[0];
const scenarios = ["initial", "refresh", "next"].flatMap((phase) =>
  ["missing rubric observation", "unreadable row"].map((failure) => ({ phase, failure })),
);

for (const { phase, failure } of scenarios) {
  test(`test-case list ${phase} with ${failure} stays unknown and recovers`, async () => {
    const page = app();
    const paged = Array.from({ length: 51 }, (_, index) => ({
      ...row,
      test_case_id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
    }));
    const malformed = {
      test_cases: failure === "unreadable row" ? [null] : [{ ...row, has_rubric: undefined }],
    };
    const offsets: number[] = [];
    const errors: string[] = [];
    let recovered = false;
    const collectError = (event: ErrorEvent) => errors.push(event.message);
    window.addEventListener("error", collectError);
    page.route("**/test-cases?*", ({ url }) => {
      const offset = Number(url.searchParams.get("offset"));
      offsets.push(offset);
      const failed = !recovered && (phase === "initial" || offsets.length > 1);
      return {
        json: failed
          ? malformed
          : { test_cases: phase === "next" ? (offset === 0 ? paged : [paged[50]]) : [row] },
      };
    });
    try {
      await page.goto(`/lab/test-cases?skill=${SKILL}&version=${VERSION}`);
      const index = page.locator(".test-case-index");
      const rows = index.locator("[data-role=evidence] > li");
      const name = page.getByLabel("名稱", { exact: true });
      const prompt = page.getByLabel("User Prompt", { exact: true });
      const alert = index.getByRole("alert");
      if (phase !== "initial") {
        await page.eventually(() => expect(rows.count()).toBe(phase === "next" ? 50 : 1));
        await name.fill("kept new test-case name");
        await prompt.fill("kept new test-case prompt");
        await index
          .getByRole("button", { name: phase === "next" ? "載入更多" : "重新整理", exact: true })
          .click();
      }
      await page.eventually(() => {
        expect(alert.count()).toBe(1);
        expect(alert.text()).toContain(phase === "next" ? "尚未讀完" : "無法讀取測試題");
        expect(rows.count()).toBe(phase === "next" ? 50 : 0);
        expect(index.getByText("還沒有測試題。", { exact: true }).count()).toBe(0);
        expect(index.getByText(/Rubric\s*無/).count()).toBe(0);
        expect(index.getByRole("button", { name: "載入更多", exact: true }).count()).toBe(0);
      });
      if (phase === "initial") {
        await name.fill("kept new test-case name");
        await prompt.fill("kept new test-case prompt");
      }
      expect(await page.axeViolations("main")).toEqual([]);
      recovered = true;
      const retry = index.getByRole("button", { name: "再試一次", exact: true });
      await retry.focus();
      await retry.press("Enter");
      await page.eventually(() => {
        expect(rows.count()).toBe(phase === "next" ? 51 : 1);
        expect(alert.count()).toBe(0);
        expect(page.locator("#existing-test-cases-heading").isFocused()).toBe(true);
        expect(name.value()).toBe("kept new test-case name");
        expect(prompt.value()).toBe("kept new test-case prompt");
      });
      expect(offsets).toEqual(
        phase === "next" ? [0, 50, 50] : phase === "refresh" ? [0, 0, 0] : [0, 0],
      );
      expect(
        page.requests.filter(
          (request) => request.method !== "GET" && request.url.pathname.includes("/test-cases"),
        ),
      ).toEqual([]);
      expect(errors).toEqual([]);
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
    } finally {
      window.removeEventListener("error", collectError);
    }
  });
}

test("an unnamed test-case list row remains named for assistive technology", async () => {
  const page = app();
  page.route("**/test-cases?*", () => ({
    json: { test_cases: [{ ...row, name: "", skill_name: "" }] },
  }));
  await page.goto(`/lab/test-cases?version=${VERSION}`);
  const index = page.locator(".test-case-index");
  const link = index.getByRole("link", { name: "未命名的測試題", exact: true });
  await page.eventually(() => {
    expect(link.count()).toBe(1);
    expect(link.attribute("href")).toBe(`/lab/test-cases/${row.test_case_id}?version=${VERSION}`);
    expect(index.getByText("這個小工具已經不在你的清單裡（已刪除，或已下架）").count()).toBe(1);
  });
  expect(await page.axeViolations("main")).toEqual([]);
});
