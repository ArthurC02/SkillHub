import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";

// 02:TEST-002 第 2 條「上傳前顯示大小限制、保存政策及資料使用範圍」. Same hand-rolled
// DOM plumbing as lab.test.tsx — @testing-library is not a dependency of this app.

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
  allowed_kinds: ["text (.txt .md .csv)", "documents (.pdf .docx)"],
  note: "file type is decided by content, not by file extension",
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
  // Size limits, straight from GET /test-cases/limits rather than from a copy in
  // the UI — the published number and the enforced number are the same number.
  expect(text).toContain("25 MB");
  expect(text).toContain("100 MB");
  expect(text).toContain("20");
  // Retention policy.
  expect(text).toContain("90");
  // Scope of use.
  expect(text).toContain("資料使用範圍");
  expect(text).toContain("Secrets");
  // Format rule: judged by content, not by extension.
  expect(text).toContain("副檔名");

  expect(calls.some((u) => u.includes("/test-cases/limits"))).toBe(true);
  // Nothing was uploaded to get here: the rules are shown first, not as the text
  // of a refusal.
  expect(calls.some((u) => u.includes("/datasets"))).toBe(false);
  expect(container.querySelector("input[type=file]")).not.toBeNull();
});

test("02:TEST-002 without the rules there is no upload control at all", async () => {
  stubPlatform(500);
  await renderUpload();

  expect(container.textContent).toContain("無法讀取上傳規則");
  expect(container.querySelector("input[type=file]")).toBeNull();
});
