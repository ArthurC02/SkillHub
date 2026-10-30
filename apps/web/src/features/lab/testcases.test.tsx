import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { onlineManager, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { queryKeys } from "../../core/api/queryKeys";
import { TestCaseDetail } from "./test-cases/TestCaseDetail.page";
import { TestCaseList } from "./test-cases/TestCaseList.page";
import type { TestCase } from "./testcases.service";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import {
  ME,
  RUNS,
  CHECK_KINDS,
  PLATFORM_GRADER,
  WORKSPACE_GRADER,
} from "../../testing/fixtures/platform";

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

vi.mock("../evaluation/components/CheckFlow", () => ({ CheckFlow: () => null }));

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
  simulated_user_facts: [],
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
  cleanup_status: RUNS.runs[0].cleanup_status,
  evaluation: {
    value: "met",
    label: "符合",
    note: "依這次試跑當時的驗收條件判定為符合。",
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
    (req: PlatformRequest) => (req.path === "/me" ? json(ME) : undefined),
    (req: PlatformRequest) =>
      req.path === "/evaluation/check-kinds" ? json(CHECK_KINDS) : undefined,
    (req: PlatformRequest) =>
      req.path === "/evaluation/graders"
        ? json({ graders: [PLATFORM_GRADER, WORKSPACE_GRADER] })
        : undefined,
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
            skills: (over.skills ?? [{ skill_id: SKILL, name: "去重複工具", summary: "" }]).map(
              (skill) => ({
                ...skill,
                redistribution: "allowed",
                risk: {
                  scan_status: "unavailable",
                  level: "unknown",
                  warnings: 0,
                  disclosures: [],
                  note: "尚未掃描。",
                },
                verification: { value: "not_measured", label: "未測量", note: "尚未掃描。" },
              }),
            ),
            total: over.skills?.length ?? 1,
            limit: 100,
            truncated: false,
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
        note: "測試題與它上傳的檔案已移除，檔案本身也刪了；過去試跑紀錄的快照仍保留 Prompt、驗收條件，以及每個檔案的檔名與內容雜湊。",
      });
    },
    (req: PlatformRequest) =>
      removedRef.removed && req.path === `/test-cases/${TEST_CASE}`
        ? json({ error: "找不到這個測試題" }, 404)
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

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const text = () => container.textContent ?? "";

test("the 測試題 Dataset entry preserves the exact 測試題 and Version", async () => {
  listSearch = { version: VERSION };
  stubPlatform();
  await render();

  const link = container.querySelector('[data-to="/lab/test-cases/$testCaseId/datasets"]');
  expect(link?.getAttribute("data-params")).toBe(JSON.stringify({ testCaseId: TEST_CASE }));
  expect(link?.getAttribute("data-search")).toBe(JSON.stringify({ version: VERSION }));
});

