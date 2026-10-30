import { afterEach, expect, test, vi } from "vitest";
import { apiFetch } from "../../core/api/client";
import { PREFLIGHT } from "../../testing/fixtures/platform";
import { getPreflight } from "./lab.service";

vi.mock("../../core/api/client", async (original) => ({
  ...(await original<typeof import("../../core/api/client")>()),
  apiFetch: vi.fn(),
}));

afterEach(() => vi.resetAllMocks());

const SKILL = "a1111111-1111-4111-8111-111111111111";
const VERSION = "a2222222-2222-4222-8222-222222222222";
const TEST_CASE = "b3333333-3333-4333-8333-333333333333";
const SUMMARY = { ...PREFLIGHT.summary, skill_version_id: VERSION, test_case_id: TEST_CASE };

test.each(["lowercase", "uppercase"])(
  "a matching %s preflight context retains the returned summary",
  async (format) => {
    const result = {
      ...PREFLIGHT,
      summary: {
        ...SUMMARY,
        skill_version_id: format === "uppercase" ? VERSION.toUpperCase() : VERSION,
        test_case_id: format === "uppercase" ? TEST_CASE.toUpperCase() : TEST_CASE,
      },
    };
    const preserved = structuredClone(result);
    vi.mocked(apiFetch).mockResolvedValueOnce(result);

    await expect(getPreflight(SKILL, VERSION, TEST_CASE)).resolves.toEqual(preserved);
    expect(result).toEqual(preserved);
    expect(apiFetch).toHaveBeenCalledExactlyOnceWith(
      "/skills/a1111111-1111-4111-8111-111111111111/runs/preflight?version_id=a2222222-2222-4222-8222-222222222222&test_case_id=b3333333-3333-4333-8333-333333333333",
    );
  },
);

test.each([
  ["another version", { ...SUMMARY, skill_version_id: "c4444444-4444-4444-8444-444444444444" }],
  ["another test case", { ...SUMMARY, test_case_id: "c4444444-4444-4444-8444-444444444444" }],
  ["missing version", { ...SUMMARY, skill_version_id: undefined }],
  ["missing test case", { ...SUMMARY, test_case_id: undefined }],
  ["numeric version", { ...SUMMARY, skill_version_id: 22 }],
  ["numeric test case", { ...SUMMARY, test_case_id: 33 }],
  ["null summary", null],
])("a preflight with %s cannot supply permission facts", async (_name, summary) => {
  vi.mocked(apiFetch).mockResolvedValueOnce({ ...PREFLIGHT, summary });

  await expect(getPreflight(SKILL, VERSION, TEST_CASE)).rejects.toThrow(
    "Invalid preflight response context",
  );
});

test("a null preflight response cannot supply permission facts", async () => {
  vi.mocked(apiFetch).mockResolvedValueOnce(null);

  await expect(getPreflight(SKILL, VERSION, TEST_CASE)).rejects.toThrow(
    "Invalid preflight response context",
  );
});
