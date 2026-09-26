import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { queryClient } from "../../core/api/queryClient";
import { PublicPublication } from "./PublicPublication.page";
import { PublisherSection } from "./components/PublisherSection";
import { PublishPanel } from "./components/PublishPanel";
import {
  PUBLISHING_REFUSAL_LABEL,
  type PublishingRefusalReason,
  refusalSentence,
} from "./publishing.model";
import {
  OWN_PUBLICATION,
  OWN_PUBLISHER,
  PUBLIC_BUNDLE_PUBLICATION,
  PUBLIC_PUBLICATION,
  PUBLICATION,
  PUBLISHER,
  SKILL,
  skillDetail,
} from "../../testing/fixtures/platform";
import type { SkillDetail } from "../../core/api/types";

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
  useParams: () => ({ publisher: PUBLISHER, name: PUBLICATION }),
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
function button(label: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes(label),
  );
}

function stub(routes: Record<string, { body: unknown; status?: number }>) {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    const url = String(input);
    calls.push(url);
    const path = url.replace(/^https?:\/\/[^/]+/, "").split("?")[0];
    const hit = routes[path];
    if (hit) return json(hit.body, hit.status ?? 200);
    return json({ error: "not found" }, 404);
  });
  return calls;
}

const PUB_ADDRESS = `/publications/${PUBLISHER}/${PUBLICATION}`;

test("PACK-004 available 公開頁：每一個允收欄位都出現", async () => {
  stub({ [PUB_ADDRESS]: { body: PUBLIC_PUBLICATION } });
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(text()).toContain("PDF Summariser");
  expect(text()).toContain("把 PDF 整理成摘要");
  expect(text()).toContain(PUBLISHER);
  expect(text()).toContain("v2");
  expect(text()).toContain("sha256:aa");
  expect(text()).toContain("靜態掃描");
  expect(text()).toContain("MIT");
  expect(text()).toContain("可再散布");
  expect(text()).toContain("這個發佈物還沒有經過目錄審核");
  expect(text()).toContain("登入後可以下載這一版的標準 Agent Skill 套件");
});

// T3 等價類: 未登入（不呼叫 /me）也能讀到公開頁
test("PACK-004 公開頁不需要登入：從不呼叫 /me 也能顯示內容", async () => {
  const calls = stub({ [PUB_ADDRESS]: { body: PUBLIC_PUBLICATION } });
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(calls.some((u) => u.includes("/me"))).toBe(false);
});

// T2 決策表：五種不可用狀態只顯示理由，不顯示內容；delisted 額外顯示撤回時間
describe("PACK-004 不可用狀態", () => {
  const CASES: { value: string; label: string; note: string; delistedAt?: string }[] = [
    {
      value: "delisted",
      label: "作者已撤回",
      note: "作者撤回了這個發佈物，這一頁不再提供它的內容。",
      delistedAt: "2026-09-01T00:00:00Z",
    },
    {
      value: "withdrawn",
      label: "已不提供",
      note: "這個發佈物指向的 Skill 已經被作者刪除。",
    },
    { value: "taken_down", label: "已不提供", note: "這個 Skill 已被平台下架。" },
    {
      value: "held",
      label: "已不提供",
      note: "這個 Skill 的內容因授權問題被保留，釐清之前不提供。",
    },
    {
      value: "not_redistributable",
      label: "已不提供",
      note: "這個 Skill 目前的授權判定不允許再散布。",
    },
  ];

  for (const testCase of CASES) {
    test(`availability=${testCase.value} 只顯示理由，不顯示 Skill 內容`, async () => {
      stub({
        [PUB_ADDRESS]: {
          body: {
            ...PUBLIC_PUBLICATION,
            availability: { value: testCase.value, label: testCase.label, note: testCase.note },
            delisted_at: testCase.delistedAt,
            skill: undefined,
            release: undefined,
          },
        },
      });
      await render(<PublicPublication />, () => text().includes(testCase.label));

      expect(text()).toContain(testCase.note);
      expect(text()).not.toContain("PDF Summariser");
      expect(text()).not.toContain("sha256:aa");
      if (testCase.delistedAt) {
        expect(text()).toContain("撤回時間");
      } else {
        expect(text()).not.toContain("撤回時間");
      }
    });
  }
});

test("PACK-004 沒有這個發佈物：404 說出來，不是空白", async () => {
  stub({ [PUB_ADDRESS]: { body: { error: "no publication has this address" }, status: 404 } });
  await render(<PublicPublication />, () => text().includes("沒有這個發佈物"));
});

const ACQUIRE_ADDRESS = `${PUB_ADDRESS}/acquisitions`;

function stubAcquire(post: () => { body: unknown; status?: number }) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === ACQUIRE_ADDRESS && init?.method === "POST") {
      const { body, status } = post();
      return json(body, status ?? 201);
    }
    if (path === PUB_ADDRESS) return json(PUBLIC_PUBLICATION);
    return json({ error: "not found" }, 404);
  });
}

