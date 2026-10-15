import { afterEach, expect, test, vi } from "vitest";
import { ApiError, SessionNotReadyError } from "../../core/api/client";
import {
  isVersionUploadResultUnknown,
  readVersionUploadFindings,
  saveSkillVersion,
} from "./versions.service";

const SKILL = "abcdefab-1111-4111-8111-111111111111";
const VERSION = "abcdef01-2345-4789-abcd-0123456789ab";
const RECEIPT = {
  skill_id: SKILL,
  version_id: VERSION,
  version_number: 3,
  content_hash: "sha256:next",
  duplicate: false,
  findings: { errors: [], warnings: [], infos: [] },
};
const FINDING = {
  severity: "error",
  code: "name-missing",
  message: "frontmatter 欄位 name 為必填",
};

afterEach(() => vi.unstubAllGlobals());

test.each([
  ["new first version", { version_number: 1 }],
  ["maximum safe version number", { version_number: 9007199254740991 }],
  ["existing content", { duplicate: true }],
  [
    "case-insensitive identity",
    { skill_id: SKILL.toUpperCase(), version_id: VERSION.toUpperCase() },
  ],
])("accepts a complete upload receipt for %s", async (_name, change) => {
  const receipt = { ...RECEIPT, ...change };
  const fetch = vi.fn(() => Promise.resolve(Response.json(receipt, { status: 201 })));
  vi.stubGlobal("fetch", fetch);
  const file = new File(["zip bytes"], "skill.zip");
  await expect(saveSkillVersion(SKILL, file)).resolves.toEqual({
    ...receipt,
    skill_id: SKILL,
    version_id: VERSION,
  });
  expect(fetch).toHaveBeenCalledWith(
    `/skills/${SKILL}/versions`,
    expect.objectContaining({
      method: "POST",
      body: file,
      headers: expect.objectContaining({ "Content-Type": "application/zip" }),
    }),
  );
});

test.each([
  ["null", null],
  ["array", []],
  ["missing fields", {}],
  ["different skill", { ...RECEIPT, skill_id: VERSION }],
  ["invalid skill identity", { ...RECEIPT, skill_id: "invalid" }],
  ["invalid version identity", { ...RECEIPT, version_id: "invalid" }],
  ["missing version identity", { ...RECEIPT, version_id: undefined }],
  ["non-string version identity", { ...RECEIPT, version_id: 3 }],
  ["zero version", { ...RECEIPT, version_number: 0 }],
  ["fractional version", { ...RECEIPT, version_number: 1.5 }],
  ["unsafe version", { ...RECEIPT, version_number: 9007199254740992 }],
  ["string version", { ...RECEIPT, version_number: "3" }],
  ["empty content hash", { ...RECEIPT, content_hash: " " }],
  ["non-string content hash", { ...RECEIPT, content_hash: 3 }],
  ["missing duplicate verdict", { ...RECEIPT, duplicate: undefined }],
  ["non-boolean duplicate verdict", { ...RECEIPT, duplicate: "false" }],
  ["missing findings", { ...RECEIPT, findings: undefined }],
  ["incomplete findings", { ...RECEIPT, findings: { errors: [] } }],
  ["malformed finding", { ...RECEIPT, findings: { errors: [null], warnings: [], infos: [] } }],
])("does not trust an upload receipt with %s", async (_name, receipt) => {
  vi.stubGlobal("fetch", () => Promise.resolve(Response.json(receipt, { status: 201 })));
  await expect(saveSkillVersion(SKILL, new File(["zip bytes"], "skill.zip"))).rejects.toThrow(
    "Invalid version upload response",
  );
});

test.each([
  ["network loss", new TypeError("Failed to fetch"), true],
  ["unreadable receipt", new SyntaxError("Unexpected end of JSON input"), true],
  ["invalid receipt", new Error("Invalid version upload response"), true],
  ["server error", new ApiError(500, "internal server error"), true],
  ["upstream unavailable", new ApiError(503, "service unavailable"), true],
  ["session not ready", new SessionNotReadyError(), false],
  ["not authenticated", new ApiError(401, "not authenticated"), false],
  ["forbidden", new ApiError(403, "forbidden"), false],
  ["missing skill", new ApiError(404, "not found"), false],
  ["oversized file", new ApiError(413, "request body too large"), false],
  ["validation refusal", new ApiError(422, "Unprocessable Entity"), false],
])("classifies %s without inventing a write outcome", (_name, error, unknown) => {
  expect(isVersionUploadResultUnknown(error)).toBe(unknown);
});

test.each([
  ["error", { errors: [FINDING], warnings: [], infos: [] }],
  ["warning", { errors: [], warnings: [{ ...FINDING, severity: "warning" }], infos: [] }],
  [
    "info with details",
    {
      errors: [],
      warnings: [],
      infos: [{ ...FINDING, severity: "info", path: "SKILL.md", details: ["請補上名稱"] }],
    },
  ],
])("reads a confirmed refusal with valid %s findings", (_name, findings) => {
  expect(readVersionUploadFindings(new ApiError(422, "Unprocessable Entity", findings))).toEqual(
    findings,
  );
});

test.each([
  ["null", null],
  ["missing groups", { errors: [] }],
  ["non-array group", { errors: null, warnings: [], infos: [] }],
  ["null item", { errors: [null], warnings: [], infos: [] }],
  ["missing item fields", { errors: [{}], warnings: [], infos: [] }],
  ["invalid severity", { errors: [{ ...FINDING, severity: "critical" }], warnings: [], infos: [] }],
  ["non-string code", { errors: [{ ...FINDING, code: 3 }], warnings: [], infos: [] }],
  ["non-string message", { errors: [{ ...FINDING, message: {} }], warnings: [], infos: [] }],
  ["non-string path", { errors: [{ ...FINDING, path: 3 }], warnings: [], infos: [] }],
  ["non-array details", { errors: [{ ...FINDING, details: "invalid" }], warnings: [], infos: [] }],
  ["non-string details", { errors: [{ ...FINDING, details: [null] }], warnings: [], infos: [] }],
])("does not render refusal details with %s", (_name, findings) => {
  expect(
    readVersionUploadFindings(new ApiError(422, "Unprocessable Entity", findings)),
  ).toBeUndefined();
});

test.each([401, 403, 413, 500, 503])(
  "does not interpret HTTP %s as a validation refusal",
  (status) => {
    expect(
      readVersionUploadFindings(
        new ApiError(status, "error", { errors: [FINDING], warnings: [], infos: [] }),
      ),
    ).toBeUndefined();
  },
);
