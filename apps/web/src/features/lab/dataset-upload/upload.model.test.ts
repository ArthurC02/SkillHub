import { expect, test } from "vitest";
import { uploadRefusal } from "./upload.model";
import type { DatasetLimits } from "../lab.service";

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
