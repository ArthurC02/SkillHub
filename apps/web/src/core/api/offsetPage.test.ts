import { describe, expect, it } from "vitest";
import { offsetPage } from "./offsetPage";

const rows = (n: number) => Array.from({ length: n }, (_, i) => i);

describe("offset page", () => {
  it("a page of exactly fifty rows is the last page", () => {
    expect(offsetPage("runs", rows(50), 100)).toEqual({ runs: rows(50), nextOffset: undefined });
  });

  it("a fifty-first row only says there is a next page and is not shown", () => {
    expect(offsetPage("runs", rows(51), 100)).toEqual({ runs: rows(50), nextOffset: 150 });
  });

  it("an empty page has no next page", () => {
    expect(offsetPage("test_cases", [], 0)).toEqual({ test_cases: [], nextOffset: undefined });
  });
});