test("the 測試題 workbench names the URL-selected 小工具, Version and 測試題", async () => {
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

test("the 測試題 workbench puts execution after its editable design and before history", async () => {
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

test("the 測試題 workbench keeps design primary and supporting actions in a rail", async () => {
  listSearch = { version: VERSION };
  stubPlatform({ runs: [RUN] });
  await render();

  const layout = container.querySelector(".test-case-detail-layout");
  expect(layout).not.toBeNull();
  expect(layout!.querySelector(":scope > .test-case-design")).not.toBeNull();
  expect(layout!.querySelector(":scope > .test-case-detail-rail")).not.toBeNull();
});

test("the 測試題 workbench labels a Version inherited from the latest matching 試跑紀錄", async () => {
  stubPlatform({ runs: [RUN] });
  await render();
  await waitFor(() => text().includes("沿用最近一次試跑紀錄"));

  const context = container.querySelector("[data-role='test-case-context']")!;
  expect(context.textContent).toContain("v2");
  const runLink = container.querySelector<HTMLAnchorElement>(
    '[data-to="/skills/$skillId/test-cases/$testCaseId/runs/new"]',
  );
  expect(runLink?.getAttribute("data-search")).toBe(JSON.stringify({ version: VERSION }));
});

test("the 測試題 workbench asks for a Version when no matching 試跑紀錄 can supply one", async () => {
  stubPlatform({ runs: [{ ...RUN, skill_id: OTHER_SKILL }] });
  await render();
  await waitFor(() => text().includes("尚未選擇 Version"));

  expect(text()).toContain("選擇 Version 並確認權限");
  const runLink = container.querySelector<HTMLAnchorElement>(
    '[data-to="/skills/$skillId/test-cases/$testCaseId/runs/new"]',
  );
  expect(runLink?.getAttribute("data-search")).toBe(JSON.stringify({ version: undefined }));
});

test("the 測試題 workbench refuses an URL Version outside the owner-scoped list", async () => {
  listSearch = { version: "66666666-6666-6666-6666-666666666666" };
  stubPlatform({ runs: [RUN] });
  await render();
  await waitFor(() => text().includes("Version 不屬於目前的小工具"));

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

test.each(["", "   "])(
  "a blank test-case name %j explains the disabled create action",
  async (name) => {
    listSearch = { skill: SKILL };
    stubPlatform();
    await renderList();
    await waitFor(() => container.querySelector<HTMLSelectElement>("#tc-skill")?.value === SKILL);
    await act(async () => setValue(container.querySelector<HTMLInputElement>("#tc-name")!, name));
    await act(async () =>
      setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
    );
    const create = button("建立");
    expect(create.disabled).toBe(true);
    expect(create.getAttribute("aria-describedby")).toBe("create-why");
    expect(container.querySelector("#create-why")?.textContent).toContain("填名稱");
    await act(async () =>
      setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "有效名稱"),
    );
    expect(create.disabled).toBe(false);
    expect(container.querySelector("#create-why")).toBeNull();
  },
);

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
  expect(added?.body).not.toContain('"check"');

  await act(async () => button("取消確認").click());
  const withdrawn = calls.find((c) => c.method === "PATCH" && c.url.includes("/criteria/c1"));
  expect(withdrawn?.body).toContain('"confirmed":false');

  await act(async () => button("刪除這一條").click());
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);
  await act(async () => button("確認刪除這一條").click());
  expect(calls.some((c) => c.method === "DELETE" && c.url.includes("/criteria/c1"))).toBe(true);
});

test("EVAL-014 a criterion is added with its designed check only once every node is complete", async () => {
  const calls = stubPlatform();
  await render();

  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#new-criterion")!, "提到總數"),
  );
  await waitFor(() => container.querySelector("#check-editor-root") !== null);
  await act(async () =>
    selectValue(
      container.querySelector<HTMLSelectElement>("#check-editor-root")!,
      "contains_values",
    ),
  );
  await act(async () => button("開始設計").click());
  await waitFor(() => container.textContent!.includes("有空白的值"));

  expect(button("新增").disabled).toBe(true);
  expect(container.textContent).toContain("檢查設計尚未確認可用");
  expect(container.textContent).toContain("有空白的值");

  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#check-node-0-value-0")!, " 42 "),
  );
  expect(button("新增").disabled).toBe(false);
  await act(async () => button("新增").click());

  const added = calls.find((c) => c.method === "POST" && c.url.endsWith("/criteria"));
  expect(JSON.parse(added!.body!)).toEqual({
    source: "user",
    text: "提到總數",
    check: { kind: "contains_values", values: ["42"] },
  });
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

test("the simulated person's facts are saved one per line with blank lines dropped", async () => {
  const calls = stubPlatform({ testCase: { ...draft, simulated_user_facts: ["本期用水 45 度"] } });
  await render();

  const box = container.querySelector<HTMLTextAreaElement>("#simulated-user-facts")!;
  expect(box.value).toBe("本期用水 45 度");
  await act(async () => setValue(box, " 本期用水 45 度 \n\n上期沒有欠費"));
  await act(async () => button("儲存模擬使用者").click());

  const saved = calls.find((c) => c.method === "PATCH" && c.body?.includes("simulated_user_facts"));
  expect(JSON.parse(saved?.body ?? "{}")).toEqual({
    simulated_user_facts: ["本期用水 45 度", "上期沒有欠費"],
  });
});

test("eleven facts cannot be saved and the screen says how many lines there are", async () => {
  stubPlatform();
  await render();

  const box = container.querySelector<HTMLTextAreaElement>("#simulated-user-facts")!;
  const lines = (n: number) => Array.from({ length: n }, (_, i) => `事實 ${i + 1}`).join("\n");
  await act(async () => setValue(box, lines(10)));
  expect(button("儲存模擬使用者").disabled).toBe(false);
  await act(async () => setValue(box, lines(11)));
  expect(button("儲存模擬使用者").disabled).toBe(true);
  expect(text()).toContain("目前 11 行，最多 10 行。");
});

const evaluationTextFields = [
  { name: "criterion", selector: "#criterion-c1", save: "儲存文字", cap: 2000, label: "驗收條件" },
  { name: "new criterion", selector: "#new-criterion", save: "新增", cap: 2000, label: "驗收條件" },
  {
    name: "rubric item",
    selector: "#rubric-c1",
    save: "儲存 Rubric",
    cap: 2000,
    label: "Rubric 說明",
  },
  {
    name: "rubric version",
    selector: "#rubric-version",
    save: "儲存 Rubric",
    cap: 200,
    label: "Rubric 版本",
  },
];

async function mountEvaluationTextInput(selector: string, saveLabel: string) {
  const testCase = structuredClone(draft);
  testCase.rubric = {
    version: "base/v1",
    items: [{ id: "c1", text: "原有的評分說明", evidence_required: false }],
  };
  const calls = stubPlatform({ testCase });
  await render();
  return {
    calls,
    field: container.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector)!,
    save: button(saveLabel),
  };
}

function descriptionOf(element: HTMLElement) {
  return (element.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .filter(Boolean)
    .map((id) => container.querySelector(`#${id}`)?.textContent ?? "")
    .join(" ");
}

test.each(
  evaluationTextFields.flatMap((field) =>
    [
      { shape: "ASCII cap", value: "X".repeat(field.cap), invalid: false },
      { shape: "ASCII overflow", value: "X".repeat(field.cap + 1), invalid: true },
      { shape: "astral cap", value: "𠮷".repeat(field.cap), invalid: false },
      { shape: "astral overflow", value: "𠮷".repeat(field.cap + 1), invalid: true },
    ].map((input) => ({ ...field, ...input })),
  ),
)("Evaluation text limit preserves $name $shape", async (scenario) => {
  const { field, save, calls } = await mountEvaluationTextInput(scenario.selector, scenario.save);
  await act(async () => setValue(field, scenario.value));
  expect(field.value).toBe(scenario.value);
  expect(save.disabled).toBe(scenario.invalid);
  if (scenario.invalid) {
    expect(field.getAttribute("aria-invalid")).toBe("true");
    const reason = `${scenario.label}最多 ${scenario.cap} 個字，目前 ${scenario.cap + 1} 個字。`;
    expect(descriptionOf(field)).toContain(reason);
    expect(descriptionOf(save)).toContain(reason);
  } else {
    expect([null, "false"]).toContain(field.getAttribute("aria-invalid"));
  }
  expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
});

test.each([
  { selector: "#criterion-c1", saveLabel: "儲存文字", reason: "驗收條件不能是空白" },
  { selector: "#rubric-version", saveLabel: "儲存 Rubric", reason: "Rubric 版本是空的" },
])("Evaluation required text exposes $selector blank state", async (scenario) => {
  const { field, save, calls } = await mountEvaluationTextInput(
    scenario.selector,
    scenario.saveLabel,
  );
  await act(async () => setValue(field, ""));
  expect(field.value).toBe("");
  expect(save.disabled).toBe(true);
  expect(field.getAttribute("aria-invalid")).toBe("true");
  expect(descriptionOf(field)).toContain(scenario.reason);
  expect(descriptionOf(save)).toContain(scenario.reason);
  expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
});

test("Evaluation new-entry hint does not mark untouched blank input invalid", async () => {
  const { field, save, calls } = await mountEvaluationTextInput("#new-criterion", "新增");
  expect(field.value).toBe("");
  expect(save.disabled).toBe(true);
  expect([null, "false"]).toContain(field.getAttribute("aria-invalid"));
  expect(descriptionOf(save)).toContain("欄位是空的");
  expect(calls.filter((call) => call.method !== "GET")).toEqual([]);
});

test.each([
  { name: "blank unused version", version: "", item: "" },
  { name: "oversized unused version", version: "X".repeat(201), item: "" },
  { name: "oversized whitespace-only optional item", version: "base/v1", item: " ".repeat(2001) },
])("Evaluation optional rubric removal ignores $name", async ({ version, item }) => {
  const { field, save, calls } = await mountEvaluationTextInput("#rubric-version", "儲存 Rubric");
  const itemField = container.querySelector<HTMLTextAreaElement>("#rubric-c1")!;
  await act(async () => setValue(field, version));
  await act(async () => setValue(itemField, item));
  expect(field.value).toBe(version);
  expect(itemField.value).toBe(item);
  expect(save.disabled).toBe(false);
  expect([null, "false"]).toContain(field.getAttribute("aria-invalid"));
  expect([null, "false"]).toContain(itemField.getAttribute("aria-invalid"));
  await act(async () => save.click());
  await waitFor(() => text().includes("已儲存。"));
  expect(calls.filter((call) => call.method === "PATCH")).toEqual([
    { url: `/test-cases/${TEST_CASE}`, method: "PATCH", body: JSON.stringify({ rubric: null }) },
  ]);
});

test("Evaluation version cap measures the existing trimmed wire value", async () => {
  const { field, save, calls } = await mountEvaluationTextInput("#rubric-version", "儲存 Rubric");
  const raw = `  ${"X".repeat(200)}  `;
  await act(async () => setValue(field, raw));
  expect(field.value).toBe(raw);
  expect(save.disabled).toBe(false);
  expect([null, "false"]).toContain(field.getAttribute("aria-invalid"));
  await act(async () => save.click());
  await waitFor(() => text().includes("已儲存。"));
  const patch = calls.find((call) => call.method === "PATCH");
  expect(JSON.parse(patch!.body!)).toEqual({
    rubric: {
      version: "X".repeat(200),
      items: [{ id: "c1", text: "原有的評分說明", evidence_required: false }],
    },
  });
});

const newCriterionDraftCases = [
  {
    mode: "unchanged text",
    initialCheck: false,
    edit: "none",
    expectedText: "",
    expectedCheck: null,
  },
  {
    mode: "unchanged check",
    initialCheck: true,
    edit: "none",
    expectedText: "",
    expectedCheck: null,
  },
  {
    mode: "newer text",
    initialCheck: true,
    edit: "text",
    expectedText: "下一條草稿",
    expectedCheck: " 42 ",
  },
  {
    mode: "newer check",
    initialCheck: false,
    edit: "check",
    expectedText: "已送出的這一條",
    expectedCheck: "下一條的值",
  },
  {
    mode: "cleared text",
    initialCheck: true,
    edit: "clear",
    expectedText: "",
    expectedCheck: " 42 ",
  },
  {
    mode: "removed check",
    initialCheck: true,
    edit: "remove",
    expectedText: "已送出的這一條",
    expectedCheck: null,
  },
  {
    mode: "reverted text",
    initialCheck: true,
    edit: "revert",
    expectedText: "已送出的這一條",
    expectedCheck: " 42 ",
  },
  {
    mode: "failed save",
    initialCheck: true,
    edit: "none",
    expectedText: "已送出的這一條",
    expectedCheck: " 42 ",
  },
  {
    mode: "failed newer text",
    initialCheck: true,
    edit: "text",
    expectedText: "下一條草稿",
    expectedCheck: " 42 ",
  },
];

test.each(newCriterionDraftCases)(
  "new criterion retains draft ownership for $mode",
  async ({ mode, initialCheck, edit, expectedText, expectedCheck }) => {
    const current = structuredClone(draft);
    const calls = stubPlatform({ testCase: current });
    const platformFetch = globalThis.fetch;
    const writes: unknown[] = [];
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const failed = mode.startsWith("failed");
    vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
      if (init?.method !== "POST" || !String(input).endsWith(`/test-cases/${TEST_CASE}/criteria`)) {
        return platformFetch(input, init);
      }
      const body = JSON.parse(init.body as string);
      writes.push(body);
      await held;
      if (failed) return json({ error: "新增驗收條件失敗" }, 500);
      current.acceptance_criteria.push({
        id: "c-added",
        text: body.text,
        source: "user",
        confirmed_at: null,
      });
      return json(current, 201);
    });
    try {
      await render();
      const field = container.querySelector<HTMLInputElement>("#new-criterion")!;
      await act(async () => setValue(field, "已送出的這一條"));
      if (initialCheck) {
        await act(async () => button("開始設計").click());
        await waitFor(() => container.querySelector("#check-node-0-value-0") !== null);
        await act(async () =>
          setValue(container.querySelector<HTMLInputElement>("#check-node-0-value-0")!, " 42 "),
        );
      }
      expect(button("新增").disabled).toBe(false);
      await act(async () => button("新增").click());
      await waitFor(() => writes.length === 1);
      await waitFor(() => text().includes("新增中…"));
      expect(button("新增中…").disabled).toBe(true);
      expect(field.disabled).toBe(false);
      if (edit === "text" || edit === "clear") {
        await act(async () => setValue(field, expectedText));
      } else if (edit === "revert") {
        await act(async () => setValue(field, "暫時的改動"));
        await act(async () => setValue(field, "已送出的這一條"));
      } else if (edit === "check") {
        await act(async () => button("開始設計").click());
        await waitFor(() => container.querySelector("#check-node-0-value-0") !== null);
        await act(async () =>
          setValue(
            container.querySelector<HTMLInputElement>("#check-node-0-value-0")!,
            "下一條的值",
          ),
        );
      } else if (edit === "remove") {
        await act(async () => button("拿掉整個檢查").click());
      }
      await act(async () => release());
      if (failed) await waitFor(() => text().includes("尚未確認這一條驗收條件是否已新增"));
      else await waitFor(() => container.querySelector("#criterion-c-added") !== null);
      await waitFor(() => !text().includes("新增中…"));
      expect(field.value).toBe(expectedText);
      const checkField = container.querySelector<HTMLInputElement>("#check-node-0-value-0");
      if (expectedCheck === null) expect(checkField).toBeNull();
      else expect(checkField?.value).toBe(expectedCheck);
      expect(button("新增").disabled).toBe(expectedText === "");
      expect(writes).toEqual([
        {
          source: "user",
          text: "已送出的這一條",
          ...(initialCheck ? { check: { kind: "contains_values", values: ["42"] } } : {}),
        },
      ]);
      expect(
        calls.filter(
          (call) =>
            call.method === "GET" && call.url.split("?")[0].endsWith(`/test-cases/${TEST_CASE}`),
        ),
      ).toHaveLength(2);
      const receipt = Array.from(container.querySelectorAll('[role="status"]')).find((node) =>
        node.textContent?.includes("已新增驗收條件。"),
      );
      if (failed) {
        expect(receipt).toBeUndefined();
        const alert = container.querySelector('[role="alert"]')!;
        const editor = container.querySelector('[aria-label="檢查方式與設計"]')!;
        expect(alert.compareDocumentPosition(editor) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(4);
      } else
        expect(receipt?.textContent).toBe(
          edit === "none" ? "已新增驗收條件。" : "已新增驗收條件。目前欄位裡的草稿尚未新增。",
        );
    } finally {
      release();
    }
  },
);

const criterionOwnershipCases = [
  { mode: "pristine-reread", expected: "伺服器已更新的驗收條件", dirty: false, enabled: false },
  { mode: "canonical-save", expected: "已儲存的內容", dirty: false, enabled: false },
  { mode: "dirty-reread", expected: "未送出的自己的草稿", dirty: true, enabled: true },
  { mode: "pending-typing", expected: "正在編輯的下一稿", dirty: true, enabled: true },
  { mode: "pending-revert", expected: "輸出的列數少於輸入", dirty: true, enabled: true },
  { mode: "empty-reread", expected: "", dirty: true, enabled: false },
  { mode: "unchanged-reread", expected: "輸出的列數少於輸入", dirty: false, enabled: false },
  { mode: "failed-save", expected: "  已儲存的內容  ", dirty: true, enabled: true },
];

test.each(criterionOwnershipCases)(
  "criterion draft ownership survives $mode",
  async ({ mode, expected, dirty, enabled }) => {
    const current = structuredClone(draft);
    current.acceptance_criteria.push({
      id: "c2",
      text: "沒有刪掉原始欄位",
      source: "suggested",
      confirmed_at: null,
    });
    const calls = stubPlatform({ testCase: current });
    const platformFetch = globalThis.fetch;
    const writes: { criterion: string; body: unknown }[] = [];
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
      const url = String(input);
      if (init?.method !== "PATCH" || !url.includes(`/test-cases/${TEST_CASE}/criteria/`)) {
        return platformFetch(input, init);
      }
      const criterion = url.split("/").at(-1)!;
      const body: unknown = JSON.parse(init.body as string);
      writes.push({ criterion, body });
      if (mode === "failed-save") return json({ error: "暫時無法更新驗收條件" }, 503);
      if (criterion === "c1") {
        if (mode.startsWith("pending-")) await held;
        current.acceptance_criteria[0].text = "已儲存的內容";
        current.acceptance_criteria[0].confirmed_at = null;
      } else {
        current.acceptance_criteria[1].confirmed_at = "2026-08-18T01:00:00Z";
        if (mode !== "unchanged-reread") {
          current.acceptance_criteria[0].text = "伺服器已更新的驗收條件";
          current.acceptance_criteria[0].confirmed_at = null;
        }
      }
      current.updated_at = "2026-08-18T01:00:00Z";
      return json(current);
    });
    try {
      await render();
      const field = container.querySelector<HTMLInputElement>("#criterion-c1")!;
      const row = field.closest("li")!;
      expect(field.value).toBe("輸出的列數少於輸入");
      expect(button("儲存文字").disabled).toBe(true);
      const saving =
        mode === "canonical-save" || mode.startsWith("pending-") || mode === "failed-save";
      if (saving) {
        await act(async () => setValue(field, "  已儲存的內容  "));
        await act(async () => button("儲存文字").click());
        await waitFor(() => writes.length === 1);
        if (mode.startsWith("pending-")) {
          await act(async () => setValue(field, expected));
          await act(async () => release());
        }
        if (mode === "failed-save")
          await waitFor(() => row.querySelector('[role="alert"]') !== null);
        else await waitFor(() => row.textContent?.includes("狀態：尚未確認") === true);
      } else {
        if (dirty) await act(async () => setValue(field, expected));
        await act(async () => button("確認").click());
        await waitFor(
          () =>
            container
              .querySelector("#criterion-c2")
              ?.closest("li")
              ?.textContent?.includes("狀態：已確認") === true,
        );
      }
      await waitFor(() => row.textContent?.includes("儲存中…") === false);
      expect(field.value).toBe(expected);
      expect(button("儲存文字").disabled).toBe(!enabled);
      expect(row.querySelector("#criterion-edited-c1") !== null).toBe(dirty);
      if (mode === "failed-save") {
        expect(row.querySelector('[role="alert"]')?.textContent).toBe(
          "操作結果尚未確認；請先確認最新資料，再決定是否重送。",
        );
      }
      if (mode === "empty-reread") expect(row.textContent).toContain("驗收條件不能是空白");
      expect(writes).toEqual([
        saving
          ? { criterion: "c1", body: { text: "  已儲存的內容  " } }
          : { criterion: "c2", body: { confirmed: true } },
      ]);
      expect(
        calls.filter(
          (call) =>
            call.method === "GET" && call.url.split("?")[0].endsWith(`/test-cases/${TEST_CASE}`),
        ),
      ).toHaveLength(2);
    } finally {
      release();
    }
  },
);

