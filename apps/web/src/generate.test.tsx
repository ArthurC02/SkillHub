import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import { GenerationFailureFailureEnum } from "@skillhub/api-client-ts";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  window.history.pushState({}, "", "/");
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

async function render() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
  if (container.querySelector(".app-shell") && router.state.location.pathname !== "/") {
    await act(async () => {
      await router.navigate({ to: "/", search: {} });
    });
  }
}

const NO_RESULTS = {
  query: "沒有人做過的事",
  results: [],
  limit: 20,
  truncated: false,
  degraded: false,
  partial_index: false,
  filtered_out: false,
  no_results: true,
  query_suggestion: "試著說出你手上的檔案格式。",
};

function stubSession(
  features?: Record<string, boolean>,
  failures?: unknown[],
  generateResult?: unknown,
  generateRejection?: unknown,
  referenceSearch?: { query: string; result: unknown; ownSkills?: unknown },
) {
  const posted: { path: string; body: string }[] = [];
  const searchGets: string[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    if (init?.method === "POST") {
      posted.push({ path, body: String(init.body ?? "") });
      if (path === "/skills/generate" && generateResult) {
        return Promise.resolve(new Response(JSON.stringify(generateResult), { status: 201 }));
      }
      if (path === "/skills/generate" && generateRejection) {
        return Promise.resolve(new Response(JSON.stringify(generateRejection), { status: 422 }));
      }
      return Promise.resolve(
        new Response(JSON.stringify({ error: "not implemented in this stub" }), { status: 502 }),
      );
    }
    if (path === "/skills/generate/failures") {
      return Promise.resolve(
        new Response(JSON.stringify({ failures: failures ?? [] }), { status: 200 }),
      );
    }
    if (path.startsWith("/api/skills/catalog")) {
      return Promise.resolve(
        new Response(JSON.stringify({ results: [], limit: 20, total: 0, truncated: false }), {
          status: 200,
        }),
      );
    }
    if (path.startsWith("/api/skills/search")) {
      searchGets.push(path + url.search);
      const q = url.searchParams.get("q") ?? "";
      if (referenceSearch && q === referenceSearch.query) {
        return Promise.resolve(
          new Response(JSON.stringify(referenceSearch.result), { status: 200 }),
        );
      }
      return Promise.resolve(new Response(JSON.stringify(NO_RESULTS), { status: 200 }));
    }
    if (path === "/skills" && referenceSearch?.ownSkills) {
      return Promise.resolve(
        new Response(JSON.stringify(referenceSearch.ownSkills), { status: 200 }),
      );
    }
    if (path === "/me") {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            user_id: "u-1",
            email: "t@example.com",
            display_name: "tester",
            workspace_id: "ws-1",
            deletion_requested_at: null,
            purge_after: null,
            deletion_scope: null,
            ...(features ? { features } : {}),
          }),
          { status: 200 },
        ),
      );
    }
    return Promise.resolve(new Response(JSON.stringify({ skills: [] }), { status: 200 }));
  });
  return { posted, searchGets };
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

test("GEN-008: the generate entry point is absent until /me says the flag is on", async () => {
  stubSession();
  await render();
  await submitSearch("沒有人做過的事");

  expect(container.textContent).toContain("沒有夠接近的 Skill");
  expect(container.textContent).not.toContain("讓平台依你的描述做一個");
  expect(container.querySelector("#generate-task")).toBeNull();
});

test("GEN-008: with the flag on, the entry point appears in the no-results state", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  expect(container.textContent).toContain("讓平台依你的描述做一個");
  const box = container.querySelector<HTMLTextAreaElement>("#generate-task");
  expect(box).not.toBeNull();
  expect(container.textContent).toContain("試著說出你手上的檔案格式。");
  expect(box!.value).toBe("沒有人做過的事");
});

