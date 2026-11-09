import { expect, test } from "vitest";
import { versionNumberWord } from "./versions.model";

test.each([
  ["a listed version", 3, { isPending: false, error: null }, "v3"],
  ["the list still loading", undefined, { isPending: true, error: null }, "版本編號處理中"],
  [
    "the list read failed",
    undefined,
    { isPending: false, error: new Error("503") },
    "版本編號測量失敗",
  ],
  ["read but not listed", undefined, { isPending: false, error: null }, "版本編號未測量"],
] as const)("%s", (_name, number, read, want) => {
  expect(versionNumberWord(number, read)).toBe(want);
});
