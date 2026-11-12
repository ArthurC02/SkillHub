import { expect, test } from "vitest";
import { AllowedToolsSection } from "./AllowedToolsSection";
import { mountedComponent } from "../../../../testing/component";

const { mount, text } = mountedComponent();

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
