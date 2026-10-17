import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { TEST_CASE, VERSION } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";
import type { Dataset } from "../src/features/lab/lab.service";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";

for (const scenario of [
  { name: "clears its unchanged file selection", replacement: undefined },
  { name: "retains a differently named next file", replacement: "next.csv" },
  { name: "retains a same-name next file with new content", replacement: "report.csv" },
]) {
  test(`a completed dataset upload ${scenario.name}`, async ({ page }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let release!: () => void;
    let started!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const readStarted = new Promise<void>((resolve) => {
      started = resolve;
    });
    const writes: string[] = [];
    const receiver = createServer(async (request, response) => {
      response.setHeader(
        "Access-Control-Allow-Origin",
        request.headers.origin ?? "http://127.0.0.1:4173",
      );
      response.setHeader("Access-Control-Allow-Credentials", "true");
      response.setHeader("Access-Control-Allow-Headers", "accept, content-type");
      if (request.method === "OPTIONS") {
        response.writeHead(204).end();
        return;
      }
      const chunks: Buffer[] = [];
      for await (const chunk of request) chunks.push(Buffer.from(chunk));
      writes.push(Buffer.concat(chunks).toString("utf8"));
      const number = writes.length;
      if (number === 1) {
        started();
        await gate;
      }
      response.writeHead(201, { "Content-Type": "application/json" });
      response.end(
        JSON.stringify({
          dataset_id: `00000000-0000-4000-8000-${String(number).padStart(12, "0")}`,
          file_name: number === 1 ? "report.csv" : (scenario.replacement ?? "report.csv"),
          size_bytes: number === 1 ? 10 : 11,
          content_type: "text/csv",
          content_hash: "a".repeat(64),
          expires_at: "2026-11-15T00:00:00Z",
        } satisfies Dataset),
      );
    });
    await new Promise<void>((resolve, reject) => {
      receiver.once("error", reject);
      receiver.listen(0, "127.0.0.1", resolve);
    });
    try {
      const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/test-cases/${TEST_CASE}/datasets`;
      await page.route(`**/test-cases/${TEST_CASE}/datasets`, (route) =>
        route.request().method() === "POST" ? route.continue({ url: endpoint }) : route.fallback(),
      );
      await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const file = page.getByLabel("選擇檔案", { exact: true });
      const upload = page.getByRole("button", { name: "上傳", exact: true });
      await expect(upload).toBeEnabled();
      await file.setInputFiles({
        name: "report.csv",
        mimeType: "text/csv",
        buffer: Buffer.from("first-data"),
      });
      await upload.click();
      await readStarted;
      await expect(page.getByRole("button", { name: "上傳中…", exact: true })).toBeDisabled();
      if (scenario.replacement)
        await file.setInputFiles({
          name: scenario.replacement,
          mimeType: "text/csv",
          buffer: Buffer.from("second-data"),
        });
      release();
      const completed = page.getByRole("status", { name: "上傳完成", exact: true });
      await expect(completed).toContainText("已上傳 report.csv（10 B）");
      await expect(upload).toBeEnabled();
      expect(writes).toHaveLength(1);
      expect(writes[0]).toContain("first-data");
      if (scenario.replacement) {
        expect(
          await file.evaluate((input: HTMLInputElement) => ({
            name: input.files?.[0]?.name,
            size: input.files?.[0]?.size,
          })),
        ).toEqual({ name: scenario.replacement, size: 11 });
        await page.screenshot({ path: test.info().outputPath("dataset-next-file.png") });
        expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
        expect(writes).toHaveLength(1);
        await upload.click();
        await expect(completed.locator("li")).toHaveCount(2);
        expect(writes).toHaveLength(2);
        expect(writes[1]).toContain("second-data");
      }
      await expect(file).toHaveValue("");
      expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
    } finally {
      release();
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}
