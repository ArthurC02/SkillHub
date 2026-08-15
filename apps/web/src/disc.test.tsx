import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { LicenseBadge, LicenseNotes } from "./components/LicenseBadge";
import { RiskIndicator } from "./components/RiskIndicator";
import type { PublicSearchResponse, SkillLicense, SkillRisk } from "./api/types";

// Renders against mocked fetch; no backend needed. The DOM plumbing below is
// deliberately hand-rolled — @testing-library is not a dependency of this app
// and these four assertions do not justify adding one.

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

async function render(node: React.ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(<StrictMode>{node}</StrictMode>);
  });
}

/** Answers only the search call; /me stays 401 so the page renders logged-out. */
function stubSearch(body: PublicSearchResponse) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    calls.push(String(input));
    if (String(input).includes("/api/skills/search")) {
      return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }));
    }
    return Promise.resolve(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );
  });
  return calls;
}

async function submitSearch(text: string) {
  const input = container.querySelector("input")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await waitFor(() => !container.textContent?.includes("搜尋中…"));
}

/** Polls until the query has settled and React has flushed the result. */
async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
    if (done()) return;
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

const EMPTY: PublicSearchResponse = {
  query: "",
  results: [],
  degraded: false,
  partial_index: false,
  no_results: false,
};

test("DISC-001: search hits the public endpoint, which needs no session", async () => {
  const calls = stubSearch({ ...EMPTY, query: "pdf 摘要", no_results: true });
  await render(<App />);
  await submitSearch("pdf 摘要");

  expect(calls.some((url) => url.includes("/api/skills/search?q=pdf"))).toBe(true);
  // The workspace-scoped route (which requires a session) must not be used.
  expect(calls.some((url) => /\/skills\/search\?/.test(url) && !url.includes("/api/"))).toBe(false);
});

test("DISC-005: no_results shows the server's query_suggestion, not hardcoded copy", async () => {
  stubSearch({
    ...EMPTY,
    query: "asdf",
    no_results: true,
    query_suggestion: "Try naming the file format you have.",
  });
  await render(<App />);
  await submitSearch("asdf");

  expect(container.textContent).toContain("Try naming the file format you have.");
});

test("DISC-002: each candidate shows its match reason, labelled by provenance", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    results: [
      {
        skill_id: "11111111-1111-1111-1111-111111111111",
        name: "PDF Summariser",
        summary: "把 PDF 轉成摘要",
        rank: 0.82,
        match_reason: "這個 Skill 直接處理 PDF 並輸出摘要。",
        match_reason_source: "model",
      },
      {
        skill_id: "22222222-2222-2222-2222-222222222222",
        name: "Doc Splitter",
        summary: "切分文件",
        rank: 0.4,
        match_reason: "查詢與文件共同出現：pdf",
        match_reason_source: "template",
      },
    ],
  });
  await render(<App />);
  await submitSearch("pdf");

  const text = container.textContent ?? "";
  expect(text).toContain("這個 Skill 直接處理 PDF 並輸出摘要。");
  expect(text).toContain("查詢與文件共同出現：pdf");
  // ADR-013: model-written copy carries a visible marker; template copy does not
  // borrow it.
  expect(container.querySelectorAll(".badge-source-model")).toHaveLength(1);
  expect(container.querySelectorAll(".badge-source-template")).toHaveLength(1);
});

test("DISC-005: degraded and partial_index are separate, non-blocking notices", async () => {
  stubSearch({
    query: "pdf",
    results: [
      {
        skill_id: "33333333-3333-3333-3333-333333333333",
        name: "Lexical Hit",
        summary: "只靠關鍵字命中",
        // A real FTS-only answer returned 1.4 here: on the degraded path `rank`
        // is the lexical score, not the 0..1 cosine similarity the schema
        // documents for the hybrid path.
        rank: 1.4,
      },
    ],
    degraded: true,
    degraded_reason: "embedding unavailable; lexical search only",
    partial_index: true,
    no_results: false,
  });
  await render(<App />);
  await submitSearch("pdf");

  const text = container.textContent ?? "";
  expect(container.querySelectorAll(".notice")).toHaveLength(2);
  expect(text).toContain("embedding unavailable; lexical search only");
  // Non-blocking: the result is still listed.
  expect(text).toContain("Lexical Hit");
  // The out-of-range lexical score is never shown, nor called a similarity.
  expect(text).not.toMatch(/相似度\s*[\d.]/);
  expect(text).not.toContain("1.4");
  expect(text).toContain("未計算語意相似度");
});