test.each([
  { name: "plain boundary", input: "字".repeat(1500), invalid: false, problems: [] },
  {
    name: "plain overflow",
    input: "字".repeat(1501),
    invalid: true,
    problems: ["目前合計 1501 個字，最多 1500 個字。"],
  },
  { name: "astral boundary", input: "𠮷".repeat(1500), invalid: false, problems: [] },
  {
    name: "astral overflow",
    input: "𠮷".repeat(1501),
    invalid: true,
    problems: ["目前合計 1501 個字，最多 1500 個字。"],
  },
  {
    name: "trimmed aggregate boundary",
    input: `  ${"甲".repeat(750)}  \n\n ${"乙".repeat(750)}  `,
    invalid: false,
    problems: [],
  },
  {
    name: "aggregate overflow",
    input: `${"甲".repeat(750)}\n${"乙".repeat(751)}`,
    invalid: true,
    problems: ["目前合計 1501 個字，最多 1500 個字。"],
  },
  {
    name: "both limits exceeded",
    input: `${"字".repeat(1491)}\n${Array(10).fill("乙").join("\n")}`,
    invalid: true,
    problems: ["目前 11 行，最多 10 行。", "目前合計 1501 個字，最多 1500 個字。"],
  },
  { name: "blank optional facts", input: "  \n\n ", invalid: false, problems: [] },
])("simulated facts expose accurate validation for $name", async ({ input, invalid, problems }) => {
  const calls = stubPlatform();
  await render();
  const box = container.querySelector<HTMLTextAreaElement>("#simulated-user-facts")!;
  await act(async () => setValue(box, input));
  expect(box.value).toBe(input);
  expect(button("儲存模擬使用者").disabled).toBe(invalid);
  if (invalid) expect(box.getAttribute("aria-invalid")).toBe("true");
  else expect([null, "false"]).toContain(box.getAttribute("aria-invalid"));
  expect(box.getAttribute("aria-describedby")).toBe("simulated-user-limit");
  const note = container.querySelector("#simulated-user-limit")!;
  expect(note.getAttribute("role")).toBe(invalid ? "status" : null);
  for (const problem of problems) expect(note.textContent).toContain(problem);
  expect(calls.filter((call) => call.method === "PATCH")).toHaveLength(0);
});

