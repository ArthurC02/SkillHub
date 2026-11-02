import { act } from "react";
import { expect, test } from "vitest";
import { renderedApp, type App, type Locator, type Reply } from "../../../testing/app";
import { platformResponse, TEST_CASE, VERSION } from "../../../testing/fixtures/platform";
import type { Dataset } from "../lab.service";

const app = renderedApp();

const DATASETS_URL = `**/test-cases/${TEST_CASE}/datasets`;
const DATASET_PATH = `/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`;
const SESSION_WHY = "登入狀態尚未確認；請先在頁首確認，再上傳測試資料。";

function chooseFile(input: Locator, name: string, content: string) {
  const element = input.element();
  const file = new File([content], name, { type: "text/csv" });
  Object.defineProperty(element, "files", { value: [file], configurable: true });
  Object.defineProperty(element, "value", {
    value: `C:\\fakepath\\${name}`,
    writable: true,
    configurable: true,
  });
}

async function postedFileText(body: unknown): Promise<string> {
  return await ((body as FormData).get("file") as File).text();
}

async function submitWhileIdentityStarts(page: App) {
  await expect(page.locator(".dataset-upload-picker button")).toBeEnabled();
  const button = page.locator(".dataset-upload-picker button").element() as HTMLButtonElement;
  const original = globalThis.fetch;
  let clicked = false;
  globalThis.fetch = ((input: string, init?: RequestInit) => {
    const response = original(input, init);
    if (new URL(String(input), "http://127.0.0.1").pathname === "/me") {
      globalThis.fetch = original;
      if (button.disabled) throw new Error("the identity submit probe needs an enabled button");
      button.click();
      clicked = true;
    }
    return response;
  }) as typeof fetch;
  await act(async () => {
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
  });
  globalThis.fetch = original;
  expect(clicked).toBe(true);
}

test("dataset selection puts the file picker before the upload rules and the rules before submission", async () => {
  const page = app();
  await page.goto(`/lab/test-cases/${TEST_CASE}/datasets`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  const file = page.getByLabel("選擇檔案", { exact: true });
  const guide = page.getByRole("region", { name: "上傳前請先確認", exact: true });
  const submit = page.getByRole("button", { name: "上傳", exact: true });
  await expect(submit).toBeEnabled();
  expect(
    Boolean(
      file.element().compareDocumentPosition(document.querySelector(".dataset-upload-guide")!) &
      Node.DOCUMENT_POSITION_FOLLOWING,
    ),
  ).toBe(true);
  expect(
    Boolean(
      guide
        .element()
        .compareDocumentPosition(document.querySelector(".dataset-upload-picker button")!) &
      Node.DOCUMENT_POSITION_FOLLOWING,
    ),
  ).toBe(true);
  for (const term of ["大小限制", "支援格式", "保存政策", "資料使用範圍"]) {
    await expect(guide.getByText(term, { exact: true })).toBeVisible();
  }
  expect(guide.element().querySelectorAll("details, [hidden]")).toHaveLength(0);
  expect(await page.axeViolations("body")).toEqual([]);
});

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
  test(`${transport} upload result with saved=${saved} and read failure=${failedRead} rereads evidence without replay`, async () => {
    const page = app();
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
    const uploadReply: Record<string, Reply> = {
      network: "network-error",
      server: { status: 503, json: { error: "service unavailable" } },
      malformed: { status: 201, body: "{" },
      incomplete: { status: 201, body: "{}" },
      null: { status: 201, body: "null" },
    };
    page.route(DATASETS_URL, async (request): Promise<Reply> => {
      if (request.method === "POST") {
        writes.push(await postedFileText(request.body));
        committed = saved;
        return uploadReply[transport];
      }
      reads += 1;
      if (reads === 2 && failedRead) return { status: 503, json: { error: "service unavailable" } };
      if (reads === 3) await gate;
      return { json: { datasets: committed ? [row] : [], total_bytes: committed ? 1000 : 0 } };
    });
    try {
      await page.goto(DATASET_PATH);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const upload = page.getByRole("button", { name: "上傳", exact: true });
      await expect(upload).toBeEnabled();
      const file = page.getByLabel("選擇檔案", { exact: true });
      chooseFile(file, fileName, "n".repeat(1000));
      await upload.click();
      const result = page.getByRole("region", { name: "上傳結果", exact: true });
      const message = result.getByRole("alert");
      await expect(message).toHaveText(
        "無法確認上傳結果。請先核對下方最新檔案清單，再決定是否重試。",
      );
      await expect(message).toBeFocused();
      await page.eventually(() => expect(reads).toBe(2));
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
      await page.eventually(() => expect(reads).toBe(3));
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
      expect((file.element() as HTMLInputElement).files?.[0]?.name).toBe(fileName);
      await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toHaveCount(0);
      expect(writes).toHaveLength(1);
      expect(await page.axeViolations("body")).toEqual([]);
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
    } finally {
      release();
    }
  });
}

