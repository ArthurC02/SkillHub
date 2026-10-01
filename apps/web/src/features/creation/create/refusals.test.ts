import { describe, expect, test } from "vitest";
import { compositionProblem, raiseBudgetProblem } from "./create.model";

const EMPTY = "還沒有要送出的內容：寫一句話，或附上流程圖、挑一個參考小工具。";
const BOTH =
  "流程圖和參考小工具一次只能送一種。先送其中一種，Agent 讀完之後再送另一種；文字說明可以跟著任一種一起送。";

type CompositionCase = [
  string,
  { note: string; hasFile: boolean; referenceCount: number; expected: string | undefined },
];

const COMPOSITION_CASES: CompositionCase[] = [
  [
    "no sentence, no diagram, no reference is refused as empty",
    { note: "", hasFile: false, referenceCount: 0, expected: EMPTY },
  ],
  [
    "a sentence alone is accepted",
    { note: "整理會議", hasFile: false, referenceCount: 0, expected: undefined },
  ],
  [
    "a diagram alone is accepted",
    { note: "", hasFile: true, referenceCount: 0, expected: undefined },
  ],
  [
    "a reference alone is accepted",
    { note: "", hasFile: false, referenceCount: 1, expected: undefined },
  ],
  [
    "a diagram and a reference together are refused",
    { note: "", hasFile: true, referenceCount: 1, expected: BOTH },
  ],
  [
    "a diagram and a reference are refused even with a sentence",
    { note: "整理", hasFile: true, referenceCount: 2, expected: BOTH },
  ],
];

describe("compositionProblem", () => {
  test.each(COMPOSITION_CASES)("%s", (_name, { note, hasFile, referenceCount, expected }) => {
    expect(compositionProblem(note, hasFile, referenceCount)).toBe(expected);
  });

  test("a sentence of exactly 4000 characters is accepted", () => {
    expect(compositionProblem("字".repeat(4000), false, 0)).toBeUndefined();
  });

  test("a sentence of 4001 characters is refused and says both numbers", () => {
    expect(compositionProblem("字".repeat(4001), false, 0)).toBe(
      "文字說明最多 4000 字，目前 4001 字，請先剪短。",
    );
  });

  test("characters outside the basic plane count once each, not as two halves", () => {
    expect(compositionProblem("𠀀".repeat(4000), false, 0)).toBeUndefined();
  });

  test("an over-long sentence is named before the diagram-and-reference clash", () => {
    expect(compositionProblem("字".repeat(4001), true, 1)).toContain("文字說明最多");
  });
});

describe("raiseBudgetProblem", () => {
  const refused = "請填寫高於目前上限 1300 點且不超過 6500 點的點數。";

  test.each([
    ["one point above the current ceiling is accepted", "1301", undefined],
    ["the current ceiling itself is refused", "1300", refused],
    ["exactly the maximum is accepted", "6500", undefined],
    ["one point over the maximum is refused", "6501", refused],
    ["a fraction is refused", "1300.5", refused],
    ["an empty field is refused", "", refused],
    ["text that is not a number is refused", "兩千", refused],
  ])("%s", (_name, raw, expected) => {
    expect(raiseBudgetProblem(raw, 1300, 6500)).toBe(expected);
  });
});
