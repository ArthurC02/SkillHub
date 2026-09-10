import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Dataset } from "./lab";
import type { OwnSkills } from "./types";

export type AcceptanceCriterion = {
  id: string;
  text: string;
  source: "user" | "suggested";
  confirmed_at: string | null;
};

export type RubricItem = {
  id: string;
  text: string;
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
  rubric?: Rubric;
  created_at: string;
  updated_at: string;
};

export type TestCaseListItem = TestCase & {
  skill_name: string;
  criteria_confirmed: number;
  criteria_total: number;
  has_rubric: boolean;
};

export function useTestCases(skillId?: string) {
  return useInfiniteQuery({
    queryKey: ["test-cases", "list", skillId ?? ""],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: "51", offset: String(pageParam) });
      if (skillId) params.set("skill_id", skillId);
      return apiFetch<{ test_cases: TestCaseListItem[] }>(`/test-cases?${params}`).then((page) => ({
        test_cases: page.test_cases.slice(0, 50),
        nextOffset: page.test_cases.length > 50 ? pageParam + 50 : undefined,
      }));
    },
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
    queryFn: () => apiFetch<OwnSkills>("/skills"),
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

export function addCriterion(
  testCaseId: string,
  text: string,
  source: "user" | "suggested" = "user",
) {
  return apiFetch<TestCase>(`/test-cases/${testCaseId}/criteria`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text, source }),
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

export function suggestCriteria(testCaseId: string) {
  return apiFetch<{ suggestions: { text: string }[] }>(
    `/test-cases/${testCaseId}/criteria/suggest`,
    { method: "POST" },
  );
}

export function deleteTestCase(testCaseId: string) {
  return apiFetch<{ deleted: boolean; datasets_deleted: number; note: string }>(
    `/test-cases/${testCaseId}`,
    { method: "DELETE" },
  );
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
