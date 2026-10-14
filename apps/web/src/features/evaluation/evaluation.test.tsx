import { StrictMode, act, useState, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";
import { CHECK_KINDS, PLATFORM_GRADER, WORKSPACE_GRADER } from "../../testing/fixtures/platform";
import { LazyCheckEditor as CheckEditor } from "./components/LazyCheckEditor";
import { GraderManager } from "./components/GraderManager";
import type { CheckNode } from "./check.model";
import { useCheckReady } from "./evaluation.service";
import { queryKeys } from "../../core/api/queryKeys";

vi.mock("./components/CheckFlow", () => ({ CheckFlow: () => null }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children?: ReactNode; to: string }) => <a data-to={to}>{children}</a>,
}));

let container: HTMLDivElement;
let root: Root;
let calls: { url: string; method: string; body?: string }[];
let refusal: { status: number; error: string } | undefined;

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

beforeEach(() => {
  queryClient.clear();
  calls = [];
  refusal = undefined;
  container = document.createElement("div");
  document.body.appendChild(container);
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const method = init?.method ?? "GET";
    calls.push({ url, method, body: init?.body as string | undefined });
    if (method === "POST")
      return refusal ? json({ error: refusal.error }, refusal.status) : json(WORKSPACE_GRADER, 201);
    if (url === "/me") return json({ operator: true });
    if (url === "/evaluation/check-kinds") return json(CHECK_KINDS);
    if (url === "/evaluation/graders")
      return json({ graders: [PLATFORM_GRADER, WORKSPACE_GRADER] });
    if (url === "/admin/evaluation/graders") return json({ graders: [PLATFORM_GRADER] });
    return json({ error: "not stubbed" }, 404);
  });
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function renderNode(node: ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
}

function waitFor(done: () => boolean) {
  return pollUntil(done, () => container.textContent, DEFAULT_WAIT_MS);
}

function button(label: string): HTMLButtonElement {
  const found = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === label,
  );
  if (!found) throw new Error(`no ${label} button; DOM was:\n${container.textContent}`);
  return found;
}

function field<T extends HTMLElement>(selector: string): T {
  const found = container.querySelector<T>(selector);
  if (!found) throw new Error(`no ${selector}; DOM was:\n${container.textContent}`);
  return found;
}

function setValue(
  input: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement,
  value: string,
) {
  const proto =
    input instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : input instanceof HTMLSelectElement
        ? HTMLSelectElement.prototype
        : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(proto, "value")?.set?.call(input, value);
  input.dispatchEvent(
    new Event(input instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }),
  );
}

let latest: CheckNode | null;

function Harness({ initial }: { initial: CheckNode | null }) {
  const [value, setTree] = useState(initial);
  return (
    <CheckEditor
      value={value}
      onChange={(next) => {
        latest = next;
        setTree(next);
      }}
    />
  );
}

async function renderEditor(initial: CheckNode | null = null) {
  latest = initial;
  await renderNode(<Harness initial={initial} />);
  await waitFor(() => container.querySelector("[data-loading]") === null);
}

function ReadyEditor() {
  const [value, setValue] = useState<CheckNode | null>({
    kind: "contains_values",
    values: ["preserved value"],
  });
  const ready = useCheckReady(value);
  return (
    <>
      <output data-check-ready>{String(ready)}</output>
      <CheckEditor value={value} onChange={setValue} />
    </>
  );
}

test("a failed check-kind refresh cannot approve a cached design and retry preserves its values", async () => {
  await renderNode(<ReadyEditor />);
  await waitFor(() => container.querySelector("[data-check-ready]")?.textContent === "true");
  await waitFor(
    () =>
      container.querySelector<HTMLInputElement>('[id$="-value-0"]')?.value === "preserved value",
  );
  const previousFetch = globalThis.fetch;
  let failing = true;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/evaluation/check-kinds") && failing)
      return json({ error: "service unavailable" }, 503);
    return previousFetch(input, init);
  });
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.checks.kinds });
  });
  await waitFor(() => container.querySelector('[role="alert"]') !== null);
  expect(container.querySelector("[data-check-ready]")?.textContent).toBe("false");
  expect(container.querySelector(".check-editor")).toBeNull();
  failing = false;
  await act(async () => button("再試一次").click());
  await waitFor(() => container.querySelector(".check-editor") !== null);
  expect(container.querySelector<HTMLInputElement>('[id$="-value-0"]')?.value).toBe(
    "preserved value",
  );
  expect(container.querySelector("[data-check-ready]")?.textContent).toBe("true");
  expect(document.activeElement?.getAttribute("aria-label")).toBe("檢查方式與設計");
});

