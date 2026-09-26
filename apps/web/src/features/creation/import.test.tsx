import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { ImportSkill } from "./import/ImportSkill.page";

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
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    children?: unknown;
  }) => (
    <a href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}>
      {children as never}
    </a>
  ),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
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

const ME = { user_id: "u-1", email: "a@b.c", display_name: "a", workspace_id: "ws-1" };

async function submitURL(url = "https://github.com/example/skill") {
  await render(<ImportSkill />, () => text().includes("匯入 Skill"));
  const input = container.querySelector<HTMLInputElement>('input[type="url"]')!;
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.bind(
      input,
    );
    setter(url);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    container
      .querySelector("form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

test("04 丙-150(a): a session that expired mid-import says 需要登入, not the raw server string", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST") return json({ error: "not authenticated" }, 401);
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("需要登入"));

  expect(text()).not.toContain("not authenticated");
});

test("04 丙-150(a): a 400 (bad zip / unreachable URL) gets the page's own sentence", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST") return json({ error: "bad request" }, 400);
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("這個檔案不是可用的 zip 套件，或網址抓不到內容。"));
});

test("a 413 (file too large) gets its own sentence", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST") return json({ error: "payload too large" }, 413);
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("檔案超過上限。"));
});

test("an unclassified server error (500) gets the generic retry sentence", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST") return json({ error: "internal error" }, 500);
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("匯入失敗，可以再按一次。"));

  expect(text()).not.toContain("internal error");
});

test("04 丙-152: a 422 renders every refused folder's real Chinese finding messages and codes", async () => {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST")
      return json(
        {
          shape: "plugin",
          plugin: { name: "all-bad" },
          skills: [],
          refused: [
            {
              path: "skills/one",
              findings: {
                errors: [
                  {
                    severity: "error",
                    code: "skill-md-missing",
                    path: "SKILL.md",
                    message: "套件根目錄找不到 SKILL.md",
                  },
                ],
                warnings: [],
                infos: [],
              },
            },
            {
              path: "skills/two",
              findings: {
                errors: [
                  {
                    severity: "error",
                    code: "name-invalid",
                    path: "SKILL.md",
                    message: "name 只能使用小寫英文字母、數字與單一連字號",
                  },
                ],
                warnings: [],
                infos: [],
              },
            },
          ],
          excluded_components: [],
        },
        422,
      );
    return json({ error: "not found" }, 404);
  });

  await submitURL();
  await waitFor(() => text().includes("套件根目錄找不到 SKILL.md"));

  expect(text()).toContain("skill-md-missing");
  expect(text()).toContain("skills/one");
  expect(text()).toContain("name 只能使用小寫英文字母、數字與單一連字號");
  expect(text()).toContain("name-invalid");
  expect(text()).toContain("skills/two");
});

const PLUGIN_IMPORT = {
  shape: "plugin",
  plugin: { name: "desk-tools", version: "1.4.0" },
  skills: [
    {
      path: "skills/tidy-notes",
      skill_id: "s-1",
      version_id: "v-1",
      version_number: 1,
      content_hash: "h1",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
    {
      path: "skills/split-csv",
      skill_id: "s-2",
      version_id: "v-2",
      version_number: 1,
      content_hash: "h2",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [
    {
      severity: "info",
      code: "plugin-component",
      path: "mcp.json",
      message: "這個 Plugin 宣告了 MCP server。",
    },
  ],
};

function stubImport(body: unknown) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = typeof input === "string" ? input : String(input);
    if (path.endsWith("/me")) return json(ME);
    if (init?.method === "POST") return json(body, 201);
    return json({ error: "not found" }, 404);
  });
}

test("a Plugin import lists every Skill it brought in, each with its own link", async () => {
  stubImport(PLUGIN_IMPORT);

  await submitURL();
  await waitFor(() => text().includes("匯入完成"));

  expect(text()).toContain("skills/tidy-notes");
  expect(text()).toContain("skills/split-csv");
  const links = [...container.querySelectorAll("a")].map((a) => a.getAttribute("href"));
  expect(links, "每個 Skill 都要有自己的連結，只列第一個等於把另一半藏起來").toEqual(
    expect.arrayContaining(["/skills/s-1", "/skills/s-2"]),
  );
});

test("a Plugin import says what a download of one Skill actually hands over", async () => {
  stubImport(PLUGIN_IMPORT);

  await submitURL();
  await waitFor(() => text().includes("匯入完成"));

  const compact = text().replace(/\s+/g, "");
  expect(compact).toContain("只安裝該Skill自己的目錄");
  expect(
    compact,
    "平台從不交出儲存的那一份套件：試跑只安裝該 Skill 的目錄，下載只有單一 Skill 的可攜套件",
  ).not.toContain("拿到的是整個Plugin");
});

test("a Plugin import discloses the components it did not import", async () => {
  stubImport(PLUGIN_IMPORT);

  await submitURL();
  await waitFor(() => text().includes("匯入完成"));

  expect(text()).toContain("mcp.json");
  expect(text()).toContain("plugin-component");
});

test("one refused folder does not hide the Skills that did come in", async () => {
  stubImport({
    ...PLUGIN_IMPORT,
    skills: [PLUGIN_IMPORT.skills[0]],
    refused: [
      {
        path: "skills/broken",
        findings: {
          errors: [
            {
              severity: "error",
              code: "skill-md-missing",
              path: "SKILL.md",
              message: "套件根目錄找不到 SKILL.md",
            },
          ],
          warnings: [],
          infos: [],
        },
      },
    ],
  });

  await submitURL();
  await waitFor(() => text().includes("匯入完成"));

  expect(text()).toContain("skills/tidy-notes");
  expect(text()).toContain("skills/broken");
  expect(text()).toContain("skill-md-missing");
});
