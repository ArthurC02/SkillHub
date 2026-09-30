import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { TestCaseDetail } from "./test-cases/TestCaseDetail.page";
import { TestCaseList } from "./test-cases/TestCaseList.page";
import type { TestCase } from "./testcases.service";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  listSearch = {};
  navigations.length = 0;
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

const TEST_CASE = "33333333-3333-3333-3333-333333333333";
const SKILL = "11111111-1111-1111-1111-111111111111";
const VERSION = "22222222-2222-2222-2222-222222222222";
const OTHER_VERSION = "55555555-5555-5555-5555-555555555555";
const OTHER_SKILL = "44444444-4444-4444-4444-444444444444";

let listSearch: { skill?: string; version?: string } = {};
const navigations: unknown[] = [];

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ testCaseId: TEST_CASE }),
  useNavigate: () => (options: unknown) => {
    navigations.push(options);
    return Promise.resolve();
  },
  useSearch: () => listSearch,
  Link: ({
    children,
    to,
    params,
    search,
  }: {
    children?: ReactNode;
    to: string;
    params?: unknown;
    search?: unknown;
  }) => (
    <a data-to={to} data-params={JSON.stringify(params)} data-search={JSON.stringify(search)}>
      {children}
    </a>
  ),
}));

const draft: TestCase = {
  test_case_id: TEST_CASE,
  skill_id: SKILL,
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

const RUN = {
  run_id: "99999999-9999-9999-9999-999999999999",
  status: "succeeded",
  skill_id: SKILL,
  skill_name: "去重複工具",
  skill_version_id: VERSION,
  test_case_id: TEST_CASE,
  provider: "self-hosted",
  cleanup_status: "cleaned",
  evaluation: {
    value: "met",
    label: "符合",
    note: "依這個 Run 當時的驗收條件判定為符合。",
  },
  created_at: "2026-08-18T00:00:00Z",
  finished_at: "2026-08-18T00:04:00Z",
};

const SKILL_VERSIONS = [
  {
    version_id: VERSION,
    version_number: 2,
    content_hash: "sha256:bb",
    created_at: "2026-08-18T00:00:00Z",
  },
  {
    version_id: OTHER_VERSION,
    version_number: 1,
    content_hash: "sha256:aa",
    created_at: "2026-08-17T00:00:00Z",
  },
];

type Overrides = {
  suggest?: { suggestions: { text: string }[] } | { status: number; error: string };
  suggestResponse?: Promise<Response>;
  runs?: unknown[];
  datasets?: unknown[];
  testCases?: unknown[];
  testCase?: TestCase;
  create?: { status: number; error: string };
  skills?: { skill_id: string; name: string; summary: string }[];
  versions?: typeof SKILL_VERSIONS;
};

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

type PlatformRequest = { url: string; path: string; method: string };

function platformHandlers(over: Overrides, removedRef: { removed: boolean }) {
  return [
    (req: PlatformRequest) => {
      if (!req.url.includes("/criteria/suggest")) return undefined;
      if (over.suggestResponse) return over.suggestResponse;
      const s = over.suggest ?? { suggestions: [] };
      return "status" in s ? json({ error: s.error }, s.status) : json(s);
    },
    (req: PlatformRequest) =>
      req.url.includes("/datasets")
        ? json({ datasets: over.datasets ?? [], total_bytes: 0 })
        : undefined,
    (req: PlatformRequest) => (req.path === "/runs" ? json({ runs: over.runs ?? [] }) : undefined),
    (req: PlatformRequest) =>
      req.path === "/skills"
        ? json({
            skills: over.skills ?? [{ skill_id: SKILL, name: "去重複工具", summary: "" }],
          })
        : undefined,
    (req: PlatformRequest) =>
      req.path === `/api/skills/${SKILL}`
        ? json({ skill_id: SKILL, name: "去重複工具", summary: "整理重複資料。" })
        : undefined,
    (req: PlatformRequest) =>
      req.path === `/skills/${SKILL}/versions`
        ? json({ versions: over.versions ?? SKILL_VERSIONS })
        : undefined,
    (req: PlatformRequest) => {
      if (req.method !== "POST" || req.path !== "/test-cases") return undefined;
      if (over.create) return json({ error: over.create.error }, over.create.status);
      return json(draft, 201);
    },
    (req: PlatformRequest) =>
      req.path === "/test-cases" ? json({ test_cases: over.testCases ?? [] }) : undefined,
    (req: PlatformRequest) => {
      if (req.method !== "DELETE" || req.path !== `/test-cases/${TEST_CASE}`) return undefined;
      removedRef.removed = true;
      return json({
        deleted: true,
        datasets_deleted: 2,
        note: "Test Case 與它上傳的檔案已移除，檔案本身也刪了；過去 Run 的快照仍保留 Prompt、驗收條件，以及每個檔案的檔名與內容雜湊。",
      });
    },
    (req: PlatformRequest) =>
      removedRef.removed && req.path === `/test-cases/${TEST_CASE}`
        ? json({ error: "找不到這個 Test Case" }, 404)
        : undefined,
  ];
}

function stubPlatform(over: Overrides = {}) {
  const calls: { url: string; method: string; body?: string }[] = [];
  const removedRef = { removed: false };
  const handlers = platformHandlers(over, removedRef);

  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    const method = init?.method ?? "GET";
    calls.push({ url, method, body: init?.body as string | undefined });
    const req: PlatformRequest = { url, path, method };
    for (const handle of handlers) {
      const result = handle(req);
      if (result) return result;
    }
    return json(over.testCase ?? draft);
  });

  return calls;
}