// T2 等價類: acquisition.available=false 時，即使 availability 是 available，也不顯示按鈕
test("PACK-006 acquisition.available=false 時不顯示下載按鈕，只顯示 note", async () => {
  stub({
    [PUB_ADDRESS]: {
      body: {
        ...PUBLIC_PUBLICATION,
        acquisition: { available: false, note: "這個發佈物目前不提供下載，原因見上方。" },
      },
    },
  });
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(text()).toContain("這個發佈物目前不提供下載，原因見上方。");
  expect(button("下載")).toBeUndefined();
});

test("PACK-006 acquisition.available=true 時顯示下載按鈕，按下後用 content_url 觸發下載", async () => {
  stubAcquire(() => ({
    body: {
      artifact_id: "art-1",
      file_name: "pdf-summariser-v2.zip",
      size_bytes: 100,
      content_hash: "sha256:zz",
      expires_at: "2099-01-01T00:00:00Z",
      duplicate: false,
      content_url: "/downloads/art-1/content",
    },
  }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  expect(button("下載")).toBeDefined();
  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("pdf-summariser-v2.zip"));

  const link = Array.from(container.querySelectorAll("a")).find((a) =>
    (a.textContent ?? "").includes("pdf-summariser-v2.zip"),
  );
  expect(link?.getAttribute("href")).toBe("/downloads/art-1/content");
});

test("PACK-006 401：未登入按下下載，交給既有登入元件說一次", async () => {
  stubAcquire(() => ({ body: { error: "not authenticated" }, status: 401 }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("這個下載需要登入"));

  expect(text()).not.toContain("not authenticated");
});

test("PACK-006 403：未受邀，顯示伺服器的中文說明", async () => {
  const NOT_INVITED =
    "Skill Hub 還在封測：瀏覽與 Skill 詳情對所有人開放，但 Fork、試跑與下載只開放給受邀的測試者。";
  stubAcquire(() => ({ body: { error: NOT_INVITED }, status: 403 }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("還在封測"));

  expect(text()).toContain(NOT_INVITED);
});

test("PACK-006 409：理由是 availability，訊息就是伺服器的字串", async () => {
  stubAcquire(() => ({
    body: { error: "作者撤回了這個發佈物，這一頁不再提供它的內容。", reason: "delisted" },
    status: 409,
  }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("作者撤回了這個發佈物"));
});

test("PACK-006 422：打包器拒絕，訊息就是伺服器的字串", async () => {
  stubAcquire(() => ({
    body: {
      error: "這個 Skill 的內容因授權問題尚未釐清而被保留，所以不能發佈",
      reason: "license_hold",
    },
    status: 422,
  }));
  await render(<PublicPublication />, () => text().includes("PDF Summariser"));

  await act(async () => button("下載")?.click());
  await waitFor(() => text().includes("授權問題尚未釐清"));
});

test("PACK-018 Bundle 公開頁列出成員，且每一次 Release 逐項說出誰升級、誰加入、誰移除", async () => {
  stub({ [PUB_ADDRESS]: { body: PUBLIC_BUNDLE_PUBLICATION } });
  await render(<PublicPublication />, () => text().includes("一組跟 PDF 有關的 Skill"));

  expect(text()).toContain("summariser · v3");
  expect(text()).toContain("splitter · v1");
  expect(text()).toContain("summariser：v2 升到 v3");
  expect(text()).toContain("splitter：新增（v1）");
});

test("PACK-003/004 每一個 PublishingRefusal.reason 都有一句中文", () => {
  const REASONS: PublishingRefusalReason[] = [
    "no_publisher",
    "already_registered",
    "name_taken",
    "name_is_permanent",
    "name_shape",
    "name_reserved",
    "license_hold",
    "not_redistributable",
    "license_unknown",
    "validation_blocked",
    "rights_not_attested",
  ];
  expect(Object.keys(PUBLISHING_REFUSAL_LABEL).sort()).toEqual([...REASONS].sort());
  for (const reason of REASONS) {
    expect(PUBLISHING_REFUSAL_LABEL[reason], `${reason} 沒有句子`).toMatch(/\p{Script=Han}/u);
  }
});

// PublisherSection ----------------------------------------------------

test("PublisherSection：已註冊時顯示名稱與永久事實，不顯示表單", async () => {
  stub({ "/me/publisher": { body: OWN_PUBLISHER } });
  await render(<PublisherSection />, () => text().includes(PUBLISHER));

  expect(text()).toContain("沒有改名的功能");
  expect(container.querySelector("form")).toBeNull();
});

test("PublisherSection：未註冊時顯示名稱規則、永久警語與表單", async () => {
  stub({ "/me/publisher": { body: { error: "no publisher" }, status: 404 } });
  await render(<PublisherSection />, () => text().includes("還沒有註冊發佈者名稱"));

  expect(text()).toContain("小寫英文字母、數字與連字號");
  expect(text()).toContain("沒有改名的功能");
  expect(container.querySelector("form")).not.toBeNull();
});

test("PublisherSection：409 already_registered 顯示中文，不顯示英文原文", async () => {
  const calls: string[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    calls.push(url);
    if (init?.method === "POST") {
      return json(
        { error: "this account already has a publisher name", reason: "already_registered" },
        409,
      );
    }
    return json({ error: "no publisher" }, 404);
  });
  await render(<PublisherSection />, () => text().includes("還沒有註冊發佈者名稱"));

  const input = container.querySelector("input")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(input, "acme-tools");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => button("註冊")?.click());
  await waitFor(() => text().includes("已經有一個發佈者名稱了"));

  expect(text()).not.toContain("this account already has a publisher name");
});

test("PublisherSection：422 name_shape 顯示中文", async () => {
  vi.stubGlobal("fetch", (_input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      return json(
        {
          error: "a name is 1 to 64 lowercase letters, digits and single hyphens",
          reason: "name_shape",
        },
        422,
      );
    }
    return json({ error: "no publisher" }, 404);
  });
  await render(<PublisherSection />, () => text().includes("還沒有註冊發佈者名稱"));

  const input = container.querySelector("input")!;
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(input, "Bad Name!");
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => button("註冊")?.click());
  await waitFor(() => text().includes("名稱格式不對"));

  expect(text()).not.toContain("lowercase letters");
});

