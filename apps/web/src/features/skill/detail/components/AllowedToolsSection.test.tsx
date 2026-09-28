import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
import { AllowedToolsSection } from "./AllowedToolsSection";

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

async function mount(node: React.ReactElement) {
  root = createRoot(container);
  await act(async () => {
    root.render(<StrictMode>{node}</StrictMode>);
  });
}

const text = () => container.textContent ?? "";

test("lists each declared tool when the package declares allowed-tools", async () => {
  await mount(<AllowedToolsSection allowedTools={["Bash", "WebFetch"]} scanStatus="scanned" />);

  expect(text()).toContain("Bash");
  expect(text()).toContain("WebFetch");
  expect(text()).toContain("未經驗證");
});

test("says unmeasured when there is no scan result to read", async () => {
  await mount(<AllowedToolsSection allowedTools={undefined} scanStatus="unavailable" />);

  expect(text()).toContain("未測量");
  expect(text()).not.toContain("不適用");
});

test("says not-applicable, not unmeasured, when a completed scan found no declaration", async () => {
  await mount(<AllowedToolsSection allowedTools={[]} scanStatus="scanned" />);

  expect(text()).toContain("不適用");
  expect(text()).not.toContain("未測量");
});
