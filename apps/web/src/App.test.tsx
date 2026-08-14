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
});
