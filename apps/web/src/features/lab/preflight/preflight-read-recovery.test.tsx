import { expect, test } from "vitest";
import {
  RUN,
  SKILL,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

for (const [owner, endpoint, sentence] of [
  ["skill", `/api/skills/${SKILL}`, "暫時無法讀取這個小工具"],
  ["test case", `/test-cases/${TEST_CASE}`, "暫時無法讀取測試題"],
  ["summary", `/skills/${SKILL}/runs/preflight`, "暫時無法讀取權限摘要"],
] as const) {
  test(`preflight recovers its unreadable ${owner} without starting a Run or losing context`, async () => {
    const page = app();
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(
      (url) => url.pathname === endpoint,
      async ({ url, method }) => {
        if (method !== "GET") return undefined;
        reads += 1;
        if (reads === 1) return { status: 503, json: { error: "service unavailable" } };
        await gate;
        return { json: platformResponse(url.pathname + url.search).body };
      },
    );
    const writes = () => page.requests.filter((request) => request.method === "POST");
    try {
      await page.goto(
        `/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}&against=${RUN}`,
      );
      const main = () => page.locator("main");
      await page.eventually(() => expect(page.getByRole("alert").text()).toContain(sentence));
      expect(reads).toBe(1);
      const retry = page.getByRole("button", { name: "再試一次", exact: true });
      expect(retry.count()).toBe(1);
      await retry.focus();
      await page.press("Enter");
      await page.eventually(() => expect(reads).toBe(2));
      const heading = page.getByRole("heading", { name: "執行前權限確認", exact: true });
      await page.eventually(() => expect(heading.isFocused()).toBe(true));
      expect(page.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(0);
      expect(writes()).toEqual([]);
      release();
      await page.eventually(() => expect(page.getByRole("alert").count()).toBe(0));
      const context = main().locator(".preflight-context");
      await page.eventually(() => {
        expect(context.text()).toContain("PDF Summariser");
        expect(context.text()).toContain("去重複列");
      });
      expect(page.getByLabel("小工具 Version", { exact: true }).value()).toBe(VERSION);
      const start = page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true });
      await page.eventually(() => expect(start.isDisabled()).toBe(false));
      expect(heading.isFocused()).toBe(true);
      expect(reads).toBe(2);
      expect(writes()).toEqual([]);
      const search = new URLSearchParams(page.search());
      expect(search.get("version")).toBe(VERSION);
      expect(search.get("against")).toBe(RUN);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
    }
  });
}