async function renderComponent(node: ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
}

async function render() {
  await renderComponent(<TestCaseDetail />);
  await waitFor(() => container.querySelector("[data-loading]") === null);
}

async function renderList() {
  await renderComponent(<TestCaseList />);
  await waitFor(() => container.querySelector("[data-loading]") === null);
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

const text = () => container.textContent ?? "";

test("the Test Case Dataset entry preserves the exact Test Case and Version", async () => {
  listSearch = { version: VERSION };
  stubPlatform();
  await render();

  const link = container.querySelector('[data-to="/lab/test-cases/$testCaseId/datasets"]');
  expect(link?.getAttribute("data-params")).toBe(JSON.stringify({ testCaseId: TEST_CASE }));
  expect(link?.getAttribute("data-search")).toBe(JSON.stringify({ version: VERSION }));
});

test("the Test Case workbench names the URL-selected Skill, Version and Test Case", async () => {
  listSearch = { version: OTHER_VERSION };
  stubPlatform({ runs: [RUN] });
  await render();
  await waitFor(() => text().includes("由這個網址選定"));

  const context = container.querySelector("[data-role='test-case-context']")!;
  expect(context.textContent).toContain("去重複工具");
  expect(context.textContent).toContain("v1");
  expect(context.textContent).toContain("去重複列");
  expect(context.textContent).not.toContain("v2");
  const runLink = container.querySelector<HTMLAnchorElement>(
    '[data-to="/skills/$skillId/test-cases/$testCaseId/runs/new"]',
  );
  expect(runLink?.getAttribute("data-search")).toBe(JSON.stringify({ version: OTHER_VERSION }));
});

test("the Test Case workbench puts execution after its editable design and before history", async () => {
  listSearch = { version: VERSION };
  stubPlatform({ runs: [RUN] });
  await render();

  const headings = Array.from(
    container.querySelectorAll("h2,h3"),
    (heading) => heading.textContent,
  );
  const journey = [
    "名稱與 User Prompt",
    "驗收條件",
    "Rubric（選用）",
    "測試資料",
    "下一步：試跑這個驗證設計",
    "執行歷史",
  ];
  expect(headings.filter((heading) => heading && journey.includes(heading))).toEqual(journey);
});

test("the Test Case workbench keeps design primary and supporting actions in a rail", async () => {
  listSearch = { version: VERSION };
  stubPlatform({ runs: [RUN] });
  await render();

  const layout = container.querySelector(".test-case-detail-layout");
  expect(layout).not.toBeNull();
  expect(layout!.querySelector(":scope > .test-case-design")).not.toBeNull();
  expect(layout!.querySelector(":scope > .test-case-detail-rail")).not.toBeNull();
});

test("the Test Case workbench labels a Version inherited from the latest matching Run", async () => {
  stubPlatform({ runs: [RUN] });
  await render();
  await waitFor(() => text().includes("沿用最近一次 Run"));

  const context = container.querySelector("[data-role='test-case-context']")!;
  expect(context.textContent).toContain("v2");
  const runLink = container.querySelector<HTMLAnchorElement>(
    '[data-to="/skills/$skillId/test-cases/$testCaseId/runs/new"]',
  );
  expect(runLink?.getAttribute("data-search")).toBe(JSON.stringify({ version: VERSION }));
});

test("the Test Case workbench asks for a Version when no matching Run can supply one", async () => {
  stubPlatform({ runs: [{ ...RUN, skill_id: OTHER_SKILL }] });
  await render();
  await waitFor(() => text().includes("尚未選擇 Version"));

  expect(text()).toContain("選擇 Version 並確認權限");
  const runLink = container.querySelector<HTMLAnchorElement>(
    '[data-to="/skills/$skillId/test-cases/$testCaseId/runs/new"]',
  );
  expect(runLink?.getAttribute("data-search")).toBe(JSON.stringify({ version: undefined }));
});

test("the Test Case workbench refuses an URL Version outside the owner-scoped list", async () => {
  listSearch = { version: "66666666-6666-6666-6666-666666666666" };
  stubPlatform({ runs: [RUN] });
  await render();
  await waitFor(() => text().includes("Version 不屬於目前的 Skill"));

  expect(text()).toContain("選擇 Version 並確認權限");
  const runLink = container.querySelector<HTMLAnchorElement>(
    '[data-to="/skills/$skillId/test-cases/$testCaseId/runs/new"]',
  );
  expect(runLink?.getAttribute("data-search")).toBe(JSON.stringify({ version: undefined }));
});

async function submitNewTestCase(skillId: string) {
  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, skillId),
  );
  await act(async () => setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "名稱"));
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );
  await act(async () => button("建立").click());
  await waitFor(() => navigations.length > 0);
}

