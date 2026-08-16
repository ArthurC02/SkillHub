import { useQuery } from "@tanstack/react-query";
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

export type TestCase = {
  test_case_id: string;
  skill_id: string;
  name: string;
  user_prompt: string;
  acceptance_criteria: AcceptanceCriterion[];
  created_at: string;
  updated_at: string;
};

export type SkillSummary = { skill_id: string; name: string; summary: string };

export function useTestCases() {
  return useQuery({
    queryKey: ["test-cases"],
    queryFn: () => apiFetch<{ test_cases: TestCase[] }>("/test-cases"),
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

export function updateTestCase(testCaseId: string, patch: { name?: string; user_prompt?: string }) {
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
