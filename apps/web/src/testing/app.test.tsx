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

test("an action on two matches names the count instead of acting on one", async () => {
  for (const id of ["twin", "twin"]) {
    const button = document.createElement("button");
    button.className = id;
    document.body.append(button);
  }
  const twins = new Locator(() => [...document.querySelectorAll(".twin")]);
  await expect(twins.click()).rejects.toThrow("expected exactly one element, found 2");
}, 15_000);