test("a changed prompt is not labelled as saved by the previous request", async () => {
  stubPlatform();
  await render();

  await act(async () => button("儲存").click());
  await waitFor(() => text().includes("已儲存。"));
  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#edit-name")!, "新的名稱"),
  );

  expect(text()).not.toContain("已儲存。");
});

test("a changed rubric is not labelled as saved by the previous request", async () => {
  stubPlatform();
  await render();

  await act(async () => button("儲存 Rubric").click());
  await waitFor(() => text().includes("已儲存。"));
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#rubric-c1")!, "新的判定文字"),
  );

  expect(text()).not.toContain("已儲存。");
});

test("CONTENT-007 clearing every line removes the rubric rather than storing an empty one", async () => {
  const calls = stubPlatform();
  await render();

  expect(container.textContent).toContain("儲存等於移除這個測試題的 rubric");
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

const criteriaOf = (n: number): TestCase["acceptance_criteria"] =>
  Array.from({ length: n }, (_, i) => ({
    id: `c${i}`,
    text: `條件 ${i}`,
    source: "user" as const,
    confirmed_at: null,
  }));

test.each([
  [20, false],
  [21, true],
])("a test case with %i criteria warns that only the first 20 are judged: %s", async (n, warns) => {
  stubPlatform({ testCase: { ...draft, acceptance_criteria: criteriaOf(n) } });
  await render();

  expect((container.textContent ?? "").includes("評估只判定前 20 條")).toBe(warns);
});

test.each([
  [19, false],
  [20, true],
])("a test case with %i criteria blocks adding and suggesting: %s", async (n, full) => {
  stubPlatform({ testCase: { ...draft, acceptance_criteria: criteriaOf(n) } });
  await render();

  expect(button("請系統建議（選用）").disabled).toBe(full);
  expect((container.textContent ?? "").includes("刪掉幾條才能再新增或請系統建議")).toBe(full);
});

test("a 413 from suggesting criteria says the list is full and nothing was charged", async () => {
  stubPlatform({
    testCase: { ...draft, acceptance_criteria: criteriaOf(19) },
    suggest: { status: 413, error: "server words" },
  });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("這次沒有呼叫模型、也沒有扣點"));
  expect(container.textContent).toContain("已經有 20 條驗收條件");
  expect(container.textContent).not.toContain("server words");
});

test("TEST-002 an adopted suggestion is listed as 系統建議，尚未確認", async () => {
  const withSuggested: TestCase = {
    ...draft,
    acceptance_criteria: [
      { id: "c9", text: "保留欄位標題列", source: "suggested", confirmed_at: null },
    ],
  };
  stubPlatform({ testCase: withSuggested });
  await render();

  expect(container.textContent).toContain("系統建議，尚未確認");
});

test("TEST-002 a 429 on suggestions shows the server's wait time", async () => {
  stubPlatform({ suggest: { status: 429, error: "太多請求了，請在 4 秒後再試。" } });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("太多請求了，請在 4 秒後再試。"));
  expect(container.textContent).not.toContain("無法取得建議，可以再試一次。");
});

