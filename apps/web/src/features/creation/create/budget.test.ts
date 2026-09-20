import { expect, test } from "vitest";
import { nextStepBudget } from "./create.model";

const PER_STEP = 50;

test("a budget with exactly one step left still has room", () => {
  const b = nextStepBudget(
    { budget_credits: 200, reserved_credits: 0, spent_credits: 150 },
    PER_STEP,
  );
  expect(b.remainingCredits).toBe(50);
  expect(b.roomForAnother).toBe(true);
});

test("one credit short of a step has no room", () => {
  const b = nextStepBudget(
    { budget_credits: 200, reserved_credits: 0, spent_credits: 151 },
    PER_STEP,
  );
  expect(b.remainingCredits).toBe(49);
  expect(b.roomForAnother).toBe(false);
});

test("what a step already reserved is not still available", () => {
  const b = nextStepBudget(
    { budget_credits: 200, reserved_credits: 60, spent_credits: 100 },
    PER_STEP,
  );
  expect(b.remainingCredits).toBe(40);
  expect(b.roomForAnother).toBe(false);
});

test("a spend nobody could measure counts as nothing spent, as the server counts it", () => {
  const b = nextStepBudget({ budget_credits: 200, reserved_credits: 0 }, PER_STEP);
  expect(b.remainingCredits).toBe(200);
  expect(b.roomForAnother).toBe(true);
});

test("an overspent budget reports nothing left rather than a negative", () => {
  const b = nextStepBudget(
    { budget_credits: 200, reserved_credits: 0, spent_credits: 260 },
    PER_STEP,
  );
  expect(b.remainingCredits).toBe(0);
  expect(b.roomForAnother).toBe(false);
});
