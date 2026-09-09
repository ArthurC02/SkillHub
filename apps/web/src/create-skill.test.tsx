import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { router } from "./router";

/**
 * `/workspace/creations` 的旗標關閉那一半（⛔ `01` §10 邊界 1，ADR-052）。
 *
 * **這支測試是那一頁存在的前提，不是它的附錄。** `ia.test.ts` 的 `FLAG_OFF_ASSERTED`
 * 名冊逐字寫著：先寫旗標關閉的斷言，再把檔案的名字加進名冊——因為「開工不等於曝光」
 * 這條邊界失效時**沒有症狀**，畫面看起來完全正確，壞掉的是 `01` §11.2 第一段漏斗的
 * 意義，而那一段只有一次機會與十二個人。
 *
 * 這一頁比其他三個掛載點更需要它：另外三個都是「某一頁上的一塊」，旗標關著時那一塊
 * 不渲染就結束了；這一頁**本身**就是那個能力，而且它有一個任何人都猜得到的網址。
 * 所以斷言有兩半——看不到工作台，而且**看得到一條回得去的路**（§2.2 第三向：擋住人
 * 的訊息要說出下一步）。
 */
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

function stubMe(features?: Record<string, boolean>) {
  vi.stubGlobal("fetch", (input: string) => {
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    if (path === "/me")
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
    return Promise.resolve(new Response(JSON.stringify({}), { status: 200 }));
  });
}

async function visit() {
  router.update({ history: createMemoryHistory({ initialEntries: ["/workspace/creations"] }) });
  await act(async () => {
    root = createRoot(container);
    root.render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
  });
  await act(async () => new Promise((r) => setTimeout(r, 40)));
}

const text = () => container.textContent ?? "";

test("⛔ with the flag off, /workspace/creations is not a workbench and says so", async () => {
  stubMe();
  await visit();

  expect(text()).toContain("這一頁現在不存在");
  // 工作台的兩個入口各自的識別：生成表單的 textarea、互動創作的標題。
  expect(container.querySelector("#generate-task"), "旗標關著卻渲染了生成表單").toBeNull();
  expect(text(), "旗標關著卻渲染了互動創作").not.toContain("和 Agent 一起創作");
  // 擋住人的訊息要說出下一步（設計 §2.2 第三向）。
  const hrefs = Array.from(container.querySelectorAll("a")).map((a) => a.getAttribute("href"));
  expect(hrefs, "沒有給一條回得去的路").toContain("/workspace/skills");
});

test("with generate_skill on, the page is the generation workbench", async () => {
  stubMe({ generate_skill: true });
  await visit();

  expect(container.querySelector("#generate-task")).not.toBeNull();
  expect(text()).not.toContain("這一頁現在不存在");
});

/**
 * 2026-09-09，負責人回報：「並沒有取消回到上一頁的按鈕」。
 *
 * 工作台在卡片裡就地展開時不需要出口——它周圍就是那一頁。搬成一個位址之後周圍什麼
 * 都沒有了，而**這一頁不在導覽列上**（§0.1 R7），所以連導覽列那條退路也沒有。這正是
 * §0.1 R3 的出處在講的危險，而 IA-12 說「唯一的入邊就在使用者按上一頁會回到的那一頁
 * 上」——那句話只對瀏覽器的上一頁成立，對從書籤或別人給的連結進來的人不成立。
 *
 * 兩個旗標狀態都斷言：工作台開著的時候最容易忘，關著的時候那句話本來就帶著出口。
 */
test.each([
  ["旗標開著", { generate_skill: true }],
  ["旗標關著", undefined],
] as const)("%s 時這一頁都有一條回得去的路", async (_label, features) => {
  stubMe(features);
  await visit();

  // 排除頁首的導覽列：`ia.test.ts` 的入邊計數也不算它（§2.3 第一句），而這一支問的
  // 是同一件事——這一頁自己有沒有給出路，不是瀏覽器的家具有沒有。
  const back = Array.from(container.querySelectorAll("main a")).filter(
    (a) => a.getAttribute("href") === "/workspace/skills",
  );
  expect(back.length, "這一頁沒有出口").toBeGreaterThan(0);
  // 同一件事一頁只出現一次（設計 §3 第 14 條）。
  expect(back.length, "回去的路出現了兩次").toBe(1);
});

/**
 * `creation_skill` 只在 `generate_skill` 也開著時由 Go 送出
 * （`apiserver/app.go` 的 `entryPointFeatures`），所以這一支同時開兩個——
 * 只開 `creation_skill` 是一個伺服器產不出來的狀態。
 */
test("with creation_skill on as well, the page is the conversation", async () => {
  stubMe({ generate_skill: true, creation_skill: true });
  await visit();

  expect(text()).toContain("和 Agent 一起創作");
  expect(container.querySelector("#generate-task"), "兩個工作台同時掛上了").toBeNull();
});
