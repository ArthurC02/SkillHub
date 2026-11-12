import { describe, expect, it } from "vitest";
import { loadMoreLabel } from "./loadMoreLabel";

describe("load more label", () => {
  it("waiting for the network wins over a fetch in flight", () => {
    expect(loadMoreLabel(true, true, "載入更多證據")).toBe("等待連線…");
  });

  it("a fetch in flight says it is loading", () => {
    expect(loadMoreLabel(true, false, "載入更多證據")).toBe("載入中…");
  });

  it("an idle button says what the next page holds, or just more by default", () => {
    expect(loadMoreLabel(false, false, "載入更多證據")).toBe("載入更多證據");
    expect(loadMoreLabel(false, false)).toBe("載入更多");
  });
});
