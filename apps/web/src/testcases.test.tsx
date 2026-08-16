import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { TestCaseDetail } from "./pages/TestCases";
import type { TestCase } from "./api/testcases";

// 03:TEST-012, whose bar is 02:TEST-001 第 3 條: 「使用者可新增、編輯、刪除及
// 確認驗收條件」. The four verbs have a user as their subject, so the test drives
// the buttons and asserts the requests that come out — an assertion about fields
// being present on screen would not have caught the state TEST-003 was handed
// back for.

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

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ testCaseId: TEST_CASE }),
  useNavigate: () => () => Promise.resolve(),
  Link: ({ children }: { children?: unknown }) => children,
}));

const draft: TestCase = {
  test_case_id: TEST_CASE,
  skill_id: "11111111-1111-1111-1111-111111111111",
  name: "去重複列",
  user_prompt: "把重複的列刪掉，保留第一次出現的那一列。",
  acceptance_criteria: [
    {
      id: "c1",
      text: "輸出的列數少於輸入",
      source: "user",
      confirmed_at: "2026-08-17T01:00:00Z",
    },
  ],
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T01:00:00Z",
};

function stubPlatform() {
  const calls: { url: string; method: string; body?: string }[] = [];
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );

  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, method: init?.method ?? "GET", body: init?.body as string | undefined });
    if (url.includes("/datasets")) return json({ datasets: [], total_bytes: 0 });
    return json(draft);
  });

  return calls;
}

async function render() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <TestCaseDetail />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() => !(container.textContent ?? "").includes("載入 Test Case 中"));
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

function button(label: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === label,
  );
  if (!found) throw new Error(`no ${label} button; DOM was:\n${container.textContent}`);
  return found;
}

function setValue(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    input instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

test("02:TEST-001 第 3 條 a user can add, delete and withdraw the confirmation of a criterion", async () => {
  const calls = stubPlatform();
  await render();

  const box = container.querySelector<HTMLInputElement>("#new-criterion");
  expect(box).not.toBeNull();
  await act(async () => setValue(box as HTMLInputElement, "不得改動其他欄位"));
  await act(async () => button("新增").click());
  const added = calls.find((c) => c.method === "POST" && c.url.endsWith("/criteria"));
  expect(added?.body).toContain("不得改動其他欄位");

  // The stored criterion is confirmed, so the control on offer is the one that
  // withdraws it — confirming twice is not a thing a user can ask for.
  await act(async () => button("取消確認").click());
  const withdrawn = calls.find((c) => c.method === "PATCH" && c.url.includes("/criteria/c1"));
  expect(withdrawn?.body).toContain('"confirmed":false');

  await act(async () => button("刪除").click());
  expect(calls.some((c) => c.method === "DELETE" && c.url.includes("/criteria/c1"))).toBe(true);
});

test("CONTENT-007 a rubric line is written against a criterion and saved with its version", async () => {
  const calls = stubPlatform();
  await render();

  // The editor offers one box per acceptance criterion, so the item's id is the
  // criterion's by construction — the id correspondence is not something the
  // user can get wrong.
  const box = container.querySelector<HTMLTextAreaElement>("#rubric-c1");
  expect(box).not.toBeNull();
  await act(async () => setValue(box as HTMLTextAreaElement, "引出顯示列數變少的那一句。"));
  const version = container.querySelector<HTMLInputElement>("#rubric-version");
  await act(async () => setValue(version as HTMLInputElement, "content-007/writing/v1"));
  const evidence = container.querySelector<HTMLInputElement>("#rubric-evidence-c1");
  await act(async () => (evidence as HTMLInputElement).click());

  await act(async () => button("儲存 Rubric").click());
  const saved = calls.find((c) => c.method === "PATCH" && c.body?.includes("rubric"));
  expect(saved?.body).toContain('"version":"content-007/writing/v1"');
  expect(saved?.body).toContain('"id":"c1"');
  expect(saved?.body).toContain('"evidence_required":true');
});

test("CONTENT-007 clearing every line removes the rubric rather than storing an empty one", async () => {
  const calls = stubPlatform();
  await render();

  // Nothing was typed, so there is no rubric — and "no rubric" is null, not an
  // empty item list, because the two are different statements to a reader.
  expect(container.textContent).toContain("儲存等於移除這個 Test Case 的 rubric");
  await act(async () => button("儲存 Rubric").click());
  const saved = calls.find((c) => c.method === "PATCH" && c.body?.includes("rubric"));
  expect(saved?.body).toContain('"rubric":null');
});

test("iron rule 4 the screen says a run freezes a snapshot and that editing clears a confirmation", async () => {
  stubPlatform();
  await render();

  // The snapshot rule is on screen rather than assumed: editing a draft must not
  // read as rewriting what a past run executed.
  expect(container.textContent).toContain("凍結成快照");

  const edit = container.querySelector<HTMLInputElement>("#criterion-c1");
  expect(edit).not.toBeNull();
  await act(async () => setValue(edit as HTMLInputElement, "輸出的列數必須少於輸入"));

  // Confirmation applied to the old words, so the page warns before the save
  // rather than letting the cleared state look like a bug.
  expect(container.textContent).toContain("會清除這一條的確認");
});
