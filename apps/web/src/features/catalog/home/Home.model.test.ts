import { expect, test } from "vitest";
import {
  clearedFilterFields,
  filtersFromSearch,
  nextSelection,
  parseSelection,
  queryLengthError,
} from "./Home.model";

test("parseSelection splits the compare list and drops empty entries", () => {
  expect(parseSelection("a,,b")).toEqual(["a", "b"]);
});

test("parseSelection is empty when the search param is absent", () => {
  expect(parseSelection(undefined)).toEqual([]);
});

test("parseSelection truncates to MAX_COMPARE even if the URL carries more", () => {
  expect(parseSelection("a,b,c,d,e")).toEqual(["a", "b", "c"]);
});

test("nextSelection removes an id that is already selected", () => {
  expect(nextSelection(["a", "b"], "a")).toEqual(["b"]);
});

test("nextSelection adds an id below the MAX_COMPARE boundary", () => {
  expect(nextSelection(["a", "b"], "c")).toEqual(["a", "b", "c"]);
});

test("nextSelection refuses to add a fourth id once at the MAX_COMPARE boundary", () => {
  expect(nextSelection(["a", "b", "c"], "d")).toEqual(["a", "b", "c"]);
});

test("filtersFromSearch reads each filter field from the route search params", () => {
  expect(
    filtersFromSearch({
      script: "yes",
      validation: "passed",
      agent: "native",
      tier: "curated",
      category: "documents",
    }),
  ).toEqual({
    script: "yes",
    validation: "passed",
    agent: "native",
    tier: "curated",
    category: "documents",
  });
});

test("filtersFromSearch leaves fields undefined when the route has none of them", () => {
  expect(filtersFromSearch({})).toEqual({
    script: undefined,
    validation: undefined,
    agent: undefined,
    tier: undefined,
    category: undefined,
  });
});

test("queryLengthError accepts a query at the 2000-rune boundary", () => {
  expect(queryLengthError("a".repeat(2000))).toBeNull();
});

test("queryLengthError rejects a query one rune past the boundary, naming the count", () => {
  expect(queryLengthError("a".repeat(2001))).toBe("搜尋文字最多 2000 字，目前 2001 字。");
});

test("clearedFilterFields undefines every filter field", () => {
  expect(clearedFilterFields()).toEqual({
    script: undefined,
    validation: undefined,
    agent: undefined,
    tier: undefined,
    category: undefined,
  });
});