test("GEN-002/GEN-004: a generated skill's source is stated, and its two absences with it", async () => {
  const { GeneratedNotice } = await import("./components/GeneratedNotice");
  await act(async () => {
    root = createRoot(container);
    root.render(<GeneratedNotice />);
  });

  const text = container.textContent ?? "";
  expect(text).toContain("沒有經過任何人工檢視");
  expect(text).toContain("沒有任何試跑證據");
  expect(text).not.toContain("新建立");
  expect(text).not.toContain("來源未知");
});

test("GEN-003: past failures are readable, and the task description is not among them", async () => {
  stubSession({ generate_skill: true }, [
    {
      occurred_at: "2026-08-23T10:00:00Z",
      failure: "blocked",
      attempts: 2,
      codes: ["name-invalid"],
    },
    { occurred_at: "2026-08-23T09:00:00Z", failure: "quota", attempts: 0 },
  ]);
  await render();
  await submitSearch("沒有人做過的事");
  await waitFor(() => (container.textContent ?? "").includes("最近沒有成功的生成"));

  const text = container.textContent ?? "";
  expect(text).toContain("最近沒有成功的生成（2 次）");
  expect(text).toContain("name-invalid");
  expect(text).toContain("沒有呼叫模型，也沒有花錢");
  expect(text).toContain("這裡沒有記下你當時輸入的任務描述");
});

test("GEN-003: a workspace with no failures is shown no history section", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  expect(container.textContent).toContain("讓平台依你的描述做一個");
  expect(container.textContent).not.toContain("最近沒有成功的生成");
});

test("GEN-008: the bounds the server enforces are stated before the button, and the cost is a sourced estimate", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const text = container.textContent ?? "";
  expect(text).toContain("這一次最多會用到");
  expect(text).toContain("4,000 字");
  expect(text).toContain("推理加輸出合計 16,000 token");
  expect(text).toContain("最多嘗試 2 次");
  expect(text).not.toContain("尚未定值");
  expect(text).toContain("約 US$0.003–$0.03");
  expect(text).toContain("多數落在 US$0.006 上下");
  expect(text).toContain("估計值，非報價");
  expect(text).toContain("2026-08-25 對真實閘道生成 10 次的實付分布");
  expect(text).toContain("平台沒有為單次生成設定費用上限");
  const dd = (needle: string) =>
    Array.from(container.querySelectorAll("dd")).find((n) => n.textContent?.includes(needle));
  expect(dd("估計值，非報價")?.closest("details"), "預估成本被折進去了").toBeNull();
  expect(dd("16,000 token")?.closest("details"), "上限那一列還攤在表單上").not.toBeNull();
  expect(container.querySelector<HTMLTextAreaElement>("#generate-task")!.maxLength).toBe(-1);
});

test("GEN-001: the description counter counts what the server counts (runes, not UTF-16 units)", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  await act(async () => {
    const textarea = container.querySelector("#generate-task")!;
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
    setter.call(textarea, "一二三🙂");
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });

  const count = container.querySelector("#generate-task-count")!;
  expect(count, "沒有計數器").not.toBeNull();
  expect(count.textContent).toContain("4 / 4,000 字");
  expect(
    container
      .querySelector<HTMLTextAreaElement>("#generate-task")!
      .getAttribute("aria-describedby"),
  ).toBe("generate-task-count");
});

test("GEN-005: the diagram's accepted types and size ceiling are stated before the picker, not after a refusal", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const note = container.querySelector("#generate-diagram-note")!;
  expect(note, "檔案選擇器旁邊沒有說出它會擋什麼").not.toBeNull();
  expect(note.textContent).toContain("PNG、JPEG 或 WebP");
  expect(note.textContent).toContain("4 MB 以內");
  expect(container.querySelector("#generate-diagram-file")!.getAttribute("aria-describedby")).toBe(
    "generate-diagram-note",
  );
});

test("GEN-003: every failure value in the contract has a sentence", async () => {
  const { FAILURE_SENTENCE } = await import("./components/generateFailureSentence");
  for (const value of Object.values(GenerationFailureFailureEnum)) {
    const sentence = FAILURE_SENTENCE[value as keyof typeof FAILURE_SENTENCE];
    expect(sentence, `no sentence for failure ${JSON.stringify(value)}`).toBeTypeOf("function");
  }
});

