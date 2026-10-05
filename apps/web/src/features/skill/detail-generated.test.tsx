import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { SkillDetail } from "./detail/SkillDetail.page";
import { skillDetail, VERSION } from "../../testing/fixtures/platform";
import type { SkillDetail as SkillDetailModel, SkillSource } from "../../core/api/types";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";

const SKILL = "11111111-1111-1111-1111-111111111111";

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
  Link: ({
    to,
    params,
    search,
    className,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    className?: string;
    children?: unknown;
  }) => {
    const path = Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to);
    const query = new URLSearchParams(
      Object.entries(search ?? {}).filter(
        (entry): entry is [string, string] => entry[1] !== undefined,
      ),
    ).toString();
    return (
      <a className={className} href={query ? `${path}?${query}` : path}>
        {children as never}
      </a>
    );
  },
  useParams: () => ({ skillId: SKILL }),
  useSearch: () => ({}),
  useNavigate: () => () => Promise.resolve(),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

function stubVisitor(detail: SkillDetailModel) {
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path === "/me") return json({ error: "not authenticated" }, 401);
    if (path.endsWith("/versions")) return json({ error: "not authenticated" }, 401);
    if (path.startsWith("/api/skills/")) return json(detail);
    return json({ error: "not found" }, 401);
  });
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
  await waitFor(settled);
}

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const text = () => container.textContent ?? "";
const settledAsVisitor = () => text().includes("登入後即可把這個小工具複製");

function generatedDetail(overrides: Partial<SkillSource>): SkillDetailModel {
  const base = skillDetail(SKILL, "生成的小工具");
  return {
    ...base,
    redistribution: { value: "generated", label: "平台生成", note: "" },
    source: {
      type: "generated",
      task_description: "",
      generator_model: "stub-model",
      generator_prompt_version: "gen/v1",
      trust: { value: "generated", label: "平台生成", note: "沒有上游可追溯。" },
      ...overrides,
    },
  };
}

test("GEN-002: task_description 非空時顯示逐字的任務描述句", async () => {
  stubVisitor(generatedDetail({ task_description: "把 PDF 轉成摘要" }));
  await render(<SkillDetail />, settledAsVisitor);

  expect(text()).toContain("來源：由平台依你的任務描述生成");
  const disclosure = Array.from(container.querySelectorAll("details")).find((d) =>
    d.textContent?.includes("你當時輸入的任務描述"),
  );
  expect(disclosure?.querySelector("p")?.textContent).toBe("把 PDF 轉成摘要");
});

test("a generated 小工具 keeps its current Version when continuing to 測試題", async () => {
  stubVisitor(generatedDetail({ task_description: "把 PDF 轉成摘要" }));
  await render(<SkillDetail />, settledAsVisitor);

  const next = Array.from(container.querySelectorAll("a")).find((link) =>
    link.textContent?.includes("先建立測試題再試跑"),
  );
  const url = new URL(next!.href);
  expect(url.pathname).toBe("/lab/test-cases");
  expect(url.searchParams.get("skill")).toBe(SKILL);
  expect(url.searchParams.get("version")).toBe(VERSION);
});

test("GEN-005: task_description 為空、只有流程圖時顯示流程圖句，且不是任務描述句", async () => {
  stubVisitor(
    generatedDetail({
      task_description: "",
      generation_inputs: {
        diagram: { media_type: "image/png", sha256: "abc123", bytes: 4096 },
      },
    }),
  );
  await render(<SkillDetail />, settledAsVisitor);

  const body = text();
  expect(body).toContain("來源：由平台依你上傳的流程圖生成");
  expect(body).not.toContain("來源：由平台依你的任務描述生成");
  expect(body).toContain("abc123");
});

test("GEN-005: task_description 為空、也沒有流程圖時顯示不指名來源的句子", async () => {
  stubVisitor(generatedDetail({ task_description: "" }));
  await render(<SkillDetail />, settledAsVisitor);

  const sourceLine = Array.from(container.querySelectorAll("p")).find((p) =>
    (p.textContent ?? "").startsWith("來源："),
  );
  expect(sourceLine?.textContent).toBe("來源：由平台生成");
});

test("GEN-006: 參考的 Skill 名稱各是一個連到 /skills/<id> 的連結", async () => {
  stubVisitor(
    generatedDetail({
      task_description: "把 PDF 轉成摘要",
      generation_inputs: {
        references: [
          { skill_id: "ref-1", version_id: "v-ref-1", name: "參考小工具甲" },
          { skill_id: "ref-2", version_id: "v-ref-2", name: "參考小工具乙" },
        ],
      },
    }),
  );
  await render(<SkillDetail />, settledAsVisitor);

  const body = text();
  expect(body).toContain("參考小工具甲");
  expect(body).toContain("參考小工具乙");
  expect(container.querySelector('a[href="/skills/ref-1"]')?.textContent).toBe("參考小工具甲");
  expect(container.querySelector('a[href="/skills/ref-2"]')?.textContent).toBe("參考小工具乙");
});