test("a disabled prompt save exposes its visible reason to assistive technology", async () => {
  stubPlatform();
  await render();

  const name = container.querySelector<HTMLInputElement>("#edit-name")!;
  await act(async () => setValue(name, ""));
  const save = button("儲存");
  expect(save.disabled).toBe(true);
  const reasonID = save.getAttribute("aria-describedby");
  expect(reasonID).toBe("edit-required-reason");
  expect(container.querySelector(`#${reasonID}`)?.textContent).toContain("名稱是空的");
});

function button(label: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === label,
  );
  if (!found) throw new Error(`no ${label} button; DOM was:\n${container.textContent}`);
  return found;
}

function selectValue(select: HTMLSelectElement, value: string) {
  select.value = value;
  select.dispatchEvent(new Event("change", { bubbles: true }));
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

  await act(async () => button("取消確認").click());
  const withdrawn = calls.find((c) => c.method === "PATCH" && c.url.includes("/criteria/c1"));
  expect(withdrawn?.body).toContain('"confirmed":false');

  await act(async () => button("刪除這一條").click());
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);
  await act(async () => button("確認刪除這一條").click());
  expect(calls.some((c) => c.method === "DELETE" && c.url.includes("/criteria/c1"))).toBe(true);
});

test("CONTENT-007 a rubric line is written against a criterion and saved with its version", async () => {
  const calls = stubPlatform();
  await render();

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

  expect(container.textContent).toContain("儲存等於移除這個 Test Case 的 rubric");
  await act(async () => button("儲存 Rubric").click());
  const saved = calls.find((c) => c.method === "PATCH" && c.body?.includes("rubric"));
  expect(saved?.body).toContain('"rubric":null');
});

