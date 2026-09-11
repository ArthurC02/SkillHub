import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { SkillDetail } from "./pages/SkillDetail";
import { skillDetail } from "./fixtures/platform";
import type { SkillDetail as SkillDetailModel, SkillSource } from "./api/types";

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
    className,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    className?: string;
    children?: unknown;
  }) => (
    <a
      className={className}
      href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}
    >
      {children as never}
    </a>
  ),
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
const settledAsVisitor = () => text().includes("登入後即可 Fork");

function generatedDetail(overrides: Partial<SkillSource>): SkillDetailModel {
  const base = skillDetail(SKILL, "生成的 Skill");
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
          { skill_id: "ref-1", version_id: "v-ref-1", name: "參考 Skill 甲" },
          { skill_id: "ref-2", version_id: "v-ref-2", name: "參考 Skill 乙" },
        ],
      },
    }),
  );
  await render(<SkillDetail />, settledAsVisitor);

  const body = text();
  expect(body).toContain("參考 Skill 甲");
  expect(body).toContain("參考 Skill 乙");
  expect(container.querySelector('a[href="/skills/ref-1"]')?.textContent).toBe("參考 Skill 甲");
  expect(container.querySelector('a[href="/skills/ref-2"]')?.textContent).toBe("參考 Skill 乙");
});
