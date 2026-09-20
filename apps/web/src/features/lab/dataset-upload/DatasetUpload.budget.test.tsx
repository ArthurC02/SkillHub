import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../../core/api/queryClient";
import { DatasetUpload } from "./DatasetUpload.page";

const TEST_CASE = "11111111-1111-4111-8111-111111111111";

let container: HTMLDivElement;
let root: Root;
let calls: string[];

beforeEach(() => {
  queryClient.clear();
  calls = [];
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children?: unknown }) => (
    <a href={to}>{children as never}</a>
  ),
  useSearch: () => ({ test_case: TEST_CASE }),
}));

function platform(totalBytes: number, fileCount: number) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input).replace(/^https?:\/\/[^/]+/, "");
    calls.push(`${init?.method ?? "GET"} ${path}`);
    if (path === "/test-cases/limits") {
      return json({
        max_file_bytes: 1_000_000,
        max_test_case_bytes: 10_000,
        max_files_per_test_case: 3,
        retention_days: 7,
        allowed_kinds: ["csv"],
        note: "",
      });
    }
    if (path === `/test-cases/${TEST_CASE}/datasets`) {
      return json({
        datasets: Array.from({ length: fileCount }, (_, i) => ({
          dataset_id: `d${i}`,
          file_name: `old${i}.csv`,
          size_bytes: totalBytes / Math.max(fileCount, 1),
        })),
        total_bytes: totalBytes,
      });
    }
    return json({});
  });
}

function json(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

async function renderPage() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <DatasetUpload />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  const deadline = Date.now() + 2000;
  while (Date.now() < deadline) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (queryClient.isFetching() === 0 && container.querySelector("input[type=file]")) break;
  }
}

async function chooseAndUpload(size: number) {
  const input = container.querySelector<HTMLInputElement>("input[type=file]");
  if (!input) throw new Error("the page never rendered a file input");
  const file = new File([new Uint8Array(size)], "rows.csv", { type: "text/csv" });
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  const button = Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("上傳"),
  );
  if (!button) throw new Error("the page never rendered an upload button");
  await act(async () => button.click());
}

test("a file that cannot fit is refused without the bytes ever leaving the browser", async () => {
  platform(9_500, 1);
  await renderPage();
  calls.length = 0;

  await chooseAndUpload(1_000);

  expect(
    calls.filter((c) => c.startsWith("POST")),
    "the page already knew it would not fit; sending it spends the user's upload for nothing",
  ).toEqual([]);
  expect(container.textContent ?? "").toContain("還剩 500 B 可用");
});

test("a file that fits is still sent", async () => {
  platform(9_500, 1);
  await renderPage();
  calls.length = 0;

  await chooseAndUpload(400);

  expect(calls.filter((c) => c.startsWith("POST"))).toEqual([
    `POST /test-cases/${TEST_CASE}/datasets`,
  ]);
});

test("the page shows what is left before anything is chosen", async () => {
  platform(9_500, 1);
  await renderPage();

  expect(container.textContent ?? "").toContain("還可以再上傳 2 個檔案");
});
