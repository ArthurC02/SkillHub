import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { expect, test } from "vitest";
import App from "./App";
import "../features/catalog/home/Home.page";
import { DEFAULT_WAIT_MS, pollUntil } from "../testing/poll";

function waitFor(done: () => boolean, timeoutMs = DEFAULT_WAIT_MS) {
  return pollUntil(done, () => document.body.textContent, timeoutMs);
}

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
  await waitFor(
    () =>
      container.querySelector("footer") !== null &&
      container.querySelector("[data-loading]") === null,
  );

  expect(container.textContent).toContain("Skill Hub");

  const build = Array.from(container.querySelectorAll<HTMLDetailsElement>("footer details")).find(
    (d) => (d.querySelector("summary")?.textContent ?? "").includes("Build"),
  );
  expect(build, "the footer has no Build 識別碼 disclosure (IA-11)").toBeTruthy();
  expect(build!.open, "the identifier is folded by default (§2.6)").toBe(false);
  expect(build!.querySelector("code")?.textContent ?? "").toMatch(/\S/);
});
