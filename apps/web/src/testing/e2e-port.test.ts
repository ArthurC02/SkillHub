import { expect, test } from "vitest";
import { previewPort } from "../../e2e/port";

const SHARED_PORT = 4173;

test("CI keeps the shared port whatever the checkout path is", () => {
  expect(previewPort("C:/src/SkillHub", true)).toBe(SHARED_PORT);
  expect(previewPort("/home/runner/work/SkillHub", true)).toBe(SHARED_PORT);
});

test("two checkouts on one machine get different ports above the shared one", () => {
  const main = previewPort("C:/src/SkillHub", false);
  const worktree = previewPort("C:/src/SkillHub/.claude/worktrees/feature-194794", false);
  expect(main).not.toBe(worktree);
  for (const port of [main, worktree]) {
    expect(port).toBeGreaterThan(SHARED_PORT);
    expect(port).toBeLessThanOrEqual(SHARED_PORT + 800);
  }
});

test("the same checkout spelled in another case gets the same port", () => {
  expect(previewPort("c:/SRC/skillhub", false)).toBe(previewPort("C:/src/SkillHub", false));
});