test("iron rule 4 the screen says a run freezes a snapshot and that editing clears a confirmation", async () => {
  stubPlatform();
  await render();

  expect(container.textContent).toContain("凍結成快照");

  const edit = container.querySelector<HTMLInputElement>("#criterion-c1");
  expect(edit).not.toBeNull();
  await act(async () => setValue(edit as HTMLInputElement, "輸出的列數必須少於輸入"));

  expect(container.textContent).toContain("會清除這一條的確認");
});

test("TEST-002 asking for suggestions writes nothing; each one is adopted or ignored by the user", async () => {
  const calls = stubPlatform({
    suggest: { suggestions: [{ text: "保留欄位標題列" }, { text: "輸出仍是 CSV" }] },
  });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("尚未加入"));

  expect(container.textContent).toContain("保留欄位標題列");
  expect(container.textContent).toContain("輸出仍是 CSV");
  expect(container.textContent).toContain("尚未加入");
  expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/criteria"))).toBe(false);

  await act(async () => button("採納").click());
  await waitFor(() => calls.some((c) => c.method === "POST" && c.url.endsWith("/criteria")));
  const adopted = calls.find((c) => c.method === "POST" && c.url.endsWith("/criteria"));
  expect(adopted?.body).toContain("保留欄位標題列");
  expect(adopted?.body).toContain('"source":"suggested"');

  await waitFor(() => !(container.textContent ?? "").includes("保留欄位標題列"));
  expect(container.textContent).toContain("輸出仍是 CSV");

  const before = calls.length;
  await act(async () => button("忽略").click());
  expect(container.textContent).not.toContain("輸出仍是 CSV");
  expect(calls.length).toBe(before);
});

test("TEST-002 an adopted suggestion is listed as 系統建議，尚未確認", async () => {
  const withSuggested: TestCase = {
    ...draft,
    acceptance_criteria: [
      { id: "c9", text: "保留欄位標題列", source: "suggested", confirmed_at: null },
    ],
  };
  const json = (body: unknown) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    if (url.split("?")[0] === "/runs") return json({ runs: [] });
    if (url.includes("/datasets")) return json({ datasets: [], total_bytes: 0 });
    return json(withSuggested);
  });
  await render();

  expect(container.textContent).toContain("系統建議，尚未確認");
});

test("TEST-002 a 503 is worded as unavailable and names the manual path, without the server's own words", async () => {
  stubPlatform({ suggest: { status: 503, error: "目前無法自動建議驗收條件，請自己手動輸入" } });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("目前無法自動建議"));
  expect(container.textContent).toContain("驗收條件可以自己手動輸入");
  expect(container.textContent).not.toContain("目前無法自動建議驗收條件，請自己手動輸入");
});

test("丙-150 建立失敗於 401 走 ReadFailure，印登入而不是 not authenticated", async () => {
  const calls = stubPlatform({ create: { status: 401, error: "not authenticated" } });
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () => setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "名稱"));
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );
  await act(async () => button("建立").click());
  await waitFor(() => (container.textContent ?? "").includes("需要登入"));

  expect(container.textContent).not.toContain("not authenticated");
  expect(calls.some((c) => c.method === "POST" && c.url === "/test-cases")).toBe(true);
});

test("丙-155③ 名稱超過 200 bytes 送出前先擋下，不送出請求", async () => {
  const calls = stubPlatform();
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "x".repeat(201)),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );

  expect(container.textContent).toContain("名稱最多 200 bytes，目前 201 bytes。");
  expect(button("建立").disabled).toBe(true);
  expect(calls.some((c) => c.method === "POST" && c.url === "/test-cases")).toBe(false);
});

test("名稱剛好 200 bytes 不擋，建立按鈕不停用", async () => {
  stubPlatform();
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "x".repeat(200)),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );

  expect(container.textContent).not.toContain("名稱最多 200 bytes，目前");
  expect(button("建立").disabled).toBe(false);
});

test("Prompt 超過 32768 bytes 送出前先擋下，不送出請求", async () => {
  const calls = stubPlatform();
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () => setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "名稱"));
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "x".repeat(32769)),
  );

  expect(container.textContent).toContain("Prompt 最多 32768 bytes，目前 32769 bytes。");
  expect(button("建立").disabled).toBe(true);
  expect(calls.some((c) => c.method === "POST" && c.url === "/test-cases")).toBe(false);
});

