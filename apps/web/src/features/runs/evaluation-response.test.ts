import { afterEach, expect, test, vi } from "vitest";
import { apiFetch, ApiError, SessionNotReadyError } from "../../core/api/client";
import {
  createVersionFromSuggestions,
  isApplyResultUnknown,
  readRejectedSuggestions,
} from "./evaluation.service";

vi.mock("../../core/api/client", async (original) => ({
  ...(await original<typeof import("../../core/api/client")>()),
  apiFetch: vi.fn(),
}));
afterEach(() => vi.resetAllMocks());
const receipt = {
  skill_id: "skill",
  version_id: "version",
  version_number: 1,
  content_hash: "hash",
  duplicate: false,
  applied_suggestion_ids: ["s1"],
  rejected_suggestions: [
    { suggestion_id: "s2", blocked_reason: "target_changed", message: "目標檔案變了。" },
  ],
};

test.each([1, 9007199254740991])(
  "a complete partial-application receipt accepts version %i",
  async (version) => {
    vi.mocked(apiFetch).mockResolvedValue({ ...receipt, version_number: version });
    await expect(
      createVersionFromSuggestions("skill", "evaluation", ["s1", "s2"]),
    ).resolves.toMatchObject({
      version_number: version,
      applied_suggestion_ids: ["s1"],
      rejected_suggestions: [
        { suggestion_id: "s2", blocked_reason: "target_changed", message: "目標檔案變了。" },
      ],
    });
  },
);
test("a duplicate receipt retains the existing version identity", async () => {
  vi.mocked(apiFetch).mockResolvedValue({ ...receipt, duplicate: true });
  await expect(
    createVersionFromSuggestions("skill", "evaluation", ["s1", "s2"]),
  ).resolves.toMatchObject({ version_id: "version", duplicate: true });
});

test("an apply receipt matches identifiers without changing UUID case semantics", async () => {
  vi.mocked(apiFetch).mockResolvedValue({
    ...receipt,
    skill_id: "ABC",
    applied_suggestion_ids: ["S1"],
    rejected_suggestions: [{ ...receipt.rejected_suggestions[0], suggestion_id: "S2" }],
  });
  await expect(
    createVersionFromSuggestions("abc", "evaluation", ["s1", "s2"]),
  ).resolves.toMatchObject({
    skill_id: "ABC",
    applied_suggestion_ids: ["S1"],
  });
});

test("repeated requested suggestions require only one outcome per identifier", async () => {
  vi.mocked(apiFetch).mockResolvedValue(receipt);
  await expect(
    createVersionFromSuggestions("skill", "evaluation", ["s1", "S1", "s2"]),
  ).resolves.toEqual(receipt);
});

test.each([
  ["another Skill", { ...receipt, skill_id: "other-skill" }],
  ["unrequested applied suggestion", { ...receipt, applied_suggestion_ids: ["s3"] }],
  [
    "unrequested rejected suggestion",
    {
      ...receipt,
      rejected_suggestions: [{ ...receipt.rejected_suggestions[0], suggestion_id: "s3" }],
    },
  ],
  ["unaccounted requested suggestion", { ...receipt, rejected_suggestions: [] }],
  ["repeated applied suggestion", { ...receipt, applied_suggestion_ids: ["s1", "S1"] }],
  [
    "repeated rejected suggestion",
    {
      ...receipt,
      rejected_suggestions: [...receipt.rejected_suggestions, ...receipt.rejected_suggestions],
    },
  ],
  ["both applied and rejected suggestion", { ...receipt, applied_suggestion_ids: ["s1", "s2"] }],
])("a receipt with %s cannot confirm the requested operation", async (_name, response) => {
  vi.mocked(apiFetch).mockResolvedValue(response);
  await expect(createVersionFromSuggestions("skill", "evaluation", ["s1", "s2"])).rejects.toThrow(
    "Invalid applied version response",
  );
});
test.each([
  ["null", null],
  ["empty", {}],
  ["blank version", { ...receipt, version_id: " " }],
  ["zero version", { ...receipt, version_number: 0 }],
  ["unsafe version", { ...receipt, version_number: 9007199254740992 }],
  ["fractional version", { ...receipt, version_number: 1.5 }],
  ["missing duplicate marker", { ...receipt, duplicate: undefined }],
  ["no applied suggestions", { ...receipt, applied_suggestion_ids: [] }],
  ["invalid applied identifier", { ...receipt, applied_suggestion_ids: [null] }],
  ["missing rejection list", { ...receipt, rejected_suggestions: undefined }],
  ["null rejection", { ...receipt, rejected_suggestions: [null] }],
  [
    "unknown rejection reason",
    {
      ...receipt,
      rejected_suggestions: [{ suggestion_id: "s2", blocked_reason: "unknown", message: "原因" }],
    },
  ],
  [
    "missing rejection message",
    {
      ...receipt,
      rejected_suggestions: [{ suggestion_id: "s2", blocked_reason: "target_changed" }],
    },
  ],
])("a %s receipt cannot announce a created version", async (_name, response) => {
  vi.mocked(apiFetch).mockResolvedValue(response);
  await expect(createVersionFromSuggestions("skill", "evaluation", ["s1", "s2"])).rejects.toThrow(
    "Invalid applied version response",
  );
});
test.each<[string, Error, boolean]>([
  ["lost response", new TypeError("Failed to fetch"), true],
  ["unreadable response", new SyntaxError("Unexpected end of JSON input"), true],
  ["first server error", new ApiError(500, "internal error"), true],
  ["unavailable server", new ApiError(503, "service unavailable"), true],
  ["no applicable suggestions", new ApiError(422, "refused"), false],
  ["session not confirmed", new SessionNotReadyError(), false],
])("%s has the expected uncertainty classification", (_name, error, unknown) => {
  expect(isApplyResultUnknown(error)).toBe(unknown);
});

test.each([
  "path_out_of_bounds",
  "target_changed",
  "validation_blocked",
  "access_restricted",
  "diff_unavailable",
])("a complete %s refusal retains its identity and explanation", (reason) => {
  const rejected = [{ suggestion_id: "s1", blocked_reason: reason, message: "無法套用。" }];
  expect(readRejectedSuggestions({ rejected_suggestions: rejected })).toEqual(rejected);
});

test("an empty refusal list remains an empty observation", () => {
  expect(readRejectedSuggestions({ rejected_suggestions: [] })).toEqual([]);
});

test.each([
  ["null body", null],
  ["primitive body", "refused"],
  ["absent reasons", {}],
  ["object reasons", { rejected_suggestions: { length: 1 } }],
  ["null reason", { rejected_suggestions: [null] }],
  [
    "missing identity",
    { rejected_suggestions: [{ blocked_reason: "target_changed", message: "無法套用。" }] },
  ],
  [
    "missing explanation",
    { rejected_suggestions: [{ suggestion_id: "s1", blocked_reason: "target_changed" }] },
  ],
  [
    "unknown reason",
    {
      rejected_suggestions: [
        { suggestion_id: "s1", blocked_reason: "future", message: "無法套用。" },
      ],
    },
  ],
  [
    "partly corrupt reasons",
    {
      rejected_suggestions: [
        { suggestion_id: "s1", blocked_reason: "target_changed", message: "無法套用。" },
        null,
      ],
    },
  ],
])("%s cannot be presented as a complete refusal list", (_name, body) => {
  expect(readRejectedSuggestions(body)).toBeUndefined();
});