test("TEST-002 a 503 is worded as unavailable and names the manual path, without the server's own words", async () => {
  stubPlatform({ suggest: { status: 503, error: "目前無法自動建議驗收條件，請自己手動輸入" } });
  await render();

  await act(async () => button("請系統建議（選用）").click());
  await waitFor(() => (container.textContent ?? "").includes("目前無法自動建議"));
  expect(container.textContent).toContain("驗收條件可以自己手動輸入");
  expect(container.textContent).not.toContain("目前無法自動建議驗收條件，請自己手動輸入");
});

test.each([
  [403, "封測受邀", "not on the invite list"],
  [422, "這次沒有呼叫模型、也沒有扣點", "點數不足，無法開始建議驗收條件"],
])(
  "a %i from suggesting criteria says what happened and names the manual path",
  async (status, sentence, serverWords) => {
    stubPlatform({ suggest: { status, error: serverWords } });
    await render();

    await act(async () => button("請系統建議（選用）").click());
    await waitFor(() => (container.textContent ?? "").includes(sentence));
    expect(container.textContent).toContain("驗收條件可以自己手動輸入");
    expect(container.textContent).not.toContain(serverWords);
  },
);

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

test("a refused create shows the server's reason for any 4xx, not only the ones a page listed", async () => {
  stubPlatform({ create: { status: 404, error: "找不到這個 Test Case" } });
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () => setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "名稱"));
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );
  await act(async () => button("建立").click());
  await waitFor(() => (container.textContent ?? "").includes("找不到這個 Test Case"));

  expect(container.textContent).not.toContain("建立沒有成功");
});

