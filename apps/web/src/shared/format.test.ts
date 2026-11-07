import { expect, test } from "vitest";
import { bytes, credits } from "./format";

test.each([
  [1023, "1023 B"],
  [1024, "1 KB"],
  [1536, "1.5 KB"],
  [1024 ** 2 - 1, "1024 KB"],
  [1024 ** 2, "1 MB"],
  [1.5 * 1024 ** 2, "1.5 MB"],
  [4_000_000, "3.8 MB"],
  [10 * 1024 ** 2, "10 MB"],
  [1024 ** 3, "1 GB"],
])("%i bytes read as %s", (n, want) => {
  expect(bytes(n)).toBe(want);
});

test.each([
  [null, "未測量"],
  [0, "0 點"],
  [12, "12 點"],
])("credits %s reads as %s", (value, want) => {
  expect(credits(value)).toBe(want);
});
