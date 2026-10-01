import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Timestamp } from "./Timestamp";
import { formatAt } from "./Timestamp.model";

let box: HTMLDivElement, root: Root;

beforeEach(() => {
  box = document.createElement("div");
  document.body.appendChild(box);
  root = createRoot(box);
});

afterEach(async () => {
  await act(async () => root.unmount());
  box.remove();
  vi.useRealTimers();
});

test("an unreadable timestamp is shown as it came, and says it could not be read", () => {
  expect(formatAt("not-a-time")).toBe("not-a-time（無法解讀的時間格式）");
});

test("the component prints the same absolute time an <option> label gets", async () => {
  const at = "2026-09-05T08:30:00Z";
  await act(async () => root.render(<Timestamp at={at} />));

  const time = box.querySelector("time")!;
  expect(time.getAttribute("datetime")).toBe(at);
  expect(time.textContent).toBe(formatAt(at));
  expect(time.textContent).toMatch(/2026/);
});

async function relativeTextAfter(elapsedMs: number): Promise<string> {
  const at = "2026-09-05T08:30:00Z";
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(at).getTime() + elapsedMs);
  await act(async () => root.render(<Timestamp at={at} relative />));
  return box.querySelector("time")!.textContent!.slice(formatAt(at).length);
}

test.each([
  [-1_000, "（剛剛）"],
  [0, "（0 秒前）"],
  [59_000, "（59 秒前）"],
  [60_000, "（1 分鐘前）"],
  [3_599_000, "（59 分鐘前）"],
  [3_600_000, "（1 小時前）"],
  [86_400_000, "（1 天前）"],
])("a timestamp %i ms old reads %s", async (elapsedMs, expected) => {
  expect(await relativeTextAfter(elapsedMs)).toBe(expected);
});

test("the elapsed time advances while the timestamp stays mounted", async () => {
  vi.useFakeTimers();
  const at = "2026-09-05T08:30:00Z";
  vi.setSystemTime(new Date(at).getTime() + 59_000);
  await act(async () => root.render(<Timestamp at={at} relative />));
  expect(box.querySelector("time")!.textContent).toMatch(/（59 秒前）$/);

  vi.setSystemTime(new Date("2026-09-05T09:30:00Z"));
  await act(async () => vi.advanceTimersByTimeAsync(1000));

  expect(box.querySelector("time")!.textContent).toMatch(/（1 小時前）$/);
});

test("the component shows an unreadable timestamp the same way formatAt does", async () => {
  await act(async () => root.render(<Timestamp at="not-a-time" />));

  expect(box.querySelector("time")!.textContent).toBe("not-a-time（無法解讀的時間格式）");
});
