import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import { GenerationFailureFailureEnum } from "@skillhub/api-client-ts";

// GEN-004 and GEN-008. Same hand-rolled harness the other suites use.
//
// The load-bearing case is the first one: an entry point that appears when it
// should not is the failure ADR-052 exists to prevent, and it has no symptom —
// the page looks fine, and what breaks is the meaning of a number nobody
// re-measures. Twelve people, one chance.

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

/** A logged-in session, with `features` present only when asked for. */
function stubSession(
  features?: Record<string, boolean>,
  failures?: unknown[],
  generateResult?: unknown,
  /** A categorised 422 — the package's own findings, verbatim (02:GEN-003). */
  generateRejection?: unknown,
  /**
   * 02:GEN-006's reference picker searches the same `/api/skills/search`
   * endpoint the home page's own search box uses, with a different `q`. This
   * stubs its answer, keyed on the query text, without disturbing the home
   * page's own `NO_RESULTS` answer for `submitSearch("沒有人做過的事")` above.
   */
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
    // 02:DISC-006: the home page reads the catalogue before anyone has searched,
    // and the catch-all below answers `{skills: []}` — a 200 with no `results`,
    // which is not a shape any endpoint returns and which made the page throw
    // before these tests could type into it.
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

// ADR-052's whole point. `/me` without a `features` object is what every
// deployment returns today, and the entry point must not be drawable from it.
test("GEN-008: the generate entry point is absent until /me says the flag is on", async () => {
  stubSession();
  await render();
  await submitSearch("沒有人做過的事");

  // The no-results state itself is still there — otherwise this passes because
  // the page failed, not because the flag held.
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
  // DISC-005's suggestion is still the server's, and the generate box is
  // seeded with what was searched for rather than starting empty.
  expect(container.textContent).toContain("試著說出你手上的檔案格式。");
  expect(box!.value).toBe("沒有人做過的事");
});

// 02:GEN-002 forbids showing a generated package as an unknown source, and
// GEN-004 requires two named absences rather than one neutral word. Both are
// sentences that stay wrong silently.
test("GEN-002/GEN-004: a generated skill's source is stated, and its two absences with it", async () => {
  const { GeneratedNotice } = await import("./components/GeneratedNotice");
  await act(async () => {
    root = createRoot(container);
    root.render(<GeneratedNotice />);
  });

  const text = container.textContent ?? "";
  expect(text).toContain("沒有經過任何人工檢視");
  expect(text).toContain("沒有任何試跑證據");
  // ADR-041 決策 2 / 02:GEN-004: a neutral word describing a package the
  // platform wrote thirty seconds ago as merely recent is the failure named.
  expect(text).not.toContain("新建立");
  expect(text).not.toContain("來源未知");
});

// GEN-003's last clause. The write half shipped first and was briefly counted as
// satisfying it; a row only a database connection can see is not a record left
// in the workspace, and the gap shows no symptom on any screen.
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
  // 02:GEN-001: a refusal before the model call costs nothing, and the row has
  // to say so — otherwise "額度不足" reads like something the user was billed for.
  expect(text).toContain("沒有呼叫模型，也沒有花錢");
  // NFR-002: the description belongs to the source row, not to 400-day history.
  expect(text).toContain("這裡沒有記下你當時輸入的任務描述");
});

// An empty history is a section with no rows, not a value rendered blank
// (design system §2.9 is about the latter). A user who has never failed must
// not be shown a heading for something that did not happen.
test("GEN-003: a workspace with no failures is shown no history section", async () => {
  stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  expect(container.textContent).toContain("讓平台依你的描述做一個");
  expect(container.textContent).not.toContain("最近沒有成功的生成");
});

// GEN-008 / 02:GEN-001 「生成前顯示…」. Two things are asserted: that the three
// enforced ceilings are on screen, and that the money position now carries a
// sourced range rather than 尚未定值.
//
// The cost half said 尚未定值 until 2026-08-25 because the only number available
// was B round's mean, and printing a mean as an estimate is the 04 乙-2 shape.
// Ten real-gateway generations gave a distribution (m5 report §8.2), which is the
// form 02:PDM-005 §5.3 accepts. What is asserted here is not the number: it is
// that the range is labelled an estimate, names where it came from, and still
// says out loud that no ceiling is enforced. A range that quietly reads as a
// promise would pass a test that only checked for digits.
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
  // The three things that keep a range from reading as a quote. Losing any one
  // of them turns a sourced estimate back into a number with nothing behind it.
  expect(text).toContain("估計值，非報價");
  expect(text).toContain("2026-08-25 對真實閘道生成 10 次的實付分布");
  expect(text).toContain("平台沒有為單次生成設定費用上限");
  // And the textarea carries no maxLength: the browser's unit (UTF-16 code
  // units) is not the server's (runes), so one enforcer, and it is the server.
  expect(container.querySelector<HTMLTextAreaElement>("#generate-task")!.maxLength).toBe(-1);
});

