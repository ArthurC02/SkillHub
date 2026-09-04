import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { expect, test } from "vitest";
import App from "./App";

test("renders the app shell", async () => {
  const container = document.createElement("div");
  document.body.appendChild(container);

  await act(async () => {
    createRoot(container).render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });

  expect(container.textContent).toContain("Skill Hub");

  // 資訊架構 IA-11: every page can say which build it is. Folded (§2.6), so the
  // summary names the identifier and the value is a <code> that is never empty
  // — a build with no id says so in words, it does not print a blank.
  const build = Array.from(container.querySelectorAll<HTMLDetailsElement>("footer details")).find(
    (d) => (d.querySelector("summary")?.textContent ?? "").includes("Build"),
  );
  expect(build, "the footer has no Build 識別碼 disclosure (IA-11)").toBeTruthy();
  expect(build!.open, "the identifier is folded by default (§2.6)").toBe(false);
  expect(build!.querySelector("code")?.textContent?.trim()).not.toBe("");
});