test("GEN-008: a successful generation does not re-run the search behind it", async () => {
  const { searchGets } = stubSession({ generate_skill: true }, [], {
    skill_id: "sk-1",
    version_number: 1,
    attempts: 1,
    generator_model: "stub",
    generator_prompt_version: "stub/v1",
  });
  await render();
  await submitSearch("沒有人做過的事");
  const before = searchGets.length;

  await act(async () => {
    container.querySelectorAll("button").forEach((b) => {
      if (b.textContent === "生成一個 Skill") b.click();
    });
  });
  await waitFor(() => (container.textContent ?? "").includes("已經產生一個 Skill"));

  expect(searchGets.length).toBe(before);
});

test("設計 §3 第 9 條：生成失敗底下的發現分組是它的內容，不是它的兄弟", async () => {
  stubSession({ generate_skill: true }, [], undefined, {
    attempts: 1,
    errors: [{ code: "skill-md-missing", message: "SKILL.md not found at package root" }],
    warnings: [],
    infos: [],
  });
  await render();
  await submitSearch("沒有人做過的事");
  await act(async () => {
    container.querySelectorAll("textarea").forEach((t) => {
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
      setter.call(t, "把 PDF 轉成摘要");
      t.dispatchEvent(new Event("input", { bubbles: true }));
    });
  });
  await act(async () => {
    container.querySelectorAll("button").forEach((b) => {
      if (b.textContent === "生成一個 Skill") b.click();
    });
  });
  await waitFor(() => (container.textContent ?? "").includes("阻擋錯誤"));

  const outline = Array.from(container.querySelectorAll("h2,h3,h4")).map(
    (h) => `${h.tagName.toLowerCase()} ${h.textContent?.trim().slice(0, 12)}`,
  );
  const failure = outline.findIndex((h) => h.includes("生成失敗"));
  expect(failure, `生成失敗 not in the outline: ${outline.join(" | ")}`).toBeGreaterThan(-1);
  expect(outline[failure]).toMatch(/^h3 /);
  expect(outline[failure + 1], outline.join(" | ")).toMatch(/^h4 阻擋錯誤/);
});

test("GEN-003: the collision sentence does not claim the neighbour is not generated", async () => {
  const { failureSentence } = await import("./components/generateFailureSentence");
  const sentence = failureSentence({
    occurred_at: "2026-08-24T01:00:00Z",
    failure: "rejected",
    attempts: 1,
    collision: true,
  });
  expect(sentence).toContain("同名");
  expect(sentence).not.toContain("不是生成的");
});

test("GEN-005: a diagram file with no text enables submit and posts the diagram, not task_description", async () => {
  const { posted } = stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  await act(async () => {
    const textarea = container.querySelector("#generate-task")!;
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
    setter.call(textarea, "");
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });

  const fileInput = container.querySelector<HTMLInputElement>("#generate-diagram-file")!;
  const file = new File([new Uint8Array([137, 80, 78, 71])], "flow.png", { type: "image/png" });
  await act(async () => {
    Object.defineProperty(fileInput, "files", { value: [file], configurable: true });
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("flow.png"));

  const submitBtn = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "生成一個 Skill",
  )!;
  expect(submitBtn.disabled).toBe(false);

  await act(async () => {
    submitBtn.click();
  });
  await waitFor(() => posted.some((p) => p.path === "/skills/generate"));

  const body = JSON.parse(posted.find((p) => p.path === "/skills/generate")!.body);
  expect(body.diagram.media_type).toBe("image/png");
  expect(typeof body.diagram.data).toBe("string");
  expect(body.diagram.data.length).toBeGreaterThan(0);
  expect(body.task_description).toBeUndefined();
});

