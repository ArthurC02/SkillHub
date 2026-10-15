import { expect, test } from "vitest";
import { ApiError, SessionNotReadyError } from "../../../core/api/client";
import { startFailureSentence } from "./preflight.model";
import { RunNotStartedError, isWriteResultUnknown } from "../lab.service";

test.each([
  ["lost response", new TypeError("Failed to fetch")],
  ["unreadable response", new SyntaxError("Unexpected end of JSON input")],
  ["invalid acknowledgement", new Error("Invalid run response")],
  ["first server error", new ApiError(500, "internal error")],
  ["unavailable server", new ApiError(503, "service unavailable")],
  ["last server error", new ApiError(599, "upstream error")],
])("a %s warns that starting again can create another Run", (_name, error) => {
  expect(startFailureSentence(error)).toBe(
    "無法確認試跑是否已開始。請先查看試跑紀錄，再決定是否重新送出；再次送出可能建立另一筆試跑並使用額度。",
  );
});

test("a session-blocked start says that the operation was not sent", () => {
  expect(startFailureSentence(new SessionNotReadyError())).toBe(
    "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。",
  );
});

test("a failed confirmation is known not to have requested Run creation", () => {
  const error = new RunNotStartedError(new ApiError(503, "service unavailable"));
  expect(isWriteResultUnknown(error)).toBe(false);
  expect(startFailureSentence(error)).toBe(
    "權限確認未完成，尚未送出開始試跑要求。請重試權限確認。",
  );
});
