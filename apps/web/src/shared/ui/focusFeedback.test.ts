import { afterEach, beforeEach, expect, test } from "vitest";
import { focusFeedback } from "./focusFeedback";

let container: HTMLDivElement;
let feedback: HTMLParagraphElement;
let recoveryHeading: HTMLHeadingElement;
let otherHeading: HTMLHeadingElement;

beforeEach(() => {
  container = document.createElement("div");
  const operation = document.createElement("div");
  feedback = document.createElement("p");
  feedback.tabIndex = -1;
  operation.append(feedback);
  recoveryHeading = document.createElement("h2");
  recoveryHeading.tabIndex = -1;
  otherHeading = document.createElement("h2");
  otherHeading.tabIndex = -1;
  container.append(operation, recoveryHeading, otherHeading);
  document.body.append(container);
});

afterEach(() => container.remove());

test("feedback can receive focus from its affiliated recovery heading", () => {
  recoveryHeading.focus();
  focusFeedback(feedback, otherHeading, recoveryHeading);
  expect(document.activeElement).toBe(feedback);
});

test("feedback preserves focus on an unrelated heading", () => {
  otherHeading.focus();
  focusFeedback(feedback, recoveryHeading);
  expect(document.activeElement).toBe(otherHeading);
});
