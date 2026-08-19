import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Dataset } from "./lab";

/**
 * 03:TEST-012 — the consumer side of the Test Case and acceptance-criteria
 * endpoints. Every route below already exists in
 * contracts/openapi/public.yaml; nothing here adds or changes a contract.
 *
 * Two facts shape the calls:
 *
 * 1. **The draft is editable, the run is not.** A run freezes the prompt and
 *    the criteria into a snapshot, so editing here never rewrites what a past
 *    run executed or what an evaluation judged (iron rule 4, ADR-003).
 * 2. **Editing text and confirming are one statement.** The server clears a
 *    confirmation when the wording changes, because the agreement was to the
 *    old words — so the client sends `text` and `confirmed` on the same PATCH
 *    rather than trying to preserve a confirmation across an edit.
 */

export type AcceptanceCriterion = {
  id: string;
  text: string;
  /** `suggested` is a model's proposal, never presented as the user's own. */
  source: "user" | "suggested";
  confirmed_at: string | null;
};

/**
 * CONTENT-007's rubric. `id` is the acceptance criterion this item strengthens,
 * not an id of its own: the judge answers one verdict per criterion id and the
 * platform drops any id it did not send, so an item pointing anywhere else could
 * never produce a stored verdict.
 */
export type RubricItem = {
  id: string;
  text: string;
  /** The author's relative-importance signal for the model. Not a score: nothing computes with it. */
  weight?: number;
  evidence_required: boolean;
};

export type Rubric = { version: string; items: RubricItem[] };

export type TestCase = {
  test_case_id: string;
  skill_id: string;
  name: string;
  user_prompt: string;
  acceptance_criteria: AcceptanceCriterion[];
  /** Absent means no rubric — never "the default rubric". */
  rubric?: Rubric;
  created_at: string;
  updated_at: string;
};

export type SkillSummary = { skill_id: string; name: string; summary: string };

export function useTestCases() {
  return useInfiniteQuery({
    queryKey: ["test-cases"],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      apiFetch<{ test_cases: TestCase[] }>(`/test-cases?limit=51&offset=${pageParam}`).then(
        (page) => ({
          test_cases: page.test_cases.slice(0, 50),
          nextOffset: page.test_cases.length > 50 ? pageParam + 50 : undefined,
        }),
      ),
    getNextPageParam: (last) => last.nextOffset,
    retry: false,
  });
}

export function useTestCase(testCaseId: string) {
  return useQuery({
    queryKey: ["test-cases", testCaseId],
    queryFn: () => apiFetch<TestCase>(`/test-cases/${testCaseId}`),
    enabled: testCaseId.length > 0,
    retry: false,
  });
}

export function useOwnSkills() {
  return useQuery({
    queryKey: ["own-skills"],
    queryFn: () => apiFetch<{ skills: SkillSummary[] }>("/skills"),
    retry: false,
  });
}

export function createTestCase(skillId: string, name: string, userPrompt: string) {
  return apiFetch<TestCase>("/test-cases", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ skill_id: skillId, name, user_prompt: userPrompt }),
  });
}

/**
 * `rubric` is three-valued and each value says something different: leaving the
 * key out keeps the stored rubric, an object replaces it, and an explicit `null`
 * removes it. There is no empty rubric.
 */
export function updateTestCase(
  testCaseId: string,
  patch: { name?: string; user_prompt?: string; rubric?: Rubric | null },
) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
}

export function addCriterion(testCaseId: string, text: string) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
  });
}

export function updateCriterion(
  testCaseId: string,
  criterionId: string,
  patch: { text?: string; confirmed?: boolean },
) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria/${criterionId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
}

export function deleteCriterion(testCaseId: string, criterionId: string) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria/${criterionId}`, {
    method: "DELETE",
  });
}

/** TEST-002's optional enhancement. 503 means unavailable, not a failed request. */
export function suggestCriteria(testCaseId: string) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria/suggest`, { method: "POST" });
}

export function useTestCaseDatasets(testCaseId: string) {
  return useQuery({
    queryKey: ["test-cases", testCaseId, "datasets"],
    queryFn: () =>
      apiFetch<{ datasets: Dataset[]; total_bytes: number }>(`/test-cases/${testCaseId}/datasets`),
    enabled: testCaseId.length > 0,
    retry: false,
  });
}

export function deleteDataset(testCaseId: string, datasetId: string) {
  return apiFetch<{ deleted: boolean; dataset_id: string; note: string }>(
    `/test-cases/${testCaseId}/datasets/${datasetId}`,
    { method: "DELETE" },
  );
}