test("a create whose answer was lost re-reads the list and warns that pressing again may add a second one", async () => {
  const calls = stubPlatform({ create: { status: 503, error: "upstream unavailable" } });
  await renderList();
  const listReads = () =>
    calls.filter((c) => c.method === "GET" && c.url.split("?")[0] === "/test-cases").length;

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () => setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "名稱"));
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );
  const readsBefore = listReads();
  await act(async () => button("建立").click());
  await waitFor(() => (container.textContent ?? "").includes("尚未確認測試題是否已建立"));
  await waitFor(() => listReads() > readsBefore);

  expect(container.textContent).not.toContain("建立沒有成功");
  expect(container.textContent).not.toContain("upstream unavailable");
});

test("名稱超過 200 個字送出前先擋下，不送出請求", async () => {
  const calls = stubPlatform();
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "長".repeat(201)),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );

  expect(container.textContent).toContain("名稱最多 200 個字，目前 201 個字。");
  expect(button("建立").disabled).toBe(true);
  expect(calls.some((c) => c.method === "POST" && c.url === "/test-cases")).toBe(false);
});

test("名稱剛好 200 個字（600 bytes）不擋，建立按鈕不停用", async () => {
  stubPlatform();
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "長".repeat(200)),
  );
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "prompt"),
  );

  expect(container.textContent).not.toContain("名稱最多 200 個字，目前");
  expect(button("建立").disabled).toBe(false);
});