test("DISC-002: an unranked hit on the hybrid path reads as unscored, not 0.00", async () => {
  stubSearch({
    ...EMPTY,
    query: "pdf",
    results: [
      {
        skill_id: "44444444-4444-4444-4444-444444444444",
        name: "Pending Enrichment",
        summary: "尚未建立索引",
        rank: 0,
      },
    ],
    partial_index: true,
  });
  await render(<App />);
  await submitSearch("pdf");

  const text = container.textContent ?? "";
  expect(text).toContain("未評分");
  expect(text).not.toContain("相似度 0.00");
});

test("DISC-008: license shows both axes — expression and provenance tier", async () => {
  const license: SkillLicense = {
    expression: "MIT",
    source: "repo-license-file",
    source_note: "來自 repo 根目錄的 LICENSE，涵蓋整個 repo。",
    status: { value: "declared", label: "License 已宣告", note: "尚未經人工核對。" },
  };
  await render(
    <>
      <LicenseBadge license={license} />
      <LicenseNotes license={license} />
    </>,
  );

  const text = container.textContent ?? "";
  expect(text).toContain("MIT");
  expect(text).toContain("已宣告"); // status axis: declared, not confirmed
  expect(text).toContain("repo 根目錄 LICENSE"); // provenance axis
  expect(text).toContain("涵蓋整個 repo");
});

test("DISC-008: unknown license never shows a name or implies permissiveness", async () => {
  const license: SkillLicense = {
    status: { value: "unknown", label: "License 未知", note: "未宣告 License，依規則不可下載。" },
  };
  await render(<LicenseBadge license={license} />);

  expect(container.textContent).toContain("License 未知");
  expect(container.querySelector(".license-expression")).toBeNull();
  expect(container.querySelector(".badge-license-source")).toBeNull();
});

const RISK: SkillRisk = {
  scan_status: "scanned",
  counts: { errors: 0, warnings: 1, infos: 321 },
  highlights: [
    {
      severity: "warning",
      code: "embedded-script",
      path: "SKILL.md",
      message: "SKILL.md 內含可執行程式碼區塊。",
    },
  ],
  info_counts: { "external-url": 320, "large-file": 1 },
  has_embedded_script: true,
  note: "以上為靜態掃描結果。",
};

test("DISC-008: warnings are up front, info findings aggregate behind a disclosure", async () => {
  await render(<RiskIndicator risk={RISK} />);

  const text = container.textContent ?? "";
  // Errors/warnings verbatim and not hidden.
  expect(text).toContain("SKILL.md 內含可執行程式碼區塊。");
  expect(container.querySelector(".risk-list .badge-risk")).not.toBeNull();
  // The embedded-script flag is its own visible marker (SKILL-003).
  expect(text).toContain("SKILL.md 內含可執行程式碼");

  // 321 info findings collapse to per-code counts inside <details>.
  const details = container.querySelector("details.risk-infos")!;
  expect(details).not.toBeNull();
  expect((details as HTMLDetailsElement).open).toBe(false);
  expect(details.textContent).toContain("external-url");
  expect(details.textContent).toContain("320");
  expect(details.querySelectorAll("li")).toHaveLength(2);
});

test("DISC-004: an unreadable package is reported as unknown, never as a clean scan", async () => {
  await render(
    <RiskIndicator
      risk={{
        scan_status: "unavailable",
        counts: { errors: 0, warnings: 0, infos: 0 },
        highlights: [],
        info_counts: {},
        note: "以上為靜態掃描結果。",
      }}
    />,
  );

  const text = container.textContent ?? "";
  expect(text).toContain("未知");
  expect(text).not.toContain("未發現錯誤或警告");
});
