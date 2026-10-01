import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { DatasetLimits } from "../../lab.service";
import { UploadRulesFacts } from "./UploadRulesFacts";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    children?: unknown;
  }) => (
    <a href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}>
      {children as never}
    </a>
  ),
}));

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
});

async function mount(node: React.ReactElement) {
  root = createRoot(container);
  await act(async () => {
    root.render(<StrictMode>{node}</StrictMode>);
  });
}

const text = () => container.textContent ?? "";

const LIMITS: DatasetLimits = {
  max_file_bytes: 1_000_000,
  max_test_case_bytes: 5_000_000,
  max_files_per_test_case: 10,
  retention_days: 30,
  allowed_kinds: ["csv", "json"],
  note: "",
};

test("shows the retention window the server reported", async () => {
  await mount(<UploadRulesFacts limits={LIMITS} used={undefined} testCase="tc-1" />);

  expect(text()).toContain("上傳後保存 30 天");
});

test("says it is still reading usage when the usage query has not settled", async () => {
  await mount(<UploadRulesFacts limits={LIMITS} used={undefined} testCase="tc-1" />);

  expect(text()).toContain("正在讀這個測試題已經用掉多少");
});

test("shows remaining budget, computed from the limit minus what is already used", async () => {
  await mount(
    <UploadRulesFacts
      limits={LIMITS}
      used={{ fileCount: 3, totalBytes: 1_000_000 }}
      testCase="tc-1"
    />,
  );

  expect(text()).toContain("已經用掉 3 個檔案");
  expect(text(), "10 - 3 = 7 個檔案還可以上傳").toContain("還可以再上傳 7 個檔案");
});

test("links to the test case's own materials section when a test case is chosen", async () => {
  await mount(
    <UploadRulesFacts limits={LIMITS} used={{ fileCount: 1, totalBytes: 1 }} testCase="tc-9" />,
  );

  const link = container.querySelector("a");
  expect(link?.getAttribute("href")).toBe("/lab/test-cases/tc-9");
});

test("names no test case link when none is chosen yet", async () => {
  await mount(
    <UploadRulesFacts limits={LIMITS} used={{ fileCount: 1, totalBytes: 1 }} testCase="" />,
  );

  expect(container.querySelector("a")).toBeNull();
  expect(text()).toContain("測試題頁的「測試資料」那一節");
});