test("GEN-005: an oversized file is refused client-side with an alert, and nothing is posted", async () => {
  const { posted } = stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const fileInput = container.querySelector<HTMLInputElement>("#generate-diagram-file")!;
  const big = new File([new Uint8Array(4_000_001)], "big.png", { type: "image/png" });
  await act(async () => {
    Object.defineProperty(fileInput, "files", { value: [big], configurable: true });
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });

  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("上限");
  expect(posted.some((p) => p.path === "/skills/generate")).toBe(false);
});

test("GEN-005: a diagram file at exactly the size ceiling is accepted", async () => {
  const { posted } = stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const fileInput = container.querySelector<HTMLInputElement>("#generate-diagram-file")!;
  const atCeiling = new File([new Uint8Array(4_000_000)], "ok.png", { type: "image/png" });
  await act(async () => {
    Object.defineProperty(fileInput, "files", { value: [atCeiling], configurable: true });
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("ok.png"));

  expect(container.querySelector('[role="alert"]')).toBeNull();
  const submitBtn = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "生成一個 Skill",
  )!;
  expect(submitBtn.disabled).toBe(false);

  await act(async () => {
    submitBtn.click();
  });
  await waitFor(() => posted.some((p) => p.path === "/skills/generate"));
});

test("GEN-005: a diagram file with an unsupported MIME type is refused with its own message", async () => {
  const { posted } = stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const fileInput = container.querySelector<HTMLInputElement>("#generate-diagram-file")!;
  const wrongType = new File([new Uint8Array([1, 2, 3, 4])], "flow.gif", { type: "image/gif" });
  await act(async () => {
    Object.defineProperty(fileInput, "files", { value: [wrongType], configurable: true });
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });

  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toBe("圖片格式需為 PNG、JPEG 或 WebP。");
  expect(posted.some((p) => p.path === "/skills/generate")).toBe(false);
});

test("GEN-001: typing past the rune ceiling warns that the server will enforce it, without disabling submit", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  await act(async () => {
    const textarea = container.querySelector("#generate-task")!;
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
    setter.call(textarea, "字".repeat(4001));
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });

  const count = container.querySelector("#generate-task-count")!;
  expect(count.textContent).toContain("4,001 / 4,000 字");
  expect(count.textContent).toContain("——超過了，送出會被伺服器擋下");
  const submitBtn = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "生成一個 Skill",
  )!;
  expect(submitBtn.disabled).toBe(false);
});

const REFERENCE_HIT = (n: number) => ({
  skill_id: `ref-${n}`,
  name: `參考 Skill ${n}`,
  summary: `摘要 ${n}`,
  summary_source: "package",
});

test("GEN-006: searching lists a result, and ticking it sends reference_skill_ids with the task text", async () => {
  const { posted } = stubSession({ generate_skill: true }, [], undefined, undefined, {
    query: "分析報表",
    result: {
      query: "分析報表",
      results: [REFERENCE_HIT(1)],
      limit: 20,
      truncated: false,
      degraded: false,
      partial_index: false,
      filtered_out: false,
      no_results: false,
    },
  });
  await render();
  await submitSearch("沒有人做過的事");

  await act(async () => {
    const textarea = container.querySelector("#generate-task")!;
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
    setter.call(textarea, "把 PDF 轉成摘要");
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });

  const refInput = container.querySelector<HTMLInputElement>("#generate-reference-query")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setter.call(refInput, "分析報表");
    refInput.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("參考 Skill 1"));

  const checkbox = Array.from(
    container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'),
  ).find((c) => c.closest("li")?.textContent?.includes("參考 Skill 1"))!;
  await act(async () => {
    checkbox.click();
  });

  const submitBtn = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "生成一個 Skill",
  )!;
  await act(async () => {
    submitBtn.click();
  });
  await waitFor(() => posted.some((p) => p.path === "/skills/generate"));

  const body = JSON.parse(posted.find((p) => p.path === "/skills/generate")!.body);
  expect(body.reference_skill_ids).toEqual(["ref-1"]);
  expect(body.task_description).toBe("把 PDF 轉成摘要");
});