// The sentence table is keyed on the hand-written union; this asserts it
// against the generated enum, so a value added to the contract and not to
// types.ts fails here rather than rendering the "unreadable" fallback for a
// value the server meant (the PACKAGING_BLOCKED_LABEL pattern).
test("GEN-003: every failure value in the contract has a sentence", async () => {
  const { FAILURE_SENTENCE } = await import("./components/generateFailureSentence");
  for (const value of Object.values(GenerationFailureFailureEnum)) {
    const sentence = FAILURE_SENTENCE[value as keyof typeof FAILURE_SENTENCE];
    expect(sentence, `no sentence for failure ${JSON.stringify(value)}`).toBeTypeOf("function");
  }
});

// fcc9238's fix without this had no teeth: success used to invalidate
// ["skills"], which matched the active ["skills","search",…] query — re-running
// the search and writing a second search_performed analytics event per success.
// That is 01 §11.2's first funnel segment, the number with one chance and
// twelve people. The assertion is on the SEARCH REQUEST COUNT, so flipping the
// key back turns this red.
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

/**
 * 04 丙-139 — 「阻擋錯誤（N）」 is the CONTENT of 生成失敗, not its sibling.
 *
 * The shared `Findings` hardcoded `h3`, which is right under `ImportSkill`'s
 * `h2 匯入失敗` and wrong under this panel: the outline read
 * `h2 沒有夠接近的？` → `h3 生成失敗` → `h3 阻擋錯誤（2）`, so a reader navigating
 * by headings met the group as a peer of the failure rather than as what the
 * failure consists of. **axe cannot see this** — it fails a skipped level,
 * never a level that should have gone down and did not — and this panel is
 * behind the exposure flag, so no `__outlines__` snapshot covers it either.
 * `Packaging`'s own findings list has run h3 → h4 all along.
 */
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
  // The group that follows it is one level deeper, never a second h3.
  expect(outline[failure + 1], outline.join(" | ")).toMatch(/^h4 阻擋錯誤/);
});

// The collision sentence must be true for BOTH kinds of neighbour: since the
// guard widened, the most common collision is a regeneration landing on the
// earlier GENERATED skill, and the old sentence asserted the opposite
// (「而它不是生成的」).
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

// 02:GEN-005. A diagram alone — no task description — must be a submittable
// generation, and the request must carry the diagram and omit
// task_description entirely (not send it as "").
test("GEN-005: a diagram file with no text enables submit and posts the diagram, not task_description", async () => {
  const { posted } = stubSession({ generate_skill: true });
  await render();
  await submitSearch("沒有人做過的事");

  // The textarea is seeded from the search query (GEN-008); this test is
  // about the diagram-only path, so it starts from a genuinely empty box.
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

// 02:GEN-005's client-side echo of generateMaxDiagramBytes: a file over the
// ceiling must never leave the browser.
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

const REFERENCE_HIT = (n: number) => ({
  skill_id: `ref-${n}`,
  name: `參考 Skill ${n}`,
  summary: `摘要 ${n}`,
  summary_source: "package",
});

// 02:GEN-006. Typing a query lists a result from the public search hook;
// ticking it and submitting alongside the task text sends both.
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

// 02:GEN-006's ceiling: GENERATE_MAX_REFERENCES. A fourth tick must not be
// possible — the fourth checkbox is disabled rather than silently accepted.
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

// A FileReader failure (corrupt file, browser refusal) must not leave the
// button clickable while nothing was decoded, and must say so rather than
// silently doing nothing.
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
  // Still reading: a click here must not race the decode.
  expect(submitBtn.disabled).toBe(true);

  await act(async () => {
    capturedReader!.onerror?.();
  });

  const alert = container.querySelector('[role="alert"]');
  expect(alert?.textContent).toContain("讀取圖片失敗，請重新選擇。");
  expect(posted.some((p) => p.path === "/skills/generate")).toBe(false);
});

// removeDiagram must reset the file input's own value, or re-selecting the
// same File fires no `change` event and 已選擇 never reappears.
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

// 02:GEN-006's fourth silent refusal (an unusable reference) surfaces as an
// uncategorised 422 — the same path as a blank-input or quota refusal — and
// must render verbatim without naming which reference failed (iron rule 3),
// while the selection the user made stays visible so they can swap one out.
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

// 02:GEN-006 search must never share the funnel with a page view: the picker
// carries `purpose=reference` and Home's own search box does not.
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

// Ticking a reference alone (empty description, no diagram) must not enable
// submit: GEN-006 reads references as worked examples, it is not itself a
// third form of task input.
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

  // Start from a genuinely empty task box — submitSearch seeds it (GEN-008).
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
});

// The cost block's basis line must carry the two real measurements alongside
// the ten-generation distribution, or the range quietly drifts back to
// reading like a promise about diagram/reference generations specifically.
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

// IA-5's exit has to be true for the visitor too: DISC-001 serves this page
// without a session, and /workspace/import needs one. Review found the first
// version handed anonymous callers a link to a page they cannot open.
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
  // The logged-in wording (which carries the link) must not be what a visitor
  // gets. The nav bar's own /workspace/import link is still there and still
  // wrong for a visitor — that is IA-6, which is open and not this fix.
  expect(text).not.toContain("直接匯入它");
});
