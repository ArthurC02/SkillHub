import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
import { Tip } from "./components/Tip";

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
        <p>
          可以關掉這一頁（平台在跑，不是你的瀏覽器）。
          <Tip anchor="為什麼可以關掉這一頁">
            Run 是資料庫裡的一個工作，由平台的 worker 消費；瀏覽器不在那條路徑上。
          </Tip>
        </p>
      </StrictMode>,
    );
  });
}

test("Tip: 內容從一開始就在 DOM 裡，預設收合，按鈕說得出自己開了沒", async () => {
  await mount();
  const button = container.querySelector<HTMLButtonElement>("button.tip-trigger")!;
  const content = container.querySelector<HTMLElement>(".tip-content")!;

  expect(content, "內容不在 DOM 裡——那是 tooltip 的形狀，不是 disclosure").not.toBeNull();
  expect(content.hidden, "預設要收合").toBe(true);
  expect(content.tagName, "§4.7：內容是 <p>，行長的尺才看得到它").toBe("P");
  expect(content.getAttribute("data-role"), "D 類要戴 teaching 標記，預算機器才數得到").toBe(
    "teaching",
  );
  expect(button.getAttribute("aria-expanded")).toBe("false");
  expect(button.getAttribute("aria-controls"), "aria-controls 要指向內容的 id").toBe(content.id);
  expect(button.textContent?.trim(), "觸發鈕永遠帶可見文字（§2.13 第 4 條）").toBe(
    "為什麼可以關掉這一頁",
  );
  expect(button.getAttribute("title"), "不得是 title（§2.13 第 2 條）").toBeNull();
});

test("Tip: 點一下開、再點關；Esc 關掉而焦點留在按鈕上", async () => {
  await mount();
  const button = container.querySelector<HTMLButtonElement>("button.tip-trigger")!;
  const content = container.querySelector<HTMLElement>(".tip-content")!;

  await act(async () => button.click());
  expect(content.hidden).toBe(false);
  expect(button.getAttribute("aria-expanded")).toBe("true");

  await act(async () => button.click());
  expect(content.hidden).toBe(true);
  expect(button.getAttribute("aria-expanded")).toBe("false");

  await act(async () => button.click());
  button.focus();
  await act(async () => {
    button.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  });
  expect(content.hidden, "Esc 要關").toBe(true);
  expect(document.activeElement, "關掉之後焦點不能跑掉").toBe(button);
});
