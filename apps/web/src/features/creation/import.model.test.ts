import { expect, test } from "vitest";
import { importRefusal } from "./import.model";
import { IMPORT_LIMITS } from "../../testing/fixtures/platform";

const max = IMPORT_LIMITS.max_zip_bytes;

test.each([
  [max, false],
  [max + 1, true],
])("a zip of %i bytes is refused: %s", (size, refused) => {
  const refusal = importRefusal("upload", "", { name: "skill.zip", size }, IMPORT_LIMITS);
  expect(refusal !== null).toBe(refused);
  if (refused) expect(refusal).toContain("超過 zip 上限");
});

test.each([
  ["https://github.com/a/b", null],
  ["https://GitHub.com/a/b", null],
  ["https://gist.github.com/a/b", "gist.github.com 不在允許的來源（github.com）裡，沒有送出。"],
  ["not a url", null],
])("the host of %s is checked against the allowed hosts", (url, want) => {
  expect(importRefusal("url", url, undefined, IMPORT_LIMITS)).toBe(want);
});