test("a failed grader refresh hides stale names without replacing the selected immutable version", async () => {
  await renderEditor({ kind: "prompt_grader", values: [WORKSPACE_GRADER.version_id] });
  await waitFor(() => container.textContent!.includes("語氣 v2"));
  const previousFetch = globalThis.fetch;
  let failing = true;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (String(input).endsWith("/evaluation/graders") && failing)
      return json({ error: "service unavailable" }, 503);
    return previousFetch(input, init);
  });
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.checks.graders("workspace") });
  });
  await waitFor(() => container.querySelector('[role="alert"]') !== null);
  const picker = container.querySelector<HTMLSelectElement>('[id$="-grader"]')!;
  expect(picker.disabled).toBe(true);
  expect(picker.value).toBe(WORKSPACE_GRADER.version_id);
  expect(container.textContent).not.toContain("語氣 v2");
  expect(
    container.querySelector(`#${picker.getAttribute("aria-describedby")}`)?.textContent,
  ).toContain("暫時無法讀取評分器");
  failing = false;
  await act(async () => button("再試一次").click());
  await waitFor(() => !picker.disabled);
  expect(picker.value).toBe(WORKSPACE_GRADER.version_id);
  expect(container.textContent).toContain("語氣 v2");
});

test("an empty composite is flagged, and a child added to it becomes the selected node", async () => {
  await renderEditor();
  await act(async () => setValue(field("#check-editor-root"), "sequence"));
  await act(async () => button("開始設計").click());
  await waitFor(() => container.textContent!.includes("要 1 到 50 個子節點"));

  expect(latest).toEqual({ kind: "sequence", children: [] });
  expect(container.textContent).toContain("要 1 到 50 個子節點，現在 0 個");

  await act(async () => setValue(field("#check-node-0-add"), "judge"));
  await act(async () => button("加入").click());

  expect(latest).toEqual({ kind: "sequence", children: [{ kind: "judge" }] });
  expect(field("#check-node-0-0-heading").textContent).toBe("選取的節點：評審判定");
  expect(container.textContent).not.toContain("要 1 到 50 個子節點");
});

test("the graders are asked for only once a check is being designed", async () => {
  await renderEditor();
  expect(calls.map((c) => c.url)).not.toContain("/evaluation/graders");

  await act(async () => setValue(field("#check-editor-root"), "judge"));
  await act(async () => button("開始設計").click());
  await waitFor(() => calls.some((c) => c.url === "/evaluation/graders"));
});

test("a prompt grader picks a version from both the platform's and the workspace's graders", async () => {
  await renderEditor({ kind: "prompt_grader", values: [""] });
  await waitFor(() => container.textContent!.includes("語氣 v2"));

  const options = Array.from(field<HTMLSelectElement>("#check-node-0-grader").options).map(
    (o) => o.textContent,
  );
  expect(options).toEqual(["選一個評分器", "引用出處 v1（平台通用）", "語氣 v2（這個 Workspace）"]);

  await act(async () => setValue(field("#check-node-0-grader"), WORKSPACE_GRADER.version_id));
  expect(latest).toEqual({ kind: "prompt_grader", values: [WORKSPACE_GRADER.version_id] });
});

test("a check keeps its pinned grader version when the latest list does not contain it", async () => {
  await renderEditor({ kind: "prompt_grader", values: ["11111111-2222-4333-8444-555555555555"] });
  await waitFor(() => container.textContent!.includes("語氣 v2"));
  const picker = field<HTMLSelectElement>("#check-node-0-grader");
  expect(picker.value).toBe("11111111-2222-4333-8444-555555555555");
  expect(picker.selectedOptions[0].textContent).toBe("已指定的版本（名稱未知）");
  expect(field("#check-node-0-heading").textContent).toContain("已指定版本（名稱未知）");
  expect(latest).toEqual({
    kind: "prompt_grader",
    values: ["11111111-2222-4333-8444-555555555555"],
  });
});

