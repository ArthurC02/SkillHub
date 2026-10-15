import { StrictMode, act, type Ref } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { VersionUpload } from "./version/components/VersionUpload";
import { SKILL_VERSIONS, platformResponse } from "../../testing/fixtures/platform";
import { queryKeys } from "../../core/api/queryKeys";
import { DEFAULT_WAIT_MS, pollUntil } from "../../testing/poll";

const SKILL = "11111111-1111-1111-1111-111111111111";
const VERSION = "22222222-2222-2222-2222-222222222222";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    children,
    ref,
  }: {
    to: string;
    params?: Record<string, string>;
    children: unknown;
    ref?: Ref<HTMLAnchorElement>;
  }) => {
    const href = Object.entries(params ?? {}).reduce(
      (path, [key, value]) => path.replace(`$${key}`, value),
      to,
    );
    return (
      <a href={href} ref={ref}>
        {children as never}
      </a>
    );
  },
}));

let container: HTMLDivElement;
let root: Root;
const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
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
          <VersionUpload skillId={SKILL} versionId={VERSION} contextConfirmed />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(() => text().includes("上傳新版本"));
}

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => container.textContent, timeoutMs);
}

const text = () => container.textContent ?? "";

function stubOwner(uploadResponse: () => Promise<Response>) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input).replace(/^https?:\/\/[^/]+/, "");
    const path = url.split("?")[0];
    if (path === "/me") return json(platformResponse("/me").body);
    if (path.endsWith("/versions") && init?.method === "POST") return uploadResponse();
    if (path.endsWith("/versions")) return json(SKILL_VERSIONS);
    return json({ error: "not found" }, 404);
  });
}

async function submitUpload() {
  await waitFor(
    () => container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled === false,
  );
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

test.each([401, 503])(
  "an unconfirmed identity %s keeps version uploads blocked",
  async (status) => {
    let writes = 0;
    stubOwner(() => {
      writes += 1;
      return json({ error: "not submitted" }, 500);
    });
    const originalFetch = globalThis.fetch;
    vi.stubGlobal("fetch", (input: string, init?: RequestInit) =>
      String(input).endsWith("/me")
        ? json({ error: "identity unavailable" }, status)
        : originalFetch(input, init),
    );
    await render();
    await waitFor(() => queryClient.getQueryState(queryKeys.me)?.status === "error");
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(
      true,
    );
    expect(text()).toContain("登入狀態尚未確認；請先在頁首確認，再上傳新版本。");
    const input = container.querySelector<HTMLInputElement>("#skill-version-file")!;
    Object.defineProperty(input, "files", { value: [new File(["draft"], "draft.zip")] });
    await act(async () => input.dispatchEvent(new Event("change", { bubbles: true })));
    await act(async () =>
      container
        .querySelector("form")!
        .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })),
    );
    expect(writes).toBe(0);
  },
);

test("丙-151: 422 兩條 finding 的中文訊息都渲染，且不出現 Unprocessable Entity", async () => {
  stubOwner(() =>
    json(
      {
        errors: [
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

test("丙-151: 檔案輸入框之前有匯入頁同樣的規則句", async () => {
  stubOwner(() => json({ error: "unused" }, 500));
  await render();

  const rule = container.querySelector("ul.note");
  expect(rule, "找不到規則句的 <ul class=note>").not.toBeNull();
  expect(rule!.textContent).toContain("SKILL.md");
  expect(rule!.textContent).toContain("大小上限見拒絕訊息");

  const input = container.querySelector("#skill-version-file")!;
  expect(rule!.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test("丙-151: 413 印「檔案超過上限，請縮小套件再上傳。」", async () => {
  stubOwner(() => json({ error: "file is larger than 25 MB" }, 413));
  await render();
  await submitUpload();
  await waitFor(() => text().includes("上傳沒有成功") || text().includes("檔案超過上限"));

  expect(text()).toContain("檔案超過上限，請縮小套件再上傳。");
  expect(text()).not.toContain("file is larger than");
});

test.each([
  [false, "已存成 v3"],
  [true, "與現有的 v3 完全相同"],
] as const)("丙-151: 上傳結果 duplicate=%s 能前往精確版本", async (duplicate, message) => {
  stubOwner(() =>
    json({
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 3,
      content_hash: "sha256:version-three",
      duplicate,
      findings: { errors: [], warnings: [], infos: [] },
    }),
  );
  await render();
  await submitUpload();
  await waitFor(() => text().includes(message));

  const destination = container.querySelector<HTMLAnchorElement>(
    `a[href="/skills/${SKILL}/versions/${VERSION}"]`,
  );
  expect(destination?.textContent).toBe("開啟 v3 並繼續驗證");
});