// PublishPanel ----------------------------------------------------

function detail(over: Partial<SkillDetail> = {}): SkillDetail {
  return { ...skillDetail(SKILL, "PDF Summariser"), ...over };
}

test("PublishPanel：未登入只說明需要登入，不顯示任何發佈狀態", async () => {
  await render(<PublishPanel skill={detail()} isLoggedIn={false} isOwner={false} />, () =>
    text().includes("發佈需要登入"),
  );

  expect(text()).not.toContain("發佈狀態");
});

test("PublishPanel：不是擁有者時整塊不顯示", async () => {
  await render(
    <div data-testid="wrap">
      <PublishPanel skill={detail()} isLoggedIn={true} isOwner={false} />
    </div>,
    () => container.querySelector("[data-testid=wrap]") !== null,
  );

  expect(text()).toBe("");
});

test("PublishPanel：還沒有發佈者時，指向帳號頁註冊", async () => {
  stub({ "/me/publisher": { body: { error: "no publisher" }, status: 404 } });
  await render(<PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />, () =>
    text().includes("註冊一個發佈者名稱"),
  );

  const link = container.querySelector("a")!;
  expect(link.getAttribute("href")).toBe("/workspace/account");
});

test("PublishPanel：尚未發佈時，名稱欄預設為 Skill 名稱", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: { error: "not published" }, status: 404 },
  });
  await render(
    <PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />,
    () => container.querySelector("input") !== null,
  );

  const input = container.querySelector("input") as HTMLInputElement;
  expect(input.value).toBe("PDF Summariser");
});

test("PublishPanel：已發佈時顯示公開位址、狀態與最新 Release", async () => {
  stub({
    "/me/publisher": { body: OWN_PUBLISHER },
    [`/skills/${SKILL}/publication`]: { body: OWN_PUBLICATION },
  });
  await render(<PublishPanel skill={detail()} isLoggedIn={true} isOwner={true} />, () =>
    text().includes("公開位址"),
  );

  expect(text()).toContain(`/p/${PUBLISHER}/${PUBLICATION}`);
  expect(text()).toContain("已發佈");
  expect(text()).toContain("v2");
  expect(button("發佈目前的版本")).toBeDefined();
  expect(button("撤回")).toBeDefined();
});

describe("PublishPanel：勾選框只在 self_supplied／generated 出現", () => {
  const CASES: { value: SkillDetail["redistribution"]["value"]; needsCheckbox: boolean }[] = [
    { value: "allowed", needsCheckbox: false },
    { value: "self_supplied", needsCheckbox: true },
    { value: "generated", needsCheckbox: true },
  ];

  for (const testCase of CASES) {
    test(`redistribution=${testCase.value}`, async () => {
      stub({
        "/me/publisher": { body: OWN_PUBLISHER },
        [`/skills/${SKILL}/publication`]: { body: { error: "not published" }, status: 404 },
      });
      const skill = detail({
        redistribution: { value: testCase.value, label: "", note: "" },
      });
      await render(
        <PublishPanel skill={skill} isLoggedIn={true} isOwner={true} />,
        () => container.querySelector("input") !== null,
      );

      const checkbox = container.querySelector('input[type="checkbox"]');
      if (testCase.needsCheckbox) {
        expect(checkbox).not.toBeNull();
        expect(button("發佈")?.disabled).toBe(true);
        expect(text()).toContain("先勾選下面的聲明才能發佈");
      } else {
        expect(checkbox).toBeNull();
        expect(button("發佈")?.disabled).toBe(false);
      }
    });
  }
});

test("refusalSentence: 沒有 reason 欄位時回傳 undefined，不誤植成中文句", () => {
  expect(refusalSentence(new Error("boom"))).toBeUndefined();
});