test("a pending suggestion says why its button is disabled", async () => {
  let finish!: (response: Response) => void;
  const suggestResponse = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  stubPlatform({ suggestResponse });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("建議中…"));
  expect(button("建議中…").disabled).toBe(true);

  await act(async () =>
    finish(
      new Response(JSON.stringify({ suggestions: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
});

test("執行歷史 links each run to the exact immutable version that produced it", async () => {
  const calls = stubPlatform({
    runs: [
      RUN,
      {
        ...RUN,
        run_id: "88888888-8888-8888-8888-888888888888",
        skill_version_id: OTHER_VERSION,
        created_at: "2026-08-19T00:00:00Z",
      },
    ],
  });
  await render();
  await waitFor(() => (container.textContent ?? "").includes("執行歷史"));

  expect(
    calls.some((c) => c.url.includes(`/runs?`) && c.url.includes(`test_case_id=${TEST_CASE}`)),
  ).toBe(true);
  expect(container.querySelector('time[datetime="2026-08-18T00:00:00Z"]')).not.toBeNull();
  expect(container.textContent).toContain(VERSION);
  const fold = Array.from(container.querySelectorAll("details")).find((d) =>
    (d.querySelector("summary")?.textContent ?? "").includes("Skill Version"),
  );
  expect(fold, "the Skill Version id is not behind a disclosure").toBeTruthy();
  expect(fold!.textContent).toContain(VERSION);
  const flat = Array.from(container.querySelectorAll(".download-item p"))
    .map((p) => p.textContent ?? "")
    .join("");
  expect(flat, "the version id is flat on the row again").not.toContain(VERSION);
  const versionLinks = Array.from(
    container.querySelectorAll<HTMLAnchorElement>(
      'ul[data-role="evidence"] a[data-to="/skills/$skillId/versions/$versionId"]',
    ),
  );
  expect(versionLinks.map((link) => JSON.parse(link.dataset.params ?? "{}"))).toEqual([
    { skillId: SKILL, versionId: VERSION },
    { skillId: SKILL, versionId: OTHER_VERSION },
  ]);
  expect(versionLinks.every((link) => link.textContent?.includes("查看這次的版本"))).toBe(true);
  const compareLinks = Array.from(
    container.querySelectorAll<HTMLAnchorElement>(
      'ul[data-role="evidence"] a[data-to="/runs/$runId/compare"]',
    ),
  );
  expect(compareLinks.map((link) => JSON.parse(link.dataset.params ?? "{}"))).toEqual([
    { runId: RUN.run_id },
    { runId: "88888888-8888-8888-8888-888888888888" },
  ]);
  expect(compareLinks.every((link) => link.dataset.search === undefined)).toBe(true);
  expect(compareLinks.every((link) => link.textContent?.includes("以這次 Run 開始比較"))).toBe(
    true,
  );
  expect(container.textContent).toContain("執行完成");
  expect(container.textContent).toContain("任務判定：符合");
  const t = container.textContent ?? "";
  expect(t.indexOf("任務判定")).toBeLessThan(t.indexOf("執行狀態"));
});

test("執行歷史 with no runs says 尚無執行 rather than rendering a zero", async () => {
  stubPlatform({ runs: [] });
  await render();
  await waitFor(() => (container.textContent ?? "").includes("執行歷史"));

  expect(container.textContent).toContain("尚無執行");
  expect(container.textContent).toContain("開始 Run 前會再次顯示權限摘要並要求確認");
  expect(container.textContent).not.toContain("還需要填入");
});

test("02:WS-002 deleting states its scope before it runs and its actual reach after", async () => {
  const calls = stubPlatform({ runs: [] });
  await render();

  await act(async () => button("刪除整個 Test Case").click());
  expect(container.textContent).toContain("已經跑過的 Run 及其快照不受影響");
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除整個 Test Case").click());
  await waitFor(() => (container.textContent ?? "").includes("已刪除這個 Test Case"));
  const sent = calls.find((c) => c.method === "DELETE");
  expect(sent?.url).toContain(`/test-cases/${TEST_CASE}`);

  expect(container.textContent).toContain("2 個上傳檔案");
  expect(container.textContent).toContain("快照與歷史 Run 不受影響");
  expect(container.textContent).toContain("回到 Test Case 列表");
});

test("02:WS-002 deleting a test case does not ask the server for it again", async () => {
  const calls = stubPlatform({ runs: [] });
  await render();
  const fetched = () =>
    calls.filter((c) => c.method === "GET" && c.url === `/test-cases/${TEST_CASE}`).length;
  const before = fetched();

  await act(async () => button("刪除整個 Test Case").click());
  await act(async () => button("確認刪除整個 Test Case").click());
  await waitFor(() => (container.textContent ?? "").includes("已刪除這個 Test Case"));

  expect(fetched()).toBe(before);
  expect(container.textContent).not.toContain("找不到這個 Test Case");
});

const LIST_ROW = {
  ...draft,
  skill_name: "去重複工具",
  criteria_confirmed: 1,
  criteria_total: 3,
  has_rubric: true,
};

test("列表 shows the skill's name, the confirmed count and whether a rubric exists", async () => {
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  expect(container.textContent).toContain("去重複工具");
  expect(container.textContent).toContain("已確認 1/3 條");
  expect(container.textContent).toContain("Rubric 有");
  expect(container.textContent).not.toContain(SKILL);
});

test("列表 separates the scenario index from the Test Case editor", async () => {
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  const workspace = container.querySelector(".test-case-workspace");
  expect(workspace?.children).toHaveLength(2);
  expect(workspace?.children[0]?.classList.contains("test-case-index")).toBe(true);
  expect(workspace?.children[1]?.classList.contains("test-case-editor")).toBe(true);
});

test("列表 ?skill= narrows the request and says so, with a way back to the full list", async () => {
  listSearch = { skill: SKILL };
  const calls = stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  expect(
    calls.some((c) => c.url.includes("/test-cases?") && c.url.includes(`skill_id=${SKILL}`)),
  ).toBe(true);
  expect(container.textContent).toContain("只顯示");
  expect(container.textContent).toContain("顯示全部");
});

test("列表 ?skill= 指名的 Skill 要預先填進建立表單，不要再問一次", async () => {
  listSearch = { skill: SKILL };
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();
  await waitFor(() => container.querySelector<HTMLSelectElement>("#tc-skill")?.value === SKILL);

  expect(container.querySelector<HTMLSelectElement>("#tc-skill")!.value).toBe(SKILL);
  expect(container.textContent).not.toContain("還不能建立，因為：選一個 Skill");
});

test("creating a Test Case for the current Skill keeps the selected version", async () => {
  listSearch = { skill: SKILL, version: VERSION };
  stubPlatform();
  await renderList();

  await submitNewTestCase(SKILL);

  expect(navigations).toContainEqual({
    to: "/lab/test-cases/$testCaseId",
    params: { testCaseId: TEST_CASE },
    search: { version: VERSION },
  });
});

test("creating a Test Case for another Skill drops the previous Skill's version", async () => {
  listSearch = { skill: SKILL, version: VERSION };
  const calls = stubPlatform({
    skills: [
      { skill_id: SKILL, name: "去重複工具", summary: "" },
      { skill_id: OTHER_SKILL, name: "另一個工具", summary: "" },
    ],
  });
  await renderList();

  await submitNewTestCase(OTHER_SKILL);

  expect(calls.find((call) => call.method === "POST")?.body).toContain(OTHER_SKILL);
  expect(navigations).toContainEqual({
    to: "/lab/test-cases/$testCaseId",
    params: { testCaseId: TEST_CASE },
    search: { version: undefined },
  });
});

test("設計 §2.4 the Rubric save button says why it cannot be pressed", async () => {
  stubPlatform();
  await render();

  const box = container.querySelector<HTMLTextAreaElement>("#rubric-c1")!;
  await act(async () => setValue(box, "引出顯示列數變少的那一句。"));

  const saveButton = button("儲存 Rubric");
  expect(saveButton.disabled).toBe(true);
  expect(saveButton.getAttribute("aria-describedby")).toBe("rubric-version-reason");
  expect(container.querySelector("#rubric-version-reason")?.textContent).toContain(
    "Rubric 版本是空的",
  );
  expect(container.textContent).toContain("還不能儲存，因為 Rubric 版本是空的");

  const version = container.querySelector<HTMLInputElement>("#rubric-version")!;
  await act(async () => setValue(version, "content-007/writing/v1"));
  expect(saveButton.disabled).toBe(false);
  expect(saveButton.getAttribute("aria-describedby")).toBeNull();
  expect(container.textContent).not.toContain("還不能儲存，因為 Rubric 版本是空的");
});

test("設計 §2.4 a 確認 disabled by an unsaved edit says why, in visible text", async () => {
  stubPlatform({
    testCase: {
      ...draft,
      acceptance_criteria: [{ id: "c9", text: "輸出仍是 CSV", source: "user", confirmed_at: null }],
    },
  });
  await render();

  expect(button("確認").disabled).toBe(false);

  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#criterion-c9")!, "輸出仍是 CSV 檔"),
  );

  expect(button("確認").disabled).toBe(true);
  const describedBy = button("確認").getAttribute("aria-describedby");
  expect(describedBy, "the disabled 確認 points at no reason").toBeTruthy();
  const reason = container.querySelector(`#${describedBy}`);
  expect(reason).not.toBeNull();
  expect(reason!.textContent ?? "").toContain("現在不能確認");
  expect(reason!.getAttribute("title")).toBeNull();
});

const NOT_MINE = "44444444-4444-4444-4444-444444444444";

test("丙-116 a list filtered to a skill outside the workspace says so, and stops inviting", async () => {
  listSearch = { skill: NOT_MINE };
  stubPlatform({ testCases: [] });
  await renderList();

  const text = container.textContent ?? "";
  // Pinned whole, whitespace included: Prettier wraps this JSX text and joins
  // the wrapped lines with a single space, so a break placed after a
  // full-width comma would render as an extra space nobody typed.
  expect(text).toContain(
    "這個 Skill 不在你的工作區。Test Case 屬於工作區，所以這裡看不到它，建立表單的 Skill 選單也選不到它——先把它 Fork 一份，才會有屬於你的版本可以建立 Test Case。",
  );
  expect(text).toContain("選單也選不到它");
  expect(text).not.toContain("這一個 Skill");
  expect(text).not.toContain("還沒有 Test Case");
});

test("丙-116 a list filtered to your OWN empty skill keeps the invitation, and can now name it", async () => {
  listSearch = { skill: SKILL };
  stubPlatform({ testCases: [] });
  await renderList();

  const text = container.textContent ?? "";
  expect(text).not.toContain("這個 Skill 不在你的工作區");
  expect(text).toContain("這個 Skill 還沒有 Test Case");
  expect(text).toContain("去重複工具");
});

test("丙-121 a test case whose skill is gone says it is gone, not that you may not see it", async () => {
  listSearch = {};
  stubPlatform({ testCases: [{ ...LIST_ROW, skill_name: "" }] });
  await renderList();

  const text = container.textContent ?? "";
  expect(text).toContain("這個 Skill 已經不在你的清單裡");
  expect(text).toContain("已刪除");
  expect(text).toContain("已下架");
  expect(text).not.toContain("無權檢視");
  expect(text).not.toContain(SKILL);
});

test("§2.12 第 6 條 執行歷史 with a run still going says how old the list is and can be refreshed", async () => {
  stubPlatform({ runs: [{ ...RUN, status: "running", finished_at: undefined }] });
  await render();
  await waitFor(() => (container.textContent ?? "").includes("上次取得於"));

  const refresh = button("重新整理");
  expect(refresh, "no visible refresh control on a history with a running row").toBeTruthy();
  expect(refresh.closest("p")!.querySelector("time")).not.toBeNull();
});