test("GEN-006: a fourth reference selection is not possible", async () => {
  stubSession({ generate_skill: true }, [], undefined, undefined, {
    query: "分析報表",
    result: {
      query: "分析報表",
      results: [REFERENCE_HIT(1), REFERENCE_HIT(2), REFERENCE_HIT(3), REFERENCE_HIT(4)],
      limit: 20,
      truncated: false,
      degraded: false,
      partial_index: false,
      filtered_out: false,
      no_results: false,
    },
  });
  await render();
  await submitSearch("沒有人做過的事");

  const refInput = container.querySelector<HTMLInputElement>("#generate-reference-query")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setter.call(refInput, "分析報表");
    refInput.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("參考 Skill 4"));

  function checkboxFor(name: string) {
    return Array.from(container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')).find(
      (c) => c.closest("li")?.textContent?.includes(name),
    )!;
  }
  for (const n of [1, 2, 3]) {
    await act(async () => {
      checkboxFor(`參考 Skill ${n}`).click();
    });
  }

  const fourth = checkboxFor("參考 Skill 4");
  expect(fourth.disabled).toBe(true);
  await act(async () => {
    fourth.click();
  });
  expect(fourth.checked).toBe(false);
  expect(container.textContent).toContain("已經選滿 3 個");
});

test("GEN-005: a FileReader error is shown as an alert and nothing is posted while reading", async () => {
  const { posted } = stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  let capturedReader: FakeFileReader | undefined;
  class FakeFileReader {
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    result: string | ArrayBuffer | null = null;
    readAsDataURL() {
      capturedReader = this;
    }
  }
  vi.stubGlobal("FileReader", FakeFileReader as unknown as typeof FileReader);

  const fileInput = container.querySelector<HTMLInputElement>("#generate-diagram-file")!;
  const file = new File([new Uint8Array([137, 80, 78, 71])], "flow.png", { type: "image/png" });
  await act(async () => {
    Object.defineProperty(fileInput, "files", { value: [file], configurable: true });
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });

  const submitBtn = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "生成一個 Skill",
  )!;
  expect(submitBtn.disabled).toBe(true);

  await act(async () => {
    capturedReader!.onerror?.();
  });

  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("讀取圖片失敗，請重新選擇。");
  expect(posted.some((p) => p.path === "/skills/generate")).toBe(false);
});

test("GEN-005: removing a diagram then re-selecting the same file shows it again", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const fileInput = container.querySelector<HTMLInputElement>("#generate-diagram-file")!;
  const file = new File([new Uint8Array([137, 80, 78, 71])], "flow.png", { type: "image/png" });
  await act(async () => {
    Object.defineProperty(fileInput, "files", { value: [file], configurable: true });
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("已選擇 flow.png"));

  await act(async () => {
    Array.from(container.querySelectorAll("button"))
      .find((b) => b.textContent === "移除")!
      .click();
  });
  expect(container.textContent).not.toContain("已選擇 flow.png");
  expect(fileInput.value).toBe("");

  await act(async () => {
    Object.defineProperty(fileInput, "files", { value: [file], configurable: true });
    fileInput.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("已選擇 flow.png"));
  expect(container.textContent).toContain("已選擇 flow.png");
});

test("GEN-006: a reference-unusable 422 renders verbatim and keeps the selected chips", async () => {
  stubSession(
    { generate_skill: true },
    [],
    undefined,
    { error: "其中一個參考的 Skill 無法使用，請換一個再試一次。" },
    {
      query: "分析報表",
      result: {
        query: "分析報表",
        results: [REFERENCE_HIT(1)],
        limit: 20,
        truncated: false,
        degraded: false,
        partial_index: false,
        filtered_out: false,
        no_results: false,
      },
    },
  );

  await render();
  await submitSearch("沒有人做過的事");

  const refInput = container.querySelector<HTMLInputElement>("#generate-reference-query")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setter.call(refInput, "分析報表");
    refInput.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("參考 Skill 1"));

  await act(async () => {
    Array.from(container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'))
      .find((c) => c.closest("li")?.textContent?.includes("參考 Skill 1"))!
      .click();
  });

  const submitBtn = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "生成一個 Skill",
  )!;
  await act(async () => {
    submitBtn.click();
  });
  await waitFor(() => container.querySelector('[role="alert"]') !== null);

  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("其中一個參考的 Skill 無法使用，請換一個再試一次。");
  expect(container.textContent).toContain("參考 Skill 1 ✕");
});

