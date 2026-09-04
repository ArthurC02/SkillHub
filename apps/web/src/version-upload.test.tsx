import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { VersionUpload } from "./components/VersionUpload";
import { SKILL_VERSIONS } from "./fixtures/platform";

// 04 丙-151: VersionUpload gained the same 422-CategorizedFindings handling,
// per-status Chinese sentences, and the rule sentence ImportSkill shows before
// its file input. Same hand-rolled DOM plumbing as detail.test.tsx /
// workspace.test.tsx; @testing-library is not a dependency of this app.

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

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

async function render() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <VersionUpload skillId={SKILL} />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() => text().includes("上傳新版本"));
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

function stubOwner(uploadResponse: () => Promise<Response>) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path.endsWith("/versions") && init?.method === "POST") return uploadResponse();
    if (path.endsWith("/versions")) return json(SKILL_VERSIONS);
    return json({ error: "not found" }, 404);
  });
}

async function submitUpload() {
  const input = container.querySelector<HTMLInputElement>("#skill-version-file")!;
  const file = new File(["zip bytes"], "skill.zip", { type: "application/zip" });
  Object.defineProperty(input, "files", { value: [file] });
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
  const form = container.querySelector("form.version-upload")!;
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

// --- 04 丙-151①: 422 CategorizedFindings 逐條渲染，不落回 statusText ------------

test("丙-151: 422 兩條 finding 的中文訊息都渲染，且不出現 Unprocessable Entity", async () => {
  stubOwner(() =>
    json(
      {
        errors: [
          // 04 丙-152 — apps/platform/internal/shared/skillpkg/skillpkg.go 真的回的字串。
          { severity: "error", code: "skill-md-missing", message: "套件根目錄找不到 SKILL.md" },
          { severity: "error", code: "name-missing", message: "frontmatter 欄位 name 為必填" },
        ],
        warnings: [],
        infos: [],
      },
      422,
    ),
  );
  await render();
  await submitUpload();
  await waitFor(() => text().includes("上傳被擋下"));

  expect(text()).toContain("套件根目錄找不到 SKILL.md");
  expect(text()).toContain("frontmatter 欄位 name 為必填");
  expect(text()).not.toContain("Unprocessable Entity");
});

// --- 04 丙-151③: 檔案框前的規則句 ------------------------------------------------

test("丙-151: 檔案輸入框之前有匯入頁同樣的規則句", async () => {
  stubOwner(() => json({ error: "unused" }, 500));
  await render();

  const rule = container.querySelector("ul.note");
  expect(rule, "找不到規則句的 <ul class=note>").toBeDefined();
  expect(rule!.textContent).toContain("SKILL.md");
  expect(rule!.textContent).toContain("大小上限見拒絕訊息");

  // 規則句要在檔案輸入框之前（§2.2 第三向：擋住人的限制在撞上之前看得見）。
  const input = container.querySelector("#skill-version-file")!;
  expect(rule!.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

// --- 04 丙-151②: 其餘狀態換這一頁自己的中文句，不印 error.message --------------

test("丙-151: 413 印「檔案超過上限，請縮小套件再上傳。」", async () => {
  stubOwner(() => json({ error: "file is larger than 25 MB" }, 413));
  await render();
  await submitUpload();
  await waitFor(() => text().includes("上傳沒有成功") || text().includes("檔案超過上限"));

  expect(text()).toContain("檔案超過上限，請縮小套件再上傳。");
  expect(text()).not.toContain("file is larger than");
});
