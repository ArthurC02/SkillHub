import { expect, test } from "vitest";
import { MAX_NAME_CHARS, MAX_PROMPT_CHARS, oversizeReason } from "./test-cases.model";

test("a name or prompt at its limit passes even with the surrounding whitespace the server trims", () => {
  expect(oversizeReason(` ${"名".repeat(MAX_NAME_CHARS)}\n`, "prompt")).toBeNull();
  expect(oversizeReason("name", `\n${"字".repeat(MAX_PROMPT_CHARS)}\n\n`)).toBeNull();
});

test("one character past the trimmed limit is refused with the trimmed count", () => {
  expect(oversizeReason(` ${"名".repeat(MAX_NAME_CHARS + 1)} `, "prompt")).toBe(
    `名稱最多 ${MAX_NAME_CHARS} 個字，目前 ${MAX_NAME_CHARS + 1} 個字。`,
  );
  expect(oversizeReason("name", `${"字".repeat(MAX_PROMPT_CHARS + 1)}\n`)).toBe(
    `Prompt 最多 ${MAX_PROMPT_CHARS} 個字，目前 ${MAX_PROMPT_CHARS + 1} 個字。`,
  );
});
