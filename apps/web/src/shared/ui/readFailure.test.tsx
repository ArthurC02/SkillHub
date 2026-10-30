import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ApiError } from "../../core/api/client";
import { ReadFailure } from "./LoginRequired";

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test.each([
  [
    new ApiError(429, "太多請求了，請在 9 秒後再試。"),
    "暫時無法讀取搜尋結果。太多請求了，請在 9 秒後再試。",
  ],
  [
    new ApiError(429, "rate limit exceeded"),
    "暫時無法讀取搜尋結果。請求太頻繁了，請稍等一下再試。",
  ],
  [
    new ApiError(503, "太多請求了，請在 9 秒後再試。"),
    "暫時無法讀取搜尋結果。請再試一次，或稍後再試。",
  ],
])("a read refused with %s says %s", async (error, says) => {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  await act(async () => root.render(<ReadFailure error={error} what="搜尋結果" />));

  expect(container.querySelector('[role="alert"]')?.textContent).toBe(says);
  await act(async () => root.unmount());
  container.remove();
});
