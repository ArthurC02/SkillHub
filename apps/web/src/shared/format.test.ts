import { expect, test } from "vitest";
import { bytes } from "./format";

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
