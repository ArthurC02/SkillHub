import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
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

test("the component shows an unreadable timestamp the same way formatAt does", async () => {
  await act(async () => root.render(<Timestamp at="not-a-time" />));

  expect(box.querySelector("time")!.textContent).toBe("not-a-time（無法解讀的時間格式）");
});
