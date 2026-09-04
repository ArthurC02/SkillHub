import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { DataPolicy } from "./pages/DataPolicy";

// 02:O11Y-004 / 04 丙-25② — the data policy page. The one thing worth a test here
// is that it never invents a retention window: ADR-029's proposed 180 days is not
// ratified, and a page that printed it while the deployment collected nothing
// would be the 04 乙-2 mistake in a new place.

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

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children?: unknown }) => (
    <a href={to}>{children as never}</a>
  ),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

async function render(node: ReactNode, settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  const deadline = Date.now() + 2000;
  while (Date.now() < deadline) {
    if (settled()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`timed out; DOM was: ${container.textContent}`);
}

const text = () => container.textContent ?? "";

const EVENTS = [
  {
    name: "search_performed",
    when: "a search is submitted",
    attributes: ["query_length", "query_language"],
    not_recorded: "not one word of the query itself",
  },
];

// The Go server's own wording (04 丙-154 ②, strings-149-152-154-go-workspace-
// learning.md) — a fixture that paraphrased this would hide a contract drift
// the same way an English fixture would (04 丙-143's rule).
const FEEDBACK = {
  what: "由已登入的參與者在 POST /feedback 送出的回報（BETA-003/004/005）",
  collected: ["kind", "message", "page_path", "run_id"],
  free_text:
    "message 是參與者自己寫的自由文字，最多 2000 字。它是這個部署唯一的自由文字欄位，不遮罩、不摘要、不截斷",
  page_path: "他當時所在的路由，從不是完整網址：查詢字串可能帶個資，這個管道不收",
  run_id: "他當時看的 Run（若有），而且只在確認是他自己的 Run 之後",
  on_account_deletion:
    "去識別而不是刪除：workspace_id 與 user_id 設為 NULL，文字保留（ADR-029 決策 5 的範圍複審建立在人們說了什麼之上，帳號刪除不能悄悄撤回已被計入的回報）",
  retention_days: null,
  note: "這個部署沒有設定回報的保存期限（FEEDBACK_RETENTION 未設），所以這些回報會一直保留，直到設定期限並執行 maintenance purge-feedback",
};

test("O11Y-004 an unconfigured deployment says 目前不收集 rather than a proposed number", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      collecting: false,
      retention_days: 0,
      events: EVENTS,
      note: "no free-text column",
      feedback: FEEDBACK,
    }),
  );
  await render(<DataPolicy />, () => text().includes("目前不收集"));

  // NFR-002 made visible: no retention value, no collection.
  expect(text()).toContain("一列都不寫");
  expect(text()).not.toContain("180");
  // No window is stated at all, not even a zero-day one: the copy that names a
  // retention period belongs to the other branch and must not leak into this one.
  expect(text()).not.toContain("到期後刪除");
  // The four events are still disclosed — "we collect nothing" is a disclosure.
  expect(text()).toContain("search_performed");
  expect(text()).toContain("query_length");
});

test("O11Y-004 a collecting deployment states the window it actually applies", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      collecting: true,
      retention_days: 180,
      events: EVENTS,
      note: "no free-text column",
      feedback: FEEDBACK,
    }),
  );
  await render(<DataPolicy />, () => text().includes("180"));

  expect(text()).toContain("180 天");
  expect(text()).not.toContain("目前不收集");
  // The disclosure the whitelist alone would hide.
  expect(text()).toContain("not one word of the query itself");
});

test("O11Y-004 a failed read is a failed read, never an implied 'nothing is collected'", async () => {
  vi.stubGlobal("fetch", () => json({ error: "boom" }, 500));
  await render(<DataPolicy />, () => text().includes("無法讀取分析事件政策"));

  expect(text()).toContain("讀不到不等於沒有收集");
  expect(text()).not.toContain("目前不收集");
});

// 04 丙-154 ②. GET /policy/data-retention has sent `feedback` since POST
// /feedback existed; this page never read it. It gets its own section under
// the four analytics events, rendered with the server's own sentences.
test("丙-154② the feedback data class is disclosed with the server's own sentences", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      collecting: false,
      retention_days: 0,
      events: EVENTS,
      note: "no free-text column",
      feedback: FEEDBACK,
    }),
  );
  await render(<DataPolicy />, () => text().includes("回報問題的資料"));

  expect(text()).toContain(FEEDBACK.what);
  for (const column of FEEDBACK.collected) {
    expect(text()).toContain(column);
  }
  expect(text()).toContain(FEEDBACK.free_text);
  expect(text()).toContain(FEEDBACK.page_path);
  expect(text()).toContain(FEEDBACK.run_id);
  expect(text()).toContain(FEEDBACK.on_account_deletion);
  // §2.9: a null retention_days is the type-level absence word, not a blank.
  expect(text()).toContain("尚未定值");
  expect(text()).toContain(FEEDBACK.note);
});

test("丙-154② a configured retention window prints the number, not 尚未定值", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      collecting: false,
      retention_days: 0,
      events: EVENTS,
      note: "no free-text column",
      feedback: { ...FEEDBACK, retention_days: 90, note: undefined },
    }),
  );
  await render(<DataPolicy />, () => text().includes("回報問題的資料"));

  expect(text()).toContain("90 天");
  expect(text()).not.toContain("尚未定值");
});
