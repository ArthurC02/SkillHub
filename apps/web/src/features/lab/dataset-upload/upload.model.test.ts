import { expect, test } from "vitest";
import { uploadErrorSentence, uploadRefusal } from "./upload.model";
import type { DatasetLimits } from "../lab.service";
import { isWriteResultUnknown } from "../lab.service";
import { ApiError, SessionNotReadyError } from "../../../core/api/client";

const LIMITS: DatasetLimits = {
  max_file_bytes: 1000,
  max_test_case_bytes: 5000,
  max_files_per_test_case: 3,
  retention_days: 7,
  allowed_kinds: ["csv"],
  note: "",
};

const file = (size: number) => ({ name: "rows.csv", size });

test("a file on the per-file limit is sent", () => {
  expect(uploadRefusal(file(1000), LIMITS, { fileCount: 0, totalBytes: 0 })).toBe("");
});

test("a file one byte over the per-file limit is refused before it is sent", () => {
  expect(uploadRefusal(file(1001), LIMITS, { fileCount: 0, totalBytes: 0 })).toContain(
    "超過單一檔案上限",
  );
});

test("the last free slot is still a slot", () => {
  expect(uploadRefusal(file(10), LIMITS, { fileCount: 2, totalBytes: 0 })).toBe("");
});

test("a test case already at its file count refuses before the upload starts", () => {
  const refusal = uploadRefusal(file(10), LIMITS, { fileCount: 3, totalBytes: 0 });
  expect(refusal).toContain("達到上限 3 個");
  expect(refusal, "the user is told what to do, not only what failed").toContain("刪掉");
});

test("a file that exactly fills the remaining total is sent", () => {
  expect(uploadRefusal(file(500), LIMITS, { fileCount: 1, totalBytes: 4500 })).toBe("");
});

test("a file one byte past the remaining total names what is left", () => {
  expect(uploadRefusal(file(501), LIMITS, { fileCount: 1, totalBytes: 4500 })).toContain(
    "還剩 500 B 可用",
  );
});

test("the per-file limit is answered before the count, so the reason names the real cause", () => {
  expect(uploadRefusal(file(2000), LIMITS, { fileCount: 3, totalBytes: 4999 })).toContain(
    "超過單一檔案上限",
  );
});

test("a localised upload refusal keeps the actionable server sentence", () => {
  expect(uploadErrorSentence(new ApiError(415, "不支援這種檔案類型"))).toBe("不支援這種檔案類型");
});

test("an English upload refusal is replaced with a local retry sentence", () => {
  expect(uploadErrorSentence(new ApiError(415, "unsupported media type"))).toBe(
    "上傳沒有成功，請檢查檔案類型與大小後再試一次。",
  );
});

test.each<[string, Error]>([
  ["lost response", new TypeError("Failed to fetch")],
  ["unreadable response", new SyntaxError("Unexpected end of JSON input")],
  ["first server error", new ApiError(500, "internal error")],
  ["unavailable server", new ApiError(503, "service unavailable")],
])("%s leaves the upload outcome unknown", (_name, error) => {
  expect(isWriteResultUnknown(error)).toBe(true);
  expect(uploadErrorSentence(error)).toBe(
    "無法確認上傳結果。請先核對下方最新檔案清單，再決定是否重試。",
  );
});

test.each<[number, string]>([
  [400, "上傳沒有成功，請檢查檔案類型與大小後再試一次。"],
  [401, "上傳沒有成功，可以再按一次。"],
  [499, "上傳沒有成功，可以再按一次。"],
])("HTTP %i is a reported refusal rather than an unknown result", (status, message) => {
  const error = new ApiError(status, "request refused");
  expect(isWriteResultUnknown(error)).toBe(false);
  expect(uploadErrorSentence(error)).toBe(message);
});

test("a blocked session operation is known not to have been sent", () => {
  const error = new SessionNotReadyError();
  expect(isWriteResultUnknown(error)).toBe(false);
  expect(uploadErrorSentence(error)).toBe("這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。");
});