test("an explicit upload refusal keeps its reason and does not reconcile an unknown result", async () => {
  const page = app();
  let reads = 0;
  let writes = 0;
  page.route(DATASETS_URL, (request) => {
    if (request.method === "POST") {
      writes += 1;
      return { status: 415, json: { error: "不支援這種檔案類型" } };
    }
    reads += 1;
    return undefined;
  });
  await page.goto(DATASET_PATH);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  const upload = page.getByRole("button", { name: "上傳", exact: true });
  await expect(upload).toBeEnabled();
  chooseFile(page.getByLabel("選擇檔案", { exact: true }), "rows.csv", "n".repeat(1000));
  await upload.click();
  await expect(
    page.getByRole("region", { name: "上傳結果", exact: true }).getByRole("alert"),
  ).toHaveText("不支援這種檔案類型");
  await expect(page.getByRole("button", { name: "重新確認檔案清單", exact: true })).toHaveCount(0);
  expect(writes).toBe(1);
  expect(reads).toBe(1);
});

test("an unconfirmed session disables dataset uploads without rereading capacity", async () => {
  const page = app();
  let checking = false;
  let reads = 0;
  let writes = 0;
  page.route("**/me", () =>
    checking ? { status: 503, json: { error: "service unavailable" } } : undefined,
  );
  page.route(DATASETS_URL, (request) => {
    if (request.method === "POST") {
      writes += 1;
      return { status: 500, json: { error: "unexpected write" } };
    }
    reads += 1;
    return undefined;
  });
  await page.goto(DATASET_PATH);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  const upload = page.getByRole("button", { name: "上傳", exact: true });
  await expect(upload).toBeEnabled();
  chooseFile(page.getByLabel("選擇檔案", { exact: true }), "rows.csv", "n".repeat(1000));
  checking = true;
  await act(async () => {
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
  });
  await expect(page.getByRole("button", { name: "重新確認登入狀態", exact: true })).toBeVisible();
  await expect(upload).toBeDisabled();
  await expect(upload).toHaveAccessibleDescription(SESSION_WHY);
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

for (const previous of [false, true]) {
  test(`an identity check in the dataset submit turn preserves ${previous ? "the preceding unknown upload" : "the unsent file"}`, async () => {
    const page = app();
    let checking = false;
    let release!: () => void;
    let gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: string[] = [];
    page.route("**/me", async () => {
      if (checking) await gate;
      return { json: platformResponse("/me").body };
    });
    page.route(DATASETS_URL, async (request): Promise<Reply | undefined> => {
      if (request.method !== "POST") return undefined;
      writes.push(await postedFileText(request.body));
      if (previous && writes.length === 1)
        return { status: 503, json: { error: "service unavailable" } };
      return {
        status: 201,
        json: {
          dataset_id: "99999999-1111-4111-8111-111111111111",
          file_name: "next.csv",
          size_bytes: 9,
          content_type: "text/csv",
          content_hash: "a".repeat(64),
          expires_at: "2026-11-15T00:00:00Z",
        },
      };
    });
    try {
      await page.goto(DATASET_PATH);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const upload = page.getByRole("button", { name: "上傳", exact: true });
      const input = page.getByLabel("選擇檔案", { exact: true });
      const result = page.getByRole("region", { name: "上傳結果", exact: true });
      await expect(upload).toBeEnabled();
      if (previous) {
        chooseFile(input, "first.csv", "first-data");
        await upload.click();
        await expect(result).toContainText("無法確認上傳結果");
        await expect(result).toContainText("送出檔案：first.csv");
        await expect(upload).toBeEnabled();
      }
      chooseFile(input, "next.csv", "next-data");
      checking = true;
      await submitWhileIdentityStarts(page);
      await expect(page.getByText("正在確認登入狀態…", { exact: true })).toBeVisible();
      const blocked = page
        .getByRole("alert")
        .filter({ hasText: "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。" });
      await expect(blocked).toBeFocused();
      await expect(upload).toBeDisabled();
      await expect(upload).toHaveAccessibleDescription(SESSION_WHY);
      if (previous) {
        await expect(result).toContainText("無法確認上傳結果");
        await expect(result).toContainText("送出檔案：first.csv");
        await expect(result).not.toContainText("送出檔案：next.csv");
      } else await expect(result).toHaveCount(0);
      expect(writes).toEqual(previous ? ["first-data"] : []);
      expect(await page.axeViolations("main")).toEqual([]);
      checking = false;
      release();
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(upload).toBeEnabled();
      const kept = (input.element() as HTMLInputElement).files?.[0];
      expect({ name: kept?.name, content: await kept?.text() }).toEqual({
        name: "next.csv",
        content: "next-data",
      });
      expect(writes).toEqual(previous ? ["first-data"] : []);
      await upload.click();
      await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toContainText(
        "已上傳 next.csv（9 B）",
      );
      await expect(result).toHaveCount(0);
      expect(writes).toEqual(previous ? ["first-data", "next-data"] : ["next-data"]);
      chooseFile(input, "third.csv", "third-data");
      gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      checking = true;
      const profileReads = page.requests.filter((r) => r.url.pathname === "/me").length;
      await submitWhileIdentityStarts(page);
      await expect(page.getByText("正在確認登入狀態…", { exact: true })).toBeVisible();
      await expect(blocked).toBeFocused();
      await expect(result).toHaveCount(0);
      await expect(page.getByRole("status", { name: "上傳完成", exact: true })).toContainText(
        "next.csv",
      );
      expect(writes).toEqual(previous ? ["first-data", "next-data"] : ["next-data"]);
      checking = false;
      release();
      await page.eventually(() =>
        expect(page.requests.filter((r) => r.url.pathname === "/me").length).toBeGreaterThan(
          profileReads,
        ),
      );
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(upload).toBeEnabled();
    } finally {
      release();
    }
  });
}
