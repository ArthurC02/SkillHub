import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

const TEST_CASE = "33333333-3333-3333-3333-333333333333";

const LIMITS = {
  max_file_bytes: 25 << 20,
  max_test_case_bytes: 100 << 20,
  max_files_per_test_case: 20,
  retention_days: 90,
  allowed_kinds: [
    "文字檔（.txt .md .csv .tsv .json .jsonl .xml .yaml .yml）",
    "文件（.pdf .docx .xlsx .pptx）",
  ],
  note: "檔案類型看內容判斷，不看副檔名；上傳的檔案只有這個 Test Case 的 Run 讀得到，到保存期限或你刪除時就會刪掉。",
};

function stubPlatform(limitsStatus = 200) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls.push(url);
    if (url.includes("/test-cases/limits")) {
      return Promise.resolve(
        new Response(limitsStatus === 200 ? JSON.stringify(LIMITS) : `{"error":"nope"}`, {
          status: limitsStatus,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    return Promise.resolve(new Response(`{"error":"not found"}`, { status: 404 }));
  });
  return calls;
}

async function renderUpload() {
  window.history.pushState({}, "", `/lab/datasets?test_case=${TEST_CASE}`);
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
  await act(async () => {
    await router.navigate({ to: "/lab/datasets", search: { test_case: TEST_CASE } });
  });
}

test("02:TEST-002 the upload rules are on screen before anything is uploaded", async () => {
  const calls = stubPlatform();
  await renderUpload();

  const text = container.textContent ?? "";
  expect(text).toContain("25 MB");
  expect(text).toContain("100 MB");
  expect(text).toContain("20");
  expect(text).toContain("90");
  expect(text).toContain("資料使用範圍");
  expect(text).toContain("Secrets");
  expect(text).toContain("副檔名");

  expect(calls.some((u) => u.includes("/test-cases/limits"))).toBe(true);
  expect(calls.some((u) => u.includes("/datasets"))).toBe(false);
  expect(container.querySelector("input[type=file]")).not.toBeNull();
});

test("02:TEST-002 without the rules there is no upload control at all", async () => {
  stubPlatform(500);
  await renderUpload();

  expect(container.textContent).toContain("無法讀取上傳規則");
  expect(container.querySelector("input[type=file]")).toBeNull();
});

test("丙-150(e) a 415 upload failure prints the server's own Chinese sentence", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/test-cases/limits")) {
      return Promise.resolve(
        new Response(JSON.stringify(LIMITS), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "不支援這種檔案類型" }), {
        status: 415,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  await renderUpload();

  const input = container.querySelector<HTMLInputElement>("input[type=file]")!;
  const file = new File(["bogus"], "a.exe", { type: "application/octet-stream" });
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  await act(async () => uploadButton().click());
  await waitFor(() => (container.textContent ?? "").includes("不支援這種檔案類型"));

  expect(container.textContent).not.toContain("上傳沒有成功");
});

test("丙-150(e) a 500 upload failure falls back to the generic retry sentence", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    if (url.includes("/test-cases/limits")) {
      return Promise.resolve(
        new Response(JSON.stringify(LIMITS), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "internal error, do not show this" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  await renderUpload();

  const input = container.querySelector<HTMLInputElement>("input[type=file]")!;
  const file = new File(["bogus"], "a.txt", { type: "text/plain" });
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  await act(async () => uploadButton().click());
  await waitFor(() => (container.textContent ?? "").includes("上傳沒有成功，可以再按一次。"));

  expect(container.textContent).not.toContain("internal error, do not show this");
});

test("02:TEST-002 changing the Test Case clears what was uploaded to the previous one", async () => {
  const OTHER = "44444444-4444-4444-4444-444444444444";
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/test-cases/limits")) {
      return Promise.resolve(
        new Response(JSON.stringify(LIMITS), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    if (init?.method === "POST") {
      return Promise.resolve(
        new Response(JSON.stringify({ dataset_id: "d-1", file_name: "a.csv", size_bytes: 10 }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }
    return Promise.resolve(new Response(`{"error":"not found"}`, { status: 404 }));
  });
  await renderUpload();

  const input = container.querySelector<HTMLInputElement>("input[type=file]")!;
  const file = new File(["a,b\n"], "a.csv", { type: "text/csv" });
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  await act(async () => uploadButton().click());
  await waitFor(() => (container.textContent ?? "").includes("已上傳 a.csv"));

  Object.defineProperty(input, "files", { value: [], configurable: true });
  await act(async () => uploadButton().click());
  expect(container.textContent).toContain("請先選擇一個檔案");
  expect(container.textContent).toContain("已上傳 a.csv");

  await act(async () => {
    await router.navigate({ to: "/lab/datasets", search: { test_case: OTHER } });
  });

  expect(container.textContent).not.toContain("已上傳 a.csv");
  expect(container.textContent).not.toContain("請先選擇一個檔案");
});

function uploadButton(): HTMLButtonElement {
  return Array.from(container.querySelectorAll("button")).find((b) => b.textContent === "上傳")!;
}

async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}