test("Prompt 超過 32768 個字送出前先擋下，不送出請求", async () => {
  const calls = stubPlatform();
  await renderList();

  await act(async () =>
    selectValue(container.querySelector<HTMLSelectElement>("#tc-skill")!, SKILL),
  );
  await act(async () => setValue(container.querySelector<HTMLInputElement>("#tc-name")!, "名稱"));
  await act(async () =>
    setValue(container.querySelector<HTMLTextAreaElement>("#tc-prompt")!, "長".repeat(32769)),
  );

  expect(container.textContent).toContain("Prompt 最多 32768 個字，目前 32769 個字。");
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
    (d.querySelector("summary")?.textContent ?? "").includes("小工具 Version"),
  );
  expect(fold, "the 小工具 Version id is not behind a disclosure").toBeTruthy();
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
  expect(compareLinks.every((link) => link.textContent?.includes("以這次試跑開始比較"))).toBe(true);
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
  expect(container.textContent).toContain("開始試跑前會再次顯示權限摘要並要求確認");
  expect(container.textContent).not.toContain("還需要填入");
});

test("02:WS-002 deleting states its scope before it runs and its actual reach after", async () => {
  const calls = stubPlatform({ runs: [] });
  await render();

  await act(async () => button("刪除整個測試題").click());
  expect(container.textContent).toContain("已經跑過的試跑紀錄及其快照不受影響");
  expect(calls.some((c) => c.method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除整個測試題").click());
  await waitFor(() => (container.textContent ?? "").includes("已刪除這個測試題"));
  const sent = calls.find((c) => c.method === "DELETE");
  expect(sent?.url).toContain(`/test-cases/${TEST_CASE}`);

  expect(container.textContent).toContain("2 個上傳檔案");
  expect(container.textContent).toContain("快照與歷史試跑紀錄不受影響");
  expect(container.textContent).toContain("回到測試題列表");
});

test("02:WS-002 deleting a test case does not ask the server for it again", async () => {
  const calls = stubPlatform({ runs: [] });
  await render();
  const fetched = () =>
    calls.filter((c) => c.method === "GET" && c.url === `/test-cases/${TEST_CASE}`).length;
  const before = fetched();

  await act(async () => button("刪除整個測試題").click());
  await act(async () => button("確認刪除整個測試題").click());
  await waitFor(() => (container.textContent ?? "").includes("已刪除這個測試題"));

  expect(fetched()).toBe(before);
  expect(container.textContent).not.toContain("找不到這個測試題");
});

const LIST_ROW = {
  ...draft,
  skill_name: "去重複工具",
  criteria_confirmed: 1,
  criteria_total: 3,
  has_rubric: true,
};

test.each(["", "   "])("a blank scenario name %j keeps its list link named", async (name) => {
  stubPlatform({ testCases: [{ ...LIST_ROW, name }] });
  await renderList();

  expect(container.querySelector(".search-result a")?.textContent).toBe("未命名的測試題");
});

test("列表 shows the skill's name, the confirmed count and whether a rubric exists", async () => {
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  expect(container.textContent).toContain("去重複工具");
  expect(container.textContent).toContain("已確認 1/3 條");
  expect(container.textContent).toContain("Rubric 有");
  expect(container.textContent).not.toContain(SKILL);
});

test("列表 separates the scenario index from the 測試題 editor", async () => {
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  const workspace = container.querySelector(".test-case-workspace");
  expect(workspace?.children).toHaveLength(2);
  expect(workspace?.children[0]?.classList.contains("test-case-index")).toBe(true);
  expect(workspace?.children[1]?.classList.contains("test-case-editor")).toBe(true);
});

test("列表在長清單前提供直達新情境編輯器的連結", async () => {
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();

  expect(container.querySelector(".test-case-create-jump")?.getAttribute("href")).toBe(
    "#new-test-case",
  );
  expect(container.querySelector(".test-case-editor")?.id).toBe("new-test-case");
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

test("列表 ?skill= 指名的小工具要預先填進建立表單，不要再問一次", async () => {
  listSearch = { skill: SKILL };
  stubPlatform({ testCases: [LIST_ROW] });
  await renderList();
  await waitFor(() => container.querySelector<HTMLSelectElement>("#tc-skill")?.value === SKILL);

  expect(container.querySelector<HTMLSelectElement>("#tc-skill")!.value).toBe(SKILL);
  expect(container.textContent).not.toContain("還不能建立，因為：選一個小工具");
});

test("creating a 測試題 for the current 小工具 keeps the selected version", async () => {
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

test("creating a 測試題 for another 小工具 drops the previous 小工具's version", async () => {
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
    "這個小工具不在你的工作區。測試題屬於工作區，所以這裡看不到它，建立表單的小工具選單也選不到它——先把它複製一份，才會有屬於你的版本可以建立測試題。",
  );
  expect(text).toContain("選單也選不到它");
  expect(text).not.toContain("這一個小工具");
  expect(text).not.toContain("還沒有測試題");
});

test("丙-116 a list filtered to your OWN empty skill keeps the invitation, and can now name it", async () => {
  listSearch = { skill: SKILL };
  stubPlatform({ testCases: [] });
  await renderList();

  const text = container.textContent ?? "";
  expect(text).not.toContain("這個小工具不在你的工作區");
  expect(text).toContain("這個小工具還沒有測試題");
  expect(text).toContain("去重複工具");
});

test("丙-121 a test case whose skill is gone says it is gone, not that you may not see it", async () => {
  listSearch = {};
  stubPlatform({ testCases: [{ ...LIST_ROW, skill_name: "" }] });
  await renderList();

  const text = container.textContent ?? "";
  expect(text).toContain("這個小工具已經不在你的清單裡");
  expect(text).toContain("已刪除");
  expect(text).toContain("已下架");
  expect(text).not.toContain("無權檢視");
  expect(text).not.toContain(SKILL);
});

test("§2.12 第 6 條 執行歷史 with a run still going says how old the list is and can be refreshed", async () => {
  stubPlatform({ runs: [{ ...RUN, status: "running", finished_at: undefined }] });
  await render();
  const history = container.querySelector(".test-case-history-panel")!;
  await waitFor(() => (history.textContent ?? "").includes("上次取得於"));

  const refresh = Array.from(history.querySelectorAll<HTMLButtonElement>("button")).find(
    (candidate) => candidate.textContent === "重新整理",
  );
  expect(refresh, "no visible refresh control on a history with a running row").toBeTruthy();
  expect(refresh!.closest("p")!.querySelector("time")).not.toBeNull();
});

const datasetReadinessCases = ["fetching", "paused"].flatMap((phase) =>
  [false, true].map((empty) => ({ phase, empty })),
);

test.each(datasetReadinessCases)(
  "dataset detail waits during $phase with cached empty=$empty and retains unrelated drafts",
  async ({ phase, empty }) => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    stubPlatform();
    const platformFetch = globalThis.fetch;
    const none = { datasets: [], total_bytes: 0 };
    const stored = {
      datasets: [
        {
          dataset_id: "77777777-7777-4777-8777-777777777777",
          file_name: "cached.csv",
          content_type: "text/csv",
          content_hash: "a".repeat(64),
          size_bytes: 1,
          expires_at: "2026-11-15T00:00:00Z",
        },
      ],
      total_bytes: 1,
    };
    let response = empty ? none : stored;
    let checking = false;
    let reads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
      if (!String(input).endsWith(`/test-cases/${TEST_CASE}/datasets`))
        return platformFetch(input, init);
      reads++;
      if (checking) await gate;
      return json(response);
    });
    await render();
    const section = container.querySelector<HTMLElement>('[aria-label="測試資料"]')!;
    await waitFor(() =>
      empty
        ? section.textContent?.includes("還沒有上傳任何檔案。") === true
        : section.querySelectorAll(".file-tree > li").length === 1,
    );
    const name = container.querySelector<HTMLInputElement>("#edit-name")!;
    await act(async () => setValue(name, "kept while checking datasets"));
    try {
      await act(async () => {
        checking = true;
        if (phase === "paused") onlineManager.setOnline(false);
        void queryClient.invalidateQueries({ queryKey: queryKeys.testCases.datasets(TEST_CASE) });
      });
      const waiting = phase === "paused" ? "等待連線以讀取檔案清單" : "載入檔案清單中";
      await waitFor(() => section.textContent?.includes(waiting) === true);
      expect(section.textContent).not.toContain("還沒有上傳任何檔案。");
      expect(section.textContent).not.toMatch(/目前 \d+ 個檔案/);
      expect(section.querySelectorAll(".file-tree > li")).toHaveLength(0);
      expect(reads).toBe(phase === "paused" ? 1 : 2);
      expect(name.value).toBe("kept while checking datasets");
    } finally {
      await act(async () => {
        response = empty ? stored : none;
        checking = false;
        release();
        onlineManager.setOnline(true);
      });
    }
    await waitFor(() => section.querySelectorAll(".file-tree > li").length === (empty ? 1 : 0));
    await waitFor(() => queryClient.isFetching() === 0);
    expect(reads).toBe(2);
    expect(name.value).toBe("kept while checking datasets");
    await waitFor(() =>
      Array.from(section.querySelectorAll("button")).some(
        (candidate) => candidate.textContent === "重新整理",
      ),
    );
    const refresh = Array.from(section.querySelectorAll<HTMLButtonElement>("button")).find(
      (candidate) => candidate.textContent === "重新整理",
    )!;
    expect(refresh).toBeTruthy();
    await act(async () => refresh.click());
    await waitFor(() => reads === 3 && document.activeElement?.id === "test-case-datasets-heading");
    expect(name.value).toBe("kept while checking datasets");
  },
);
