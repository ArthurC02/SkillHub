import { StrictMode, act, type Ref } from "react";
import { createRoot, type Root } from "react-dom/client";
import { onlineManager, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { VersionUpload } from "./version/components/VersionUpload";
import { IMPORT_LIMITS, SKILL_VERSIONS, platformResponse } from "../../testing/fixtures/platform";
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
    if (path === "/skills/import/limits") return json(IMPORT_LIMITS);
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

test("version upload shows the deployment archive limits before choosing a file", async () => {
  stubOwner(() => json({ error: "unused" }, 500));
  await render();

  const rule = container.querySelector("ul.note");
  expect(rule, "找不到規則句的 <ul class=note>").not.toBeNull();
  expect(rule!.textContent).toContain("SKILL.md");
  await waitFor(() => text().includes("zip 最大 10.0 MB"));
  expect(rule!.textContent).toContain("解壓後總量最大 100.0 MB");
  expect(rule!.textContent).toContain("最多 100 個檔案");
  expect(rule!.textContent).toContain("單一檔案最大 1.0 MB");
  expect(rule!.textContent).toContain("路徑最深 8 層");

  const input = container.querySelector("#skill-version-file")!;
  expect(rule!.compareDocumentPosition(input) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test.each([
  ["server failure", 503, { error: "unavailable" }],
  ["unreadable ceiling", 200, { ...IMPORT_LIMITS, max_zip_bytes: null }],
])(
  "a %s hides cached upload limits and blocks even a direct submit",
  async (_label, status, body) => {
    let writes = 0;
    stubOwner(() => {
      writes++;
      return json({ error: "not submitted" }, 500);
    });
    queryClient.setQueryData(queryKeys.skills.importLimits, IMPORT_LIMITS);
    const originalFetch = globalThis.fetch;
    vi.stubGlobal("fetch", (input: string, init?: RequestInit) =>
      String(input).endsWith("/skills/import/limits")
        ? json(body, status as number)
        : originalFetch(input, init),
    );
    await render();
    await waitFor(() => text().includes("暫時無法讀取上傳限制"));
    expect(text()).not.toContain("zip 最大");
    expect(text()).toContain("上傳限制尚未確認；請先重新讀取，再上傳新版本。");
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(
      true,
    );
    const input = container.querySelector<HTMLInputElement>("#skill-version-file")!;
    Object.defineProperty(input, "files", { value: [new File(["draft"], "draft.zip")] });
    await act(async () => input.dispatchEvent(new Event("change", { bubbles: true })));
    await act(async () =>
      container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true })),
    );
    expect(writes).toBe(0);
  },
);

test("a paused upload limits reread withholds cached rules and blocks submission", async () => {
  stubOwner(() => json({ error: "not submitted" }, 500));
  await render();
  await waitFor(
    () => container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled === false,
  );
  try {
    await act(async () => {
      onlineManager.setOnline(false);
      void queryClient.refetchQueries({ queryKey: queryKeys.skills.importLimits });
    });
    await waitFor(() => text().includes("等待連線以讀取上傳限制…"));
    expect(text()).not.toContain("zip 最大");
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(
      true,
    );
    expect(queryClient.getQueryState(queryKeys.me)?.fetchStatus).toBe("idle");
    expect(queryClient.getQueryState(queryKeys.skills.versions(SKILL))?.fetchStatus).toBe("idle");
  } finally {
    await queryClient.cancelQueries();
    onlineManager.setOnline(true);
  }
});

async function chooseFileOfSize(size: number) {
  const input = container.querySelector<HTMLInputElement>("#skill-version-file")!;
  const file = new File(["zip"], "skill.zip", { type: "application/zip" });
  Object.defineProperty(file, "size", { value: size });
  Object.defineProperty(input, "files", { value: [file], configurable: true });
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

test("a zip past the deployment ceiling is refused before it is uploaded, one at the ceiling is not", async () => {
  let posts = 0;
  stubOwner(() => {
    posts += 1;
    return json({ error: "file is larger than allowed" }, 413);
  });
  await render();
  const submit = () => container.querySelector<HTMLButtonElement>('button[type="submit"]')!;
  await waitFor(() => submit().disabled === false);

  await chooseFileOfSize(IMPORT_LIMITS.max_zip_bytes);
  expect(submit().disabled).toBe(false);
  expect(container.querySelector('[role="alert"]')).toBeNull();

  await chooseFileOfSize(IMPORT_LIMITS.max_zip_bytes + 1);
  expect(submit().disabled).toBe(true);
  const reason = container.querySelector(`#${submit().getAttribute("aria-describedby")}`);
  expect(reason?.textContent).toBe(
    "skill.zip 是 10,485,761 位元組，超過 zip 上限 10,485,760 位元組，沒有送出。",
  );
  await act(async () => {
    container
      .querySelector("form.version-upload")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  expect(posts).toBe(0);
});

test("丙-151: 413 印「檔案超過上限，請縮小套件再上傳。」", async () => {
  stubOwner(() => json({ error: "file is larger than 25 MB" }, 413));
  await render();
  await submitUpload();
  await waitFor(() => text().includes("上傳沒有成功") || text().includes("檔案超過上限"));

  expect(text()).toContain("檔案超過上限，請縮小套件再上傳。");
  expect(text()).not.toContain("file is larger than");
});

test("a 429 carrying the server's wait time shows that time", async () => {
  stubOwner(() => json({ error: "太多請求了，請在 9 秒後再試。" }, 429));
  await render();
  await submitUpload();
  await waitFor(() => text().includes("太多請求了，請在 9 秒後再試。"));

  expect(text()).not.toContain("上傳太頻繁了，請稍等一下再上傳。");
});

test("a 429 says the uploads came too fast and to wait", async () => {
  stubOwner(() => json({ error: "rate limit exceeded" }, 429));
  await render();
  await submitUpload();
  await waitFor(() => text().includes("上傳太頻繁"));

  expect(text()).toContain("上傳太頻繁了，請稍等一下再上傳。");
  expect(text()).not.toContain("這次上傳未被接受");
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

test("an upload's result stays on the version page it was sent from, not the one it leads to", async () => {
  const NEXT = "33333333-3333-4333-8333-333333333333";
  stubOwner(() =>
    json({
      skill_id: SKILL,
      version_id: NEXT,
      version_number: 3,
      content_hash: "sha256:version-three",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    }),
  );
  await render();
  await submitUpload();
  await waitFor(() => text().includes("已存成 v3"));

  await act(async () =>
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <VersionUpload skillId={SKILL} versionId={NEXT} contextConfirmed />
        </QueryClientProvider>
      </StrictMode>,
    ),
  );

  expect(text()).toContain("上傳新版本");
  expect(text()).not.toContain("已存成 v3");
  expect(container.querySelector(`a[href="/skills/${SKILL}/versions/${NEXT}"]`)).toBeNull();
});
