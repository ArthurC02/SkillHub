import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createServer } from "node:http";
import { writeFile } from "node:fs/promises";
import type { AddressInfo } from "node:net";
import { TEST_CASE, VERSION, platformResponse } from "../src/testing/fixtures/platform";
import type { Dataset } from "../src/features/lab/lab.service";
import { stubPlatform } from "./stub";

function uploadReceiver(transport: string, onBody: (body: string) => void) {
  return createServer(async (request, response) => {
    const headers = {
      "Access-Control-Allow-Origin": request.headers.origin ?? "http://localhost:4173",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Allow-Headers": "accept, content-type",
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
    const body: Record<string, string> = {
      server: JSON.stringify({ error: "service unavailable" }),
      malformed: "{",
      incomplete: "{}",
      null: "null",
    };
    response.writeHead(transport === "server" ? 503 : 201, headers).end(body[transport]);
  });
}

for (const [transport, saved, failedRead] of [
  ["network", false, false],
  ["network", true, false],
  ["server", false, false],
  ["server", true, false],
  ["network", true, true],
  ["server", true, true],
  ["malformed", true, false],
  ["incomplete", true, false],
  ["null", true, false],
] as const) {
  test(`${transport} upload result with saved=${saved} and read failure=${failedRead} rereads evidence without replay`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    const fileName =
      transport === "server" && saved && !failedRead ? "x".repeat(220) + ".csv" : "rows.csv";
    const row: Dataset = {
      dataset_id: "99999999-1111-4111-8111-111111111111",
      file_name: fileName,
      size_bytes: 1000,
      content_type: "text/csv",
      content_hash: "a".repeat(64),
      expires_at: "2026-11-15T00:00:00Z",
    };
    const writes: string[] = [];
    let committed = false;
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const receiver = uploadReceiver(transport, (body) => {
      writes.push(body);
      committed = saved;
    });
    await new Promise<void>((resolve, reject) => {
      receiver.once("error", reject);
      receiver.listen(0, "127.0.0.1", resolve);
    });
    try {
      const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/datasets`;
      await page.route(`**/test-cases/${TEST_CASE}/datasets`, async (route) => {
        if (route.request().resourceType() === "document") return route.fallback();
        if (route.request().method() === "POST") return route.continue({ url: endpoint });
        reads += 1;
        if (reads === 2 && failedRead)
          return route.fulfill({ status: 503, json: { error: "service unavailable" } });
        if (reads === 3) await gate;
        return route.fulfill({
          json: { datasets: committed ? [row] : [], total_bytes: committed ? 1000 : 0 },
        });
      });
      await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const upload = page.getByRole("button", { name: "上傳", exact: true });
      await expect(upload).toBeEnabled();
      const file = page.getByLabel("選擇檔案", { exact: true });
      await file.setInputFiles({
        name: fileName,
        mimeType: "text/csv",
        buffer: Buffer.alloc(1000, "n"),
      });
      await upload.click();
      const result = page.getByRole("region", { name: "上傳結果", exact: true });
      const message = result.getByRole("alert");
      await expect(message).toHaveText(
        "無法確認上傳結果。請先核對下方最新檔案清單，再決定是否重試。",
      );
      await expect(message).toBeFocused();
      await expect.poll(() => reads).toBe(2);
      expect(writes).toHaveLength(1);
      expect(writes[0]).toContain("n".repeat(1000));
      await expect(result.getByText(`送出檔案：${fileName}`, { exact: true })).toBeVisible();
      if (failedRead) {
        await expect(result.getByRole("status")).toHaveText("檔案清單尚未確認。");
        await expect(upload).toBeDisabled();
      } else if (saved)
        await expect(result.locator(".file-tree")).toContainText(`${fileName}（1000 B）`);
      else await expect(result.getByRole("status")).toHaveText("目前清單沒有檔案。");
      const retry = result.getByRole("button", { name: "重新確認檔案清單", exact: true });
      await retry.click();
      await expect.poll(() => reads).toBe(3);
      await expect(result.getByRole("heading")).toBeFocused();
      await expect(retry).toBeDisabled();
      await expect(result.getByRole("status")).toHaveText("檔案清單尚未確認。");
      await expect(result.locator(".file-tree")).toHaveCount(0);
      expect(writes).toHaveLength(1);
      release();
      await expect(retry).toBeEnabled();
      await expect(upload).toBeEnabled();
      if (saved) await expect(result.locator(".file-tree")).toContainText(`${fileName}（1000 B）`);
      else await expect(result.getByRole("status")).toHaveText("目前清單沒有檔案。");
      expect(await file.evaluate((input: HTMLInputElement) => input.files?.[0]?.name)).toBe(
        fileName,
      );
      await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toHaveCount(0);
      expect(writes).toHaveLength(1);
      expect(await result.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
        true,
      );
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      await message.scrollIntoViewIfNeeded();
      await page.screenshot({ path: test.info().outputPath("dataset-unknown-result.png") });
      expect(new URL(page.url()).searchParams.get("version")).toBe(VERSION);
    } finally {
      release();
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}

test("an explicit upload refusal keeps its reason and does not reconcile an unknown result", async ({
  page,
}) => {
  await stubPlatform(page);
  let reads = 0;
  let writes = 0;
  await page.route(`**/test-cases/${TEST_CASE}/datasets`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    if (route.request().method() === "POST") {
      writes += 1;
      return route.fulfill({ status: 415, json: { error: "不支援這種檔案類型" } });
    }
    reads += 1;
    return route.fulfill({ json: platformResponse(route.request().url()).body });
  });
  await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  const upload = page.getByRole("button", { name: "上傳", exact: true });
  await expect(upload).toBeEnabled();
  await page
    .getByLabel("選擇檔案", { exact: true })
    .setInputFiles({ name: "rows.csv", mimeType: "text/csv", buffer: Buffer.alloc(1000, "n") });
  await upload.click();
  await expect(
    page.getByRole("region", { name: "上傳結果", exact: true }).getByRole("alert"),
  ).toHaveText("不支援這種檔案類型");
  await expect(page.getByRole("button", { name: "重新確認檔案清單", exact: true })).toHaveCount(0);
  expect(writes).toBe(1);
  expect(reads).toBe(1);
});

test("an unconfirmed session disables dataset uploads without rereading capacity", async ({
  page,
}) => {
  await stubPlatform(page);
  let checking = false;
  let reads = 0;
  let writes = 0;
  await page.route("**/me", (route) =>
    checking
      ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
      : route.fulfill({ json: platformResponse("/me").body }),
  );
  await page.route(`**/test-cases/${TEST_CASE}/datasets`, (route) => {
    if (route.request().resourceType() === "document") return route.fallback();
    if (route.request().method() === "POST") {
      writes += 1;
      return route.fulfill({ status: 500, json: { error: "unexpected write" } });
    }
    reads += 1;
    return route.fulfill({ json: platformResponse(route.request().url()).body });
  });
  await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  const upload = page.getByRole("button", { name: "上傳", exact: true });
  await expect(upload).toBeEnabled();
  await page
    .getByLabel("選擇檔案", { exact: true })
    .setInputFiles({ name: "rows.csv", mimeType: "text/csv", buffer: Buffer.alloc(1000, "n") });
  checking = true;
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
  );
  await expect(page.getByRole("button", { name: "重新確認登入狀態", exact: true })).toBeVisible();
  await expect(upload).toBeDisabled();
  await expect(upload).toHaveAccessibleDescription(
    "登入狀態尚未確認；請先在頁首確認，再上傳測試資料。",
  );
  await expect(page.getByRole("button", { name: "重新確認檔案清單", exact: true })).toHaveCount(0);
  expect(writes).toBe(0);
  expect(reads).toBe(1);
  checking = false;
  await page.getByRole("button", { name: "重新確認登入狀態", exact: true }).click();
  await expect(upload).toBeEnabled();
  await expect(page.getByLabel("選擇檔案", { exact: true })).toHaveValue(/rows\.csv$/);
  expect(writes).toBe(0);
  expect(reads).toBe(1);
});

async function datasetSessionReceiver(previous: boolean, writes: string[]) {
  const receiver = createServer(async (request, response) => {
    response.setHeader(
      "Access-Control-Allow-Origin",
      request.headers.origin ?? "http://localhost:4173",
    );
    response.setHeader("Access-Control-Allow-Credentials", "true");
    response.setHeader("Access-Control-Allow-Headers", "accept, content-type");
    if (request.method === "OPTIONS") {
      response.writeHead(204).end();
      return;
    }
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const form = await new Response(Buffer.concat(chunks).toString("utf8"), {
      headers: { "Content-Type": request.headers["content-type"]! },
    }).formData();
    writes.push(await (form.get("file") as File).text());
    response.setHeader("Content-Type", "application/json");
    if (previous && writes.length === 1) {
      response.writeHead(503).end(JSON.stringify({ error: "service unavailable" }));
      return;
    }
    response.writeHead(201).end(
      JSON.stringify({
        dataset_id: "99999999-1111-4111-8111-111111111111",
        file_name: "next.csv",
        size_bytes: 9,
        content_type: "text/csv",
        content_hash: "a".repeat(64),
        expires_at: "2026-11-15T00:00:00Z",
      }),
    );
  });
  await new Promise<void>((resolve, reject) => {
    receiver.once("error", reject);
    receiver.listen(0, "127.0.0.1", resolve);
  });
  return receiver;
}

function submitWhileIdentityStarts(page: Page) {
  return page.evaluate(async () => {
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    document.querySelector<HTMLButtonElement>(".dataset-upload-picker button")!.click();
  });
}

for (const previous of [false, true]) {
  test(`an identity check in the dataset submit turn preserves ${previous ? "the preceding unknown upload" : "the unsent file"}`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let checking = false;
    let release!: () => void;
    let gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: string[] = [];
    await page.route("**/me", async (route) => {
      if (checking) await gate;
      return route.fulfill({ json: platformResponse("/me").body });
    });
    const receiver = await datasetSessionReceiver(previous, writes);
    try {
      const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/datasets`;
      await page.route(`**/test-cases/${TEST_CASE}/datasets`, (route) =>
        route.request().method() === "POST" ? route.continue({ url: endpoint }) : route.fallback(),
      );
      await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const upload = page.getByRole("button", { name: "上傳", exact: true });
      const input = page.getByLabel("選擇檔案", { exact: true });
      const result = page.getByRole("region", { name: "上傳結果", exact: true });
      await expect(upload).toBeEnabled();
      if (previous) {
        await input.setInputFiles({
          name: "first.csv",
          mimeType: "text/csv",
          buffer: Buffer.from("first-data"),
        });
        await upload.click();
        await expect(result).toContainText("無法確認上傳結果");
        await expect(result).toContainText("送出檔案：first.csv");
        await expect(upload).toBeEnabled();
      }
      await input.setInputFiles({
        name: "next.csv",
        mimeType: "text/csv",
        buffer: Buffer.from("next-data"),
      });
      checking = true;
      await submitWhileIdentityStarts(page);
      await expect(page.getByText("正在確認登入狀態…", { exact: true })).toBeVisible();
      const blocked = page
        .getByRole("alert")
        .filter({ hasText: "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。" });
      await expect(blocked).toBeFocused();
      await expect(upload).toBeDisabled();
      await expect(upload).toHaveAccessibleDescription(
        "登入狀態尚未確認；請先在頁首確認，再上傳測試資料。",
      );
      if (previous) {
        await expect(result).toContainText("無法確認上傳結果");
        await expect(result).toContainText("送出檔案：first.csv");
        await expect(result).not.toContainText("送出檔案：next.csv");
      } else await expect(result).toHaveCount(0);
      expect(writes).toEqual(previous ? ["first-data"] : []);
      await page.evaluate(() => scrollTo(0, 0));
      const report = await new AxeBuilder({ page }).include("main").analyze();
      await writeFile(test.info().outputPath("dataset-session-a11y.json"), JSON.stringify(report));
      expect(report.violations).toEqual([]);
      expect(report.incomplete).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.screenshot({
        path: test.info().outputPath("dataset-session-phone.png"),
        fullPage: true,
      });
      checking = false;
      release();
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(upload).toBeEnabled();
      expect(
        await input.evaluate(async (element: HTMLInputElement) => ({
          name: element.files?.[0]?.name,
          content: await element.files?.[0]?.text(),
        })),
      ).toEqual({ name: "next.csv", content: "next-data" });
      expect(writes).toEqual(previous ? ["first-data"] : []);
      await upload.click();
      await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toContainText(
        "已上傳 next.csv（9 B）",
      );
      await expect(result).toHaveCount(0);
      expect(writes).toEqual(previous ? ["first-data", "next-data"] : ["next-data"]);
      await input.setInputFiles({
        name: "third.csv",
        mimeType: "text/csv",
        buffer: Buffer.from("third-data"),
      });
      gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      checking = true;
      await submitWhileIdentityStarts(page);
      await expect(blocked).toBeFocused();
      await expect(result).toHaveCount(0);
      await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toContainText(
        "next.csv",
      );
      expect(writes).toEqual(previous ? ["first-data", "next-data"] : ["next-data"]);
    } finally {
      release();
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}
