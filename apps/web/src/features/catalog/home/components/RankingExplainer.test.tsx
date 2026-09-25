import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
import { RankingExplainer } from "./RankingExplainer";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
});

async function mount() {
  root = createRoot(container);
  await act(async () => {
    root.render(
      <StrictMode>
        <RankingExplainer />
      </StrictMode>,
    );
  });
}

// JSX wraps long Chinese prose mid-sentence, so the rendered text carries
// spaces the source does not; comparing without whitespace keeps the assertion
// about the wording rather than about where Prettier broke the line.
function wordingOfItemStartingWith(prefix: string): string {
  const items = [...container.querySelectorAll<HTMLElement>("li")];
  const found = items.find((item) => item.querySelector("strong")?.textContent?.startsWith(prefix));
  expect(found, `找不到開頭是「${prefix}」的那一格`).not.toBeUndefined();
  return (found!.textContent ?? "").replace(/\s+/g, "");
}

test("排序說明：只能用關鍵字比對時，畫面說出中文幾乎找不到東西，不把它說成備援腿", async () => {
  await mount();
  const text = wordingOfItemStartingWith("例外一");

  expect(text, "中文在這個狀態下的實際處境沒有被說出來").toContain(
    "中文查詢在這個狀態下幾乎找不到東西",
  );
  expect(text, "沒有否認它是一條備援腿，讀者會以為中文有第二條腿接住").toContain(
    "不是一條在後面接住中文的備援腿",
  );
});

test("排序說明：兩個例外格都還在，中文那段沒有蓋掉語意索引缺席的說明", async () => {
  await mount();

  expect(wordingOfItemStartingWith("例外二")).toContain("算不出相似度");
});
