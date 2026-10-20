import { expect, test } from "vitest";
import { AcquisitionResultUnknown, readAcquisition } from "./acquisition.model";

const receipt = {
  artifact_id: "44444444-4444-4444-8444-444444444444",
  file_name: "x".repeat(128) + ".zip",
  size_bytes: 0,
  content_hash: "sha256:another-release",
  expires_at: "2020-01-01T00:00:00Z",
  duplicate: true,
  content_url: "/downloads/44444444-4444-4444-8444-444444444444/content",
};

test("a retained acquisition keeps its full filename, zero size and authoritative receipt", () => {
  expect(readAcquisition(receipt)).toBe(receipt);
});

test.each([
  ["null", null],
  ["array", []],
  ["primitive", 7],
  ["invalid artifact", { ...receipt, artifact_id: "not-an-artifact" }],
  ["nontext filename", { ...receipt, file_name: {} }],
  ["fractional size", { ...receipt, size_bytes: 0.5 }],
  ["missing hash", { ...receipt, content_hash: null }],
  ["invalid expiry", { ...receipt, expires_at: "not-a-date" }],
  ["nonboolean duplicate", { ...receipt, duplicate: 1 }],
  ["missing URL", { ...receipt, content_url: undefined }],
  [
    "another artifact URL",
    { ...receipt, content_url: "/downloads/55555555-5555-4555-8555-555555555555/content" },
  ],
])("an acquisition with %s remains an unknown result", (_, value) => {
  expect(() => readAcquisition(value)).toThrow(AcquisitionResultUnknown);
});
