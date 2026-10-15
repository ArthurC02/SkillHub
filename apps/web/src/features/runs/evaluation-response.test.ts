import { afterEach, expect, test, vi } from "vitest";
import { apiFetch, ApiError, SessionNotReadyError } from "../../core/api/client";
import { createVersionFromSuggestions, isApplyResultUnknown } from "./evaluation.service";

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
