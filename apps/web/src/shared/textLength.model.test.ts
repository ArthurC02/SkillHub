import { expect, test } from "vitest";
import { fieldCount } from "./textLength.model";

test.each([
  ["at the limit", "字".repeat(4), "4 / 4 字"],
  ["one past the limit", "字".repeat(5), "5 / 4 字——超過了，送出會被伺服器擋下"],
  ["an astral character as one", "𠮷".repeat(4), "4 / 4 字"],
  ["empty", "", "0 / 4 字"],
])("fieldCount %s", (_, text, expected) => {
  expect(fieldCount(text, 4)).toBe(expected);
});

test("fieldCount groups thousands the way the page writes numbers", () => {
  expect(fieldCount("a".repeat(1200), 4000)).toBe("1,200 / 4,000 字");
});