test("GEN-006: the reference picker's search carries purpose=reference, Home's does not", async () => {
  const { searchGets } = stubSession({ generate_skill: true }, [], undefined, undefined, {
    query: "分析報表",
    result: {
      query: "分析報表",
      results: [REFERENCE_HIT(1)],
      limit: 20,
      truncated: false,
      degraded: false,
      partial_index: false,
      filtered_out: false,
      no_results: false,
    },
  });
  await render();
  await submitSearch("沒有人做過的事");

  const homeSearchUrl = searchGets.find((u) => !u.includes("purpose="));
  expect(homeSearchUrl).toBeDefined();

  const refInput = container.querySelector<HTMLInputElement>("#generate-reference-query")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setter.call(refInput, "分析報表");
    refInput.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("參考 Skill 1"));

  const refSearchUrl = searchGets.find((u) => u.includes("purpose=reference"));
  expect(refSearchUrl).toBeDefined();
});

test("GEN-006: a reference tick with no text and no diagram keeps submit disabled", async () => {
  stubSession({ generate_skill: true }, [], undefined, undefined, {
    query: "分析報表",
    result: {
      query: "分析報表",
      results: [REFERENCE_HIT(1)],
      limit: 20,
      truncated: false,
      degraded: false,
      partial_index: false,
      filtered_out: false,
      no_results: false,
    },
  });
  await render();
  await submitSearch("沒有人做過的事");

  await act(async () => {
    const textarea = container.querySelector("#generate-task")!;
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
    setter.call(textarea, "");
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });

  const refInput = container.querySelector<HTMLInputElement>("#generate-reference-query")!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setter.call(refInput, "分析報表");
    refInput.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await waitFor(() => (container.textContent ?? "").includes("參考 Skill 1"));

  await act(async () => {
    Array.from(container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'))
      .find((c) => c.closest("li")?.textContent?.includes("參考 Skill 1"))!
      .click();
  });

  const submitBtn = Array.from(container.querySelectorAll("button")).find(
    (b) => b.textContent === "生成一個 Skill",
  )!;
  expect(submitBtn.disabled).toBe(true);

  const why = container.querySelector("#generate-why-disabled")!;
  expect(why, "停用的生成按鈕旁邊沒有任何一句話說為什麼（§2.4）").not.toBeNull();
  expect(why.textContent).toContain("任務描述與流程圖至少要有一個");
  expect(submitBtn.getAttribute("aria-describedby")).toBe("generate-why-disabled");
  expect(why.closest("details"), "§2.10 第 5 項：停用理由不得折疊").toBeNull();
});

test("GEN-008: the cost basis names the diagram and reference measurements", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  const text = container.textContent ?? "";
  expect(text).toContain("帶流程圖與帶參考各實測一次");
  expect(text).toContain("US$0.0039");
  expect(text).toContain("US$0.0040");
  expect(text).toContain("一次不是分布");
});

test("IA-5: a visitor is told what login buys, not sent to a page they cannot open", async () => {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me") {
      return Promise.resolve(
        new Response(JSON.stringify({ error: "no session" }), { status: 401 }),
      );
    }
    if (path.startsWith("/api/skills/search")) {
      return Promise.resolve(new Response(JSON.stringify(NO_RESULTS), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify({ skills: [] }), { status: 200 }));
  });
  await render();
  await submitSearch("沒有人做過的事");

  const text = container.textContent ?? "";
  expect(text).toContain("登入後可以把它匯入");
  expect(text).not.toContain("直接匯入它");
});
