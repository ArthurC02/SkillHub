import { expect, test } from "vitest";
import { OVERALL_LABEL, verdictWord } from "./verdict";

test("a known verdict reads its label", () => {
  expect(verdictWord(OVERALL_LABEL, "met")).toBe("符合");
});

test("a verdict the table does not know is shown as the server sent it", () => {
  expect(verdictWord(OVERALL_LABEL, "escalated")).toBe("escalated");
});

test("a missing or empty verdict is 未測量, not a blank", () => {
  expect(verdictWord(OVERALL_LABEL, undefined)).toBe("未測量");
  expect(verdictWord(OVERALL_LABEL, "")).toBe("未測量");
});