test("a fan-out needing more passes than it has values is flagged; at the count it is not", async () => {
  await renderEditor({
    kind: "each",
    values: ["a", "b"],
    children: [{ kind: "judge" }],
    at_least: 3,
  });
  expect(container.textContent).toContain("至少通過數要在 0 到 2 之間");

  await act(async () => setValue(field("#check-node-0-at-least"), "2"));
  expect(latest?.at_least).toBe(2);
  expect(container.textContent).not.toContain("至少通過數要在");
});

test("removing the root leaves the criterion without a check", async () => {
  await renderEditor({ kind: "judge" });
  await act(async () => button("拿掉整個檢查").click());
  expect(latest).toBeNull();
  expect(field("#check-editor-root")).toBeTruthy();
});

test("a workspace revises only its own grader, and only with a changed text and a reason", async () => {
  await renderNode(<GraderManager scope="workspace" />);
  await waitFor(() => container.textContent!.includes("語氣 v2"));

  expect(container.querySelector(`#grader-${PLATFORM_GRADER.id}-instructions`)).toBeNull();
  expect(container.textContent).toContain(PLATFORM_GRADER.instructions);

  const save = button("存成新版本");
  expect(save.disabled).toBe(true);
  expect(container.textContent).toContain("評分指示沒有變更");

  await act(async () =>
    setValue(field(`#grader-${WORKSPACE_GRADER.id}-instructions`), "太冷淡就不通過"),
  );
  expect(save.disabled).toBe(true);
  await act(async () => setValue(field(`#grader-${WORKSPACE_GRADER.id}-reason`), "更短"));
  expect(save.disabled).toBe(false);
  await act(async () => save.click());

  const sent = calls.find((c) => c.method === "POST");
  expect(sent?.url).toBe(`/evaluation/graders/${WORKSPACE_GRADER.id}/versions`);
  expect(JSON.parse(sent!.body!)).toEqual({ instructions: "太冷淡就不通過", reason: "更短" });
});

test("an operator's new grader goes to the platform list, and a refused name says why", async () => {
  refusal = { status: 409, error: "已經有同名的評分器" };
  await renderNode(<GraderManager scope="platform" />);
  await waitFor(() => container.textContent!.includes("引用出處 v1"));

  const create = button("新增評分器");
  await act(async () => setValue(field("#grader-create-name"), "引用出處"));
  await act(async () => setValue(field("#grader-create-instructions"), "要附出處"));
  expect(create.disabled).toBe(true);
  await act(async () => setValue(field("#grader-create-reason"), "第一版"));
  await act(async () => create.click());

  const sent = calls.find((c) => c.method === "POST");
  expect(sent?.url).toBe("/admin/evaluation/graders");
  expect(JSON.parse(sent!.body!)).toEqual({
    name: "引用出處",
    instructions: "要附出處",
    reason: "第一版",
  });
  await waitFor(() => container.querySelector('[role="alert"]') !== null);
  expect(field('[role="alert"]').textContent).toBe("已經有同名的評分器");
});

test.each(["workspace", "platform"] as const)(
  "a new %s grader keeps a named result and clears it for the next draft",
  async (scope) => {
    const grader = scope === "workspace" ? WORKSPACE_GRADER : PLATFORM_GRADER;
    const originalFetch = globalThis.fetch;
    vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
      if (init?.method === "POST") return json({ ...grader, name: "新評分器", version: 1 }, 201);
      return originalFetch(input, init);
    });
    await renderNode(<GraderManager scope={scope} />);
    await waitFor(() => container.textContent!.includes(grader.name));
    await act(async () => {
      field("#grader-create-heading").closest("details")!.open = true;
      setValue(field("#grader-create-name"), "新評分器");
      setValue(field("#grader-create-instructions"), "要附出處");
      setValue(field("#grader-create-reason"), "第一版");
    });
    await act(async () => button("新增評分器").click());
    await waitFor(() => field<HTMLInputElement>("#grader-create-name").value === "");
    expect(field("#grader-result").textContent).toContain("已儲存「新評分器」v1");
    expect(document.activeElement).toBe(field("#grader-result"));
    expect(field<HTMLInputElement>("#grader-create-name").value).toBe("");
    await act(async () => setValue(field("#grader-create-name"), "另一個草稿"));
    expect(container.querySelector("#grader-result")).toBeNull();
  },
);
