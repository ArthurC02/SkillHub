import { describe, expect, it } from "vitest";
import { isCount, isDate, isOptionalText, isRecord, isText, isUuid } from "./wire";

describe("response validators", () => {
  it("a record is a plain object, not null or an array", () => {
    expect(isRecord({})).toBe(true);
    expect(isRecord(null)).toBe(false);
    expect(isRecord([])).toBe(false);
    expect(isRecord("x")).toBe(false);
  });

  it("text is any string and optional text also accepts undefined but not null", () => {
    expect(isText("")).toBe(true);
    expect(isText(1)).toBe(false);
    expect(isOptionalText(undefined)).toBe(true);
    expect(isOptionalText("a")).toBe(true);
    expect(isOptionalText(null)).toBe(false);
  });

  it("a date is a string Date.parse accepts", () => {
    expect(isDate("2026-11-12T00:00:00Z")).toBe(true);
    expect(isDate("not a date")).toBe(false);
    expect(isDate(0)).toBe(false);
  });

  it("a UUID is the dashed hex form in either case", () => {
    expect(isUuid("01234567-89ab-cdef-0123-456789ABCDEF")).toBe(true);
    expect(isUuid("01234567-89ab-cdef-0123-456789abcde")).toBe(false);
    expect(isUuid("01234567-89ab-cdef-0123-456789abcdef0")).toBe(false);
    expect(isUuid("0123456789abcdef0123456789abcdef")).toBe(false);
  });

  it("a count is a safe non-negative integer", () => {
    expect(isCount(0)).toBe(true);
    expect(isCount(-1)).toBe(false);
    expect(isCount(1.5)).toBe(false);
    expect(isCount(Number.MAX_SAFE_INTEGER)).toBe(true);
    expect(isCount(Number.MAX_SAFE_INTEGER + 1)).toBe(false);
    expect(isCount("1")).toBe(false);
  });
});
