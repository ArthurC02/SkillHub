import { afterEach, expect, test } from "vitest";
import { Locator } from "./app";

const LATE_MS = 40;

afterEach(() => {
  document.body.replaceChildren();
});

function later(build: () => void) {
  setTimeout(build, LATE_MS);
}

const byId = (id: string) => new Locator(() => [...document.querySelectorAll(`#${id}`)]);

test("fill waits for an input that renders after the call", async () => {
  later(() => {
    const input = document.createElement("input");
    input.id = "late-input";
    document.body.append(input);
  });
  await byId("late-input").fill("50");
  expect((document.getElementById("late-input") as HTMLInputElement).value).toBe("50");
});

test("click waits until a disabled button is enabled", async () => {
  const button = document.createElement("button");
  button.id = "late-button";
  button.disabled = true;
  let clicks = 0;
  button.addEventListener("click", () => {
    clicks += 1;
  });
  document.body.append(button);
  later(() => {
    button.disabled = false;
  });
  await byId("late-button").click();
  expect(clicks).toBe(1);
});

test("focus does not wait for a disabled control to be enabled", async () => {
  const input = document.createElement("input");
  input.id = "readonly-input";
  input.setAttribute("aria-disabled", "true");
  document.body.append(input);
  await byId("readonly-input").focus();
  expect(document.activeElement).toBe(input);
});

test("toHaveCount waits for an element that renders after the assertion starts", async () => {
  later(() => {
    const field = document.createElement("input");
    field.id = "late-field";
    document.body.append(field);
  });
  await expect(byId("late-field")).toHaveCount(1);
});

test("not.toBeVisible waits for an element to go away", async () => {
  const banner = document.createElement("p");
  banner.id = "leaving";
  document.body.append(banner);
  later(() => banner.remove());
  await expect(byId("leaving")).not.toBeVisible();
});

test("a retrying assertion that never holds fails with what it last saw", async () => {
  const button = document.createElement("button");
  button.id = "stuck";
  button.disabled = true;
  document.body.append(button);
  await expect(expect(byId("stuck")).toBeEnabled()).rejects.toThrow(
    "expected locator toBeEnabled(), got enabled is false",
  );
}, 15_000);

test("toHaveAttribute and toHaveAccessibleDescription read one element", async () => {
  const note = document.createElement("p");
  note.id = "why";
  note.textContent = "  試跑來源尚未確認。 ";
  const button = document.createElement("button");
  button.id = "apply";
  button.setAttribute("aria-describedby", "why");
  document.body.append(note, button);
  await expect(byId("apply")).toHaveAttribute("aria-describedby", "why");
  await expect(byId("apply")).toHaveAccessibleDescription("試跑來源尚未確認。");
  await expect(byId("apply")).not.toHaveAttribute("disabled");
});

test("toContainText matches case as written, like Playwright", async () => {
  document.body.innerHTML = "<p id='mode'>Draft saved</p>";
  await expect(byId("mode")).toContainText("Draft");
  await expect(byId("mode")).not.toContainText("draft");
});

test("Space toggles a checkbox and Enter toggles a summary's details", async () => {
  const box = document.createElement("input");
  box.type = "checkbox";
  box.id = "agree";
  const details = document.createElement("details");
  const summary = document.createElement("summary");
  summary.id = "more";
  summary.tabIndex = 0;
  details.append(summary);
  document.body.append(box, details);
  await byId("agree").press(" ");
  await byId("more").press("Enter");
  expect([box.checked, details.open]).toEqual([true, true]);
});

test("filter keeps rows that contain a match, and last picks the final one", async () => {
  document.body.innerHTML =
    "<ul><li id='a'>一<button>刪除</button></li><li id='b'>二</li><li id='c'>三<button>刪除</button></li></ul>";
  const rows = new Locator(() => [...document.querySelectorAll("li")]);
  const buttons = new Locator(() => [...document.querySelectorAll("button")]);
  const withButton = rows.filter({ has: buttons });
  expect(withButton.all().map((row) => row.id)).toEqual(["a", "c"]);
  expect(withButton.last().element().id).toBe("c");
});

test("an action on two matches names the count instead of acting on one", async () => {
  for (const id of ["twin", "twin"]) {
    const button = document.createElement("button");
    button.className = id;
    document.body.append(button);
  }
  const twins = new Locator(() => [...document.querySelectorAll(".twin")]);
  await expect(twins.click()).rejects.toThrow("expected exactly one element, found 2");
}, 15_000);
